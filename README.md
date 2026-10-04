# goir

`ecosystem::goir` is a GoML SSA library and an initial native code generator for
the Go runtime. This first implementation targets Linux amd64, Go 1.26 and
the Go assembler's ABI0 convention. It emits assembly function bodies and Go
declarations that the ordinary Go toolchain assembles and links.

The library implements typed SSA construction, block parameters, a verifier,
a fuel-bounded interpreter, deterministic IR printing, a virtual-register machine
IR, liveness analysis, register allocation, ABI0 argument/result layout and amd64
assembly emission. There are no native adapters, cgo
requirements or ecosystem dependencies.

The design draws on [Cranelift](https://github.com/bytecodealliance/wasmtime/tree/main/cranelift):
a small typed IR, explicit control flow, independently testable ABI lowering,
fast compilation and differential validation. This is an independent
implementation, with a deliberately smaller initial scope.

## Build a function

```goml
use ecosystem::goir as ir;
use ecosystem::goir::amd64;

fn compile_add() -> Result[amd64::Package, ir::Error] {
    let builder = ir::Builder::new(
        "Add",
        Vec::from_array([ir::Type::I64, ir::Type::I64]),
        Vec::from_array([ir::Type::I64]),
    )?;
    let args = builder.parameters(builder.entry())?;
    let sum = builder.binary(ir::BinaryOp::Add, args[0], args[1])?;
    builder.ret(Vec::from_array([sum]))?;
    let function = builder.finish()?;
    amd64::emit_package("generated", Vec::from_array([function]))
}
```

Write `Package.assembly` to `generated_amd64.s` and `Package.declarations` to
`generated.go` in the same Go package. Both outputs are required. Go callers use
ordinary typed functions, for example `generated.Add(20, 22)`. Package emission
checks symbol uniqueness and emits the assembler include once.

The local module is ready for development and isolated downstream verification;
it has not been published as a registry version. Sources use `.goml` and require
GoML 0.1.57 or newer.

## IR and builder

| Area | Supported API |
| --- | --- |
| Types | `Type::I64`, `Type::Bool` |
| Values | Function-specific `Value` and `Block` handles, with `index()` |
| Constants | `iconst`, `bconst`, `Operation::Constant` |
| Integer operations | `Add`, `Sub`, `Mul`, `And`, `Or`, `Xor`, `Shl`, `ShrU`, `ShrS` |
| Comparisons | `Equal`, `NotEqual`, signed and unsigned less-than/less-or-equal |
| Selection | `select(condition, yes, no)` for either supported type |
| Control flow | `jump`, `branch`, `ret`; edges carry block arguments |
| Construction | `new`, `with_limits`, `entry`, `parameters`, `append_block`, `switch_to`, `ins`, `terminate`, `finish` |
| Inspection | Function name, parameter/result types, value types, value count and block snapshots |
| Execution | `interpret(function, arguments, fuel)` returning `Vec[Datum]` |
| Validation | `verify(function)` and `display(function)` |

`Builder::new` creates the entry block and its parameters from the input
signature. New blocks declare their parameter types before edges are added.
Every block must end with one terminator; all blocks must be reachable from
entry, and entry cannot have incoming edges. Conditions are booleans.
Every use must be dominated by its definition, including edge arguments.
The verifier handles loops iteratively and bounds dominance work.

Block arguments are simultaneous assignments. On a backedge such as
`loop(b, a, n - 1)`, swapping two parameters preserves both old values.
The interpreter gathers arguments before assigning them. The default native
backend schedules parallel copies between assigned registers and spill slots,
breaking cycles with a reserved scratch register. The reference stack backend
stages arguments in temporary stack slots.

Builders use shared mutable storage and are not synchronized. Copies refer to
the same builder. A successful `finish` closes all copies and returns a
read-only function; further mutations return `InvalidState`. A failed finish
leaves the builder open. Input vectors and terminators are copied, and public
function/block inspection returns independent vector snapshots.

Handles from another function return `WrongFunction`. Invalid IR and unsupported
emission requests return `Result` errors; diagnostics include block/value
indices where relevant. Function names in IR are labels of 1..128 bytes.
Go emission requires ASCII Go identifiers and rejects keywords, duplicate
symbols and names that conflict with generated declarations.

## Execution semantics

Integer addition, subtraction and multiplication wrap modulo 2^64. Bitwise
operations preserve all 64 bits. Shift counts are interpreted as bit patterns
and masked with 63, including negative counts; this differs from Go source
shifts unless the caller explicitly masks the count. `ShrS` sign-extends and
`ShrU` zero-extends. Comparisons return `Bool`; unsigned comparisons reinterpret
the operands' bit patterns. `select` takes already-computed SSA values.

The interpreter checks argument count and types, verifies the function, and
charges one fuel unit for every instruction and terminator. Even an empty
infinite loop eventually returns `FuelExhausted`. Verification has a separate
work budget.

Native execution has no fuel counter. These are ordinary assembly leaf
functions: the body makes no calls, allocates no Go heap objects and holds no
GC references. Long native loops do not provide polling safepoints and must
be bounded by the caller. Interpreter fuel is not a native execution limit.

## ABI and native output

`abi::amd64_abi0` reports input/result `Slot` records and the TEXT argument size.
Each slot contains its type, byte offset and size. Integers use eight bytes
with eight-byte alignment; booleans use one byte. Results start after inputs
rounded up to eight-byte alignment. The declared argument size ends at the last
result, without adding trailing result padding. For a function without results
it ends at the last input, without adding input padding.

`amd64::emit` returns assembly, a Go declaration, frame size and stack-slot
count. `amd64::emit_package` combines 1..256 functions into one assembly file and
one declaration file. Both lower SSA through `mach::lower` and
`regalloc::allocate`. The original implementation is available as
`amd64::emit_stack` and `amd64::emit_stack_package` for comparison.

The machine IR makes virtual register definitions, uses and edge copies explicit.
`mach::Function` exposes copied block snapshots and function-specific `Reg`
handles. `regalloc::analyze` computes fixed-point block liveness and conservative
whole-function intervals, including loops and blocks stored out of control-flow
order. `regalloc::allocate` uses deterministic linear scan and reuses expired
spill slots. Each virtual register has one location for its entire interval;
there is no interval splitting, eviction, copy coalescing or dead-code removal.
`regalloc::verify` checks location bounds and interval conflicts.

The amd64 allocator uses BX, SI, DI and R8–R11. AX, CX and DX remain reserved
for instruction operands and parallel-copy scheduling; allocation never uses
SP, BP, R14 or R15. Instructions currently pass operands through scratch
registers, so allocation primarily reduces memory traffic and edge-copy work.
Spilled values occupy eight-byte local stack slots. The frame size reports
these locals, excluding assembler-added frame-pointer and stack-split code.

`regalloc::allocate_with_options` accepts a register count and work budget.
`amd64::emit_with_options` and `amd64::emit_package_with_options` accept the same
options, restricted to 0..7 registers. Zero forces all values into reusable spill
slots. `amd64::emit_allocation` emits an already computed allocation.

The generated assembly uses normal Go stack-splitting prologues supplied by
the assembler, preserves its frame-pointer convention and declares
`NO_LOCAL_POINTERS`. It does not set `NOSPLIT`. Go declarations and the Go
toolchain supply the ABI wrappers and argument/result metadata for the supported
scalar signatures. Emission itself is a pure library operation and does not
invoke the toolchain.

ABI0 support does not imply ABIInternal support. Go's
[internal ABI](https://github.com/golang/go/blob/go1.26.0/src/cmd/compile/abi-internal.md)
is version-dependent. Direct register-ABI output, Go object files, managed
pointers, heap writes, GC maps for managed locals, calls, closures, interfaces,
panic/defer, floating-point values and JIT loading remain future work.
The [Go assembly guide](https://go.dev/doc/asm) describes the current integration
boundary.

## Limits

| Resource | Default | Maximum configurable value |
| --- | ---: | ---: |
| Blocks per function | 256 | 1,024 |
| SSA values per function | 4,096 | 65,536 |
| Parameters per block | 256 | 1,024 |
| Inputs or results per signature | 256 | 1,024 |
| Verifier work steps | 10,000,000 | 100,000,000 |
| Allocation work steps | 10,000,000 | 100,000,000 |
| Abstract allocation registers | 7 | 32 (amd64: 7) |

The entry parameters also count toward block/value limits. Dominance uses
O(blocks²) temporary storage. The amd64 emitter separately caps its local
frame at 32,768 bytes and returns `LimitExceeded` before producing assembly
when a function exceeds that bound.
Liveness stores bitsets and limits blocks × ceil(values / 64) to 1,048,576
words per matrix. Liveness and allocation share the allocation work budget;
parallel-copy scheduling separately caps dependency-search work at 10,000,000
steps per edge.

## Development and examples

Put GoML 0.1.57+ and Go 1.26.x on PATH. Native tests require Linux amd64.
From this repository:

```sh
goml fmt --check
goml test --timeout 180s
goml verify --timeout 180s
goml run --example basic
goml run --example basic -- _artifact/demo
```

The example constructs `Sum(n)` and a two-value swap loop. Without an output
directory it prints IR and assembly and checks `Sum(100) == 5050`; with a
directory it writes the two Go package source files.

`goml test` includes public API and verifier regressions plus the example's
native integration test. That test generates a temporary Go module under
`_artifact/`, runs `go vet` and `go test`, and compares generated functions with
the interpreter over more than 2,000 deterministic cases using the register,
reference stack and single-register backends. A separately written
Go oracle checks all integer operations/comparisons and exercises large frames,
mixed boolean/integer signatures, multiple returns, Go wrappers exceeding the
register argument budget, cyclic edge copies and concurrent Go callers with GC.
A 12-value rotation additionally tests forced spills, and a diamond checks
non-topological block layout and a value live through the join. A symbolic
parallel-copy test exhausts all 256 source assignments to four mixed
register/stack destinations.
Temporary native files are removed on success and retained on failure.

The native test writes `_artifact/codegen.tsv` with per-function frame size and
`amd64::metrics` results for both main backends. Metrics count emitted body
instructions, explicit stack reads/writes including ABI argument/result accesses,
and assembly-source bytes. They do not measure encoded machine-code size or
assembler-inserted instructions. Optional execution benchmarks write
`_artifact/benchmarks.txt`:

```sh
GOIR_BENCHMARK=1 goml test --example basic native_go --timeout 180s
```

The benchmarks compare a bounded sum loop and a long arithmetic chain. Use
repeated measurements on the target machine before drawing performance conclusions.

`goml verify` copies the example into an independent module against an isolated
registry snapshot, then repeats its interpreter/native checks. The standalone
CI workflow pins the released GoML archive checksum and Go 1.26.0.

This repository currently uses standalone CI. Central ecosystem registration must update
`ecosystem/catalog.json`, the verifier's module list and
`verification/ci/repositories.json` together with real published commit IDs;
the historical split manifests must remain unchanged.

## Source map and next steps

| Files | Responsibility |
| --- | --- |
| `model.goml`, `builder.goml` | IR, handles, snapshots and checked construction |
| `verify.goml` | Structural/type validation, reachability and dominance |
| `interp.goml`, `display.goml` | Reference execution and deterministic diagnostics |
| `abi/layout.goml` | Go scalar layout, symbols and declarations |
| `mach/` | Virtual-register instructions, explicit edge copies and SSA lowering |
| `regalloc/` | Bounded liveness, linear scan, spill-slot reuse and allocation checks |
| `amd64/registers.goml`, `amd64/copies.goml` | Allocated assembly and parallel-copy scheduling |
| `amd64/emit.goml`, `amd64/metrics.goml` | Reference stack backend and source-code metrics |
| `examples/basic/` | Consumer example and native differential validation |

The next implementation stages are local SSA simplification, fewer redundant
register moves, improved spill decisions and explicit call lowering. Managed
references require a separate runtime-aware design for safepoints, stack maps,
stack movement and write barriers. ABIInternal and Go object emission should
be introduced with a pinned toolchain profile and independent ABI conformance
tests before adding JIT loading.
