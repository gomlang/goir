# goir

`ecosystem::goir` is a GoML SSA library and an initial native code generator for
the Go runtime. This first implementation targets Linux amd64, Go 1.26 and
the Go assembler's ABI0 convention. It emits assembly function bodies and Go
declarations that the ordinary Go toolchain assembles and links.

The library implements typed SSA construction, block parameters, a verifier,
a fuel-bounded interpreter with explicit call frames, scalar function modules,
IR printing/reading, executable-edge SCCP and dominance-safe GVN, constrained
machine operands, segmented liveness, register allocation with splitting and an
independent dataflow checker, ABI0 layout and amd64 assembly emission. There are no native adapters, cgo
requirements or ecosystem dependencies.

The design draws on [Cranelift](https://github.com/bytecodealliance/wasmtime/tree/main/cranelift):
a small typed IR, explicit control flow, independently testable ABI lowering,
fast compilation and differential validation. This is an independent
implementation, with a deliberately smaller initial scope.

## Build a function

```goml
use ecosystem::goir as ir;
use ecosystem::goir::amd64;
use ecosystem::goir::opt;

fn compile_add() -> Result[amd64::Package, ir::Error] {
    let builder = ir::Builder::new(
        "Add",
        Vec::from_array([ir::Type::I64, ir::Type::I64]),
        Vec::from_array([ir::Type::I64]),
    )?;
    let args = builder.parameters(builder.entry())?;
    let sum = builder.binary(ir::BinaryOp::Add, args[0], args[1])?;
    builder.ret(Vec::from_array([sum]))?;
    let function = opt::simplify(builder.finish()?)?;
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
| Modules/calls | `Signature`, `FuncRef`, `ModuleBuilder`, `Builder::for_function`, `call` |
| Execution | `interpret`, `interpret_module`, `interpret_module_with_host`, returning `Vec[Datum]` |
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

Native execution has no fuel counter. Generated functions can call other module
functions and imported Go helpers; their values and local frames contain only
scalars. Go helpers may allocate or trigger GC. Long native loops do not provide
polling safepoints and must be bounded by the caller. Interpreter fuel is not a
native execution limit.

## Modules and direct calls

Declare all signatures in a `ModuleBuilder`, construct each body with
`Builder::for_function(reference)`, then `define(reference, function)` and
`finish()`. A call returns a vector of SSA results, including an empty vector for
void calls. Instruction snapshots now have `results: Vec[Value]`; scalar
`Builder::ins` remains a convenience API. References are scoped to their owning
module, definitions must match declared signatures, and unresolved declarations
are rejected. Recursion and mutual recursion use declarations made before bodies.

`ModuleBuilder::import` declares an externally supplied function in the same Go
package. `amd64::emit_module` and `amd64::emit_stack_module` emit definitions and
their Go prototypes; supply imported helper implementations in another `.go`
file in that package. Direct calls use ABI0, with the ordinary Go toolchain
providing wrappers for Go helper bodies. This is not arbitrary symbol linking
or the Go register ABI. All arguments and results remain I64/Bool.

`interpret_module` runs internal calls with an explicit frame stack, a shared
fuel counter and a default depth bound of 1,024. Imported calls require
`interpret_module_with_host(module, target, args, fuel, max_depth, host)`;
the host callback receives a `FuncRef` and `Vec[Datum]`, and its returned values
are checked against the declared signature. Fuel meters interpreter operations,
not work performed inside the host callback. `ir::rewrite` and `opt::simplify`
preserve module references, so optimized bodies can replace their declarations
before the module builder is finalized.

## SSA optimization

`opt::simplify(function)` returns a separately owned, verified function with the
same signature. Optimization is explicit: pass the returned function to either
emitter, or interpret it alongside the original. Emission alone does not run
SSA simplification.

Executable-edge sparse conditional constant propagation (SCCP) tracks unknown,
constant and overdefined values, propagating constants through block parameters
and revisiting loop edges as facts change. Dominance-safe global value numbering
(GVN) shares equivalent pure expressions, normalizing commutative operands
without reusing values from sibling branches.

The pass also folds constant integer operations, comparisons and selections, removes
algebraic identities, folds constant or identical branches, and removes
unreachable blocks, unused instructions and unused non-entry block parameters.
Dead parameter cycles are removed by tracing dependencies from returned values
and branch conditions, including dependencies carried by control-flow edges.
Folding respects wrapping arithmetic and masked shift counts. Control-flow
cleanup repeats while the number of values, blocks or branches decreases.
Block storage order need not follow dominance order.

Calls are conservatively effectful: zero-result calls and calls whose results
are unused remain, and calls are neither folded nor merged. Other current IR
operations are pure and non-trapping. Loops and divergence remain observable:
dead arithmetic inside an infinite loop can disappear, but the loop remains.
Optimization can change interpreter fuel consumption. Do not expect identical
fuel-exhaustion thresholds before and after optimization.

`opt::simplify_with_options` accepts an `opt::Options` work budget. Exceeding the
budget returns `LimitExceeded` and leaves the input unchanged. The optimizer
does not reassociate arithmetic chains or move code out of loops.

`ir::rewrite(source, blocks)` is the checked reconstruction boundary for custom
passes. It accepts edited block snapshots using existing source handles,
preserves the entry signature and original resource limits, assigns fresh
handles, and verifies the result. It can remove or reorder definitions and
blocks but cannot introduce new handles. Missing or duplicate definitions,
foreign handles, type errors and dominance violations return recoverable errors.

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

The machine IR exposes use/def operands, early/late positions and Any, Register,
Fixed and Reuse constraints. Variable shifts require CX, comparisons define AX,
and two-address operations reuse a designated input. Calls explicitly clobber
all allocatable registers and scratch registers. Flags stay internal to atomic
machine pseudo-instructions. Machine and allocation inspection returns deep
vector snapshots.

`regalloc::analyze` computes fixed-point block liveness, whole-function interval
bounds and per-block live segments. Canonical block-entry locations can share a
register or spill slot when their segments do not overlap, including holes in
non-topological block layouts. Within each block, allocation selects victims by
next use, splits lifetimes with explicit spill/reload edits, preserves values
live across calls and reconciles live-through values and block parameters at
edges. Parallel copies use DX to break cycles and AX for memory copies.

`Allocation.blocks()` exposes per-operand bindings and ordered before/after/edge
edits; `Allocation.location(reg)` reports the canonical block-entry location,
not necessarily every instruction's location. The emitter consumes these exact
bindings and edits. `regalloc::verify` checks canonical location conflicts and
runs an independent symbolic dataflow checker. `regalloc::check` validates the
actual move stream, fixed/reused operands, call clobbers and simultaneous block
parameter assignments across a CFG fixed point, without trusting live intervals.

The amd64 register pool is BX, SI, DI and R8–R11. AX, CX and DX are explicit
scratch locations; SP, BP, R14 and R15 are never allocated. With zero pool
registers, constrained instructions use scratch registers and reusable spill
slots. Local spill slots occupy eight bytes. Outgoing ABI0 arguments and results
occupy a separate area at the bottom of each caller frame. Reported frame sizes
include both areas and exclude assembler-added frame-pointer/stack-split code.

`regalloc::allocate_with_options` accepts 0..7 registers and a work budget.
`amd64::emit_with_options`, `emit_package_with_options` and
`emit_module_with_options` accept these options. `amd64::emit_allocation`
verifies and emits an already computed allocation.

The generated assembly uses normal Go stack-splitting prologues supplied by
the assembler, preserves its frame-pointer convention and declares
`NO_LOCAL_POINTERS`. It does not set `NOSPLIT`. Go declarations and the Go
toolchain supply the ABI wrappers and argument/result metadata for the supported
scalar signatures. Emission itself is a pure library operation and does not
invoke the toolchain.

ABI0 support does not imply ABIInternal support. Go's
[internal ABI](https://github.com/golang/go/blob/go1.26.0/src/cmd/compile/abi-internal.md)
is version-dependent. Direct register-ABI output, Go object files, managed
pointers, heap writes, GC maps for managed locals, closures, interfaces,
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
| SSA optimization work steps | 10,000,000 | 100,000,000 |
| Allocation registers | 7 | 7 |
| Module declarations, including imports | 256 | 256 |
| Interpreter call depth | 1,024 | 4,096 |

The entry parameters also count toward block/value limits. Dominance uses
O(blocks²) temporary storage. The amd64 emitter separately caps its local
frame at 32,768 bytes and returns `LimitExceeded` before producing assembly
when a function exceeds that bound.
Liveness stores bitsets and limits blocks × ceil(values / 64) to 1,048,576
words per matrix. Liveness, allocation, parallel-copy scheduling and the independent checker share
the allocation work budget. The checker caps blocks × physical locations at
1,048,576 state cells. Interpreter frames use at most 1,048,576 value cells and
share one fuel counter across the entire call tree.

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
the interpreter over more than 2,000 deterministic cases using original and
simplified IR, register and reference stack backends, a single-register backend
and an optimized all-spill backend. A separately written
Go oracle checks all integer operations/comparisons and exercises large frames,
mixed boolean/integer signatures, multiple returns, Go wrappers exceeding the
register argument budget, cyclic edge copies and concurrent Go callers with GC.
A 12-value rotation tests cyclic copies under register pressure, and a diamond checks
non-topological block layout and a value live through the join. A symbolic
parallel-copy test exhausts all 256 source assignments to four mixed
register/stack destinations. Another 32 programs generated from fixed seeds mix
integer operations, selections and branches, with 512 boundary/random input cases.
Library tests cover constant folding across 100 boundary pairs, dead parameter
cycles, stable repeated optimization, out-of-order blocks and work limits.
Temporary native files are removed on success and retained on failure.

The call suite compares six configurations using generated bounded loops,
nested/multiple-result calls, observable zero-result helpers, 12/20 values live
across calls and recursion that triggers Go GC and stack growth. Sources and
exact input cases are retained when native tests fail.

`reader::read` accepts printed IR with single-token ASCII names; `read_with_references` additionally
accepts a module reference table for calls. `filetest::run` provides verify,
optimize, compile and interpreter-run stages. Fixtures live in `tests/data/`.
`filetest::reduce` makes bounded, verified candidate edits and retains only those
accepted by a supplied failure predicate. The scalar native differential suite
uses this reducer on reproducible mismatches; call-module failures retain the
whole module and inputs for diagnosis.

The native test writes `_artifact/codegen.tsv` with per-function frame size and
`amd64::metrics` results before and after SSA optimization for both main backends.
Rows also include SSA value counts and optimization/emission times in nanoseconds
as medians of three samples, including validation. Timings are diagnostic rather
than test thresholds.
Metrics count emitted body
instructions, explicit stack reads/writes including ABI argument/result accesses,
and assembly-source bytes. A separate `_artifact/encoded-code.tsv` records
linked ABI0 symbol sizes from `go tool nm`, including assembler-generated code.
Optional execution benchmarks write
`_artifact/benchmarks.txt`:

```sh
GOIR_BENCHMARK=1 goml test --example basic native_go --timeout 180s
```

The benchmarks compare a bounded sum loop, a long arithmetic chain and a chain
with foldable constants and redundant operations before/after SSA optimization. Use
repeated measurements on the target machine before drawing performance conclusions.

`goml verify` copies the example into an independent module against an isolated
registry snapshot, then repeats its interpreter/native checks.

The library is registered in the [ecosystem catalog](https://github.com/gomlang/ecosystem)
and [shared verifier](https://github.com/gomlang/verification).
CI pins the shared workflow at a published commit; that revision selects the
checksum-pinned GoML release and sibling repository revisions and runs Go 1.26.x.
It checks formatting, library/example tests, independent downstream verification,
cached builds and the example program. The CI artifact includes the verification
logs, `codegen.tsv` and `encoded-code.tsv`. From the sibling verification checkout, run:

```sh
python3 ci/ecosystem.py verify --libraries .. --module goir --goml /absolute/path/to/goml
```

## Source map and next steps

| Files | Responsibility |
| --- | --- |
| `model.goml`, `builder.goml`, `module.goml` | IR, signatures, modules, handles and checked construction |
| `verify.goml` | Structural/type validation, reachability and dominance |
| `rewrite.goml`, `opt/` | Checked reconstruction, SCCP, GVN and effect-aware dead-code elimination |
| `interp.goml`, `display.goml` | Reference execution and deterministic diagnostics |
| `abi/layout.goml` | Go scalar layout, symbols and declarations |
| `mach/` | Virtual-register instructions, explicit edge copies and SSA lowering |
| `regalloc/` | Segmented liveness, next-use splitting, edge moves and symbolic allocation checks |
| `amd64/registers.goml` | Operand-driven assembly and scalar ABI0 call lowering |
| `reader/`, `filetest/` | Bounded text IR reader, stage runner and failure-preserving reducer |
| `amd64/emit.goml`, `amd64/metrics.goml` | Reference stack backend and source-code metrics |
| `examples/basic/` | Consumer example and native differential validation |

Further allocation work can compare global splitting/coalescing strategies and
measured compile-time costs against this bounded block-local allocator. Managed
references require a separate runtime-aware design for safepoints, stack maps,
stack movement and write barriers. ABIInternal and Go object emission should
be introduced with a pinned toolchain profile and independent ABI conformance
tests before adding JIT loading.
