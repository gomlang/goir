# goir

`ecosystem::goir` is a GoML SSA library and an initial native code generator for
the Go runtime. The current implementation targets Linux amd64, Go 1.26 and
the Go assembler's ABI0 convention. It emits assembly function bodies and Go
declarations that the ordinary Go toolchain assembles and links.

The library implements typed SSA construction, block parameters, a verifier,
a fuel-bounded interpreter with explicit call frames, scalar function modules,
IR and module printing/reading, shared CFG/dominance/loop/use-def analysis,
executable-edge SCCP, dominance-safe GVN, private-slot mem2reg and bounded LICM,
integer and floating-point operations, private scalar stack slots, automatic SSA
construction from variables, target instruction selection, and constrained
global register allocation with an independent dataflow checker. A reusable compilation
context caches analysis and reports phase timings. The CLI runs phase snapshots,
seeded corpus generation and allocator comparisons. There are no native adapters, cgo requirements
or ecosystem dependencies.

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
| Types | `Type::I64`, `Type::I32`, `Type::F64`, `Type::Bool` |
| Handles | Function-specific `Value`, `Block` and `StackSlot`, with `index()` |
| Constants | `iconst`, `i32const`, `fconst`, `bconst`, `Operation::Constant` |
| Integer operations | `Add`, `Sub`, `Mul`, `And`, `Or`, `Xor`, masked `Shl`/`ShrU`/`ShrS`, checked `ShlChecked`/`ShrUChecked`/`ShrSChecked`, `DivS`/`DivU`/`RemS`/`RemU` |
| Integer comparisons | `icmp`: `Equal`, `NotEqual`, signed and unsigned less-than/less-or-equal |
| Integer conversions | `convert`: `IntConversion::SignExtend`, `ZeroExtend`, `Truncate` |
| Floating-point operations | `fbinary`: `FloatOp::Add`, `Sub`, `Mul`, `Div`; `fcmp`: `FloatCC::Equal`, `NotEqual`, `Less`, `LessEqual` |
| Private memory | `stack_slot(type)`, `stack_load(slot)`, `stack_store(slot, value)` |
| Selection | `select(condition, yes, no)` for values of the same supported type |
| Control flow | `jump`, `branch`, `ret`; edges carry block arguments |
| Construction | `new`, `with_limits`, `entry`, `parameters`, `append_block`, `switch_to`, `ins`, `terminate`, `finish` |
| Inspection | Function name, parameter/result types, value types, value count, block snapshots, `stack_slots()` and `stack_slot_type(slot)` |
| Modules/calls | `Signature`, `FuncRef`, `ModuleBuilder`, `Builder::for_function`, `call` |
| Execution | `interpret`, `interpret_module`, `interpret_module_with_host`, returning `Vec[Datum]` |
| Validation | `verify(function)`, `verified(function)` and `display(function)` |
| Frontend | `Frontend`, `Variable`, `declare_var`, `def_var`, `use_var`, `seal_block` |

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

## Construct SSA from variables

`Frontend::new`, `with_limits` and `for_function` wrap the checked builder.
Declare a typed `Variable`, assign it with `def_var`, and read the current value
with `use_var`. Use `frontend.builder()` for instructions and function inputs;
create blocks and control flow through `Frontend::append_block`, `jump`,
`branch`, `ret` and `switch_to`. Frontend jumps take a target block, and branches
take a condition and two target blocks; callers do not supply block arguments.

Call `seal_block(block)` once all its predecessors have been added, or let
`finish()` seal every block. Sealing prevents new incoming edges. `finish()`
resolves the required block parameters and edge arguments with a bounded fixed
point, then runs the ordinary verifier. Reads without an assignment on an
incoming path return an error. This API handles branches, loops and calls;
`opt::simplify` can subsequently remove redundant parameters. The consumer in
`examples/basic/frontend.goml` exercises assignments, a conditional, a loop and
a module call and participates in independent downstream verification. An ANF
adapter for the GoML compiler remains a separate integration task.

## Execution semantics

Integer addition, subtraction and multiplication wrap modulo 2^64 for `I64`
and modulo 2^32 for `I32`. Integer operations require operands of the same type.
Shift counts are bit patterns masked with 63 or 31 respectively, including
negative counts; Go source shifts need explicit masking to match. `ShrS`
sign-extends and `ShrU` zero-extends. Unsigned comparisons reinterpret the
operands' bit patterns. `SignExtend` and `ZeroExtend` convert `I32` to `I64`;
`Truncate` keeps the low 32 bits of an `I64`.

`DivS` and `RemS` use signed truncation toward zero; `DivU` and `RemU`
reinterpret both operands as unsigned bit patterns. Division by zero returns
`ArithmeticTrap` in the interpreter and produces a recoverable Go panic in native
execution. Signed minimum divided by -1 wraps to the signed minimum, with
remainder zero. The checked shifts treat counts as signed integers: negative
counts trap, and counts at least the operand width produce zero for left/logical
right shifts and sign fill for arithmetic right shifts. The original masked
shift operations retain their semantics. Dead trapping operations remain
observable after optimization; successful constant operations can be folded.

`F64` implements binary64 addition, subtraction, multiplication and division
without fast-math identities, reassociation or contraction. Comparisons follow
IEEE behavior: equality, less-than and less-or-equal are false if either operand
is NaN; not-equal is true. Positive and negative zero compare equal. Arithmetic
NaN payloads are unspecified; constants, selections and copies preserve bits.
`select` takes already-computed SSA values and does not evaluate a branch lazily.

`Datum::bits()` and `datum_from_bits(type, bits)` expose scalar bit patterns.
`Datum` equality compares both type and bits, so it distinguishes signed zeros
and treats matching NaN payloads as equal. Use `fcmp` for floating-point numeric
comparison. Printed floating-point constants use `fconst_bits` with an unsigned
64-bit bit pattern, avoiding decimal round-trip loss.

A `StackSlot` owns exactly one scalar of a declared type. Slots are initialized
to all-zero bits on every invocation, including recursive invocations, and
loads/stores must match the slot's type. Slots have no address-taking, indexing,
escape or aliasing operation; helpers cannot access a caller's slots. Slots
remain separate from SSA spill storage and the outgoing call area.

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
or the Go register ABI. Arguments and results may use any of the four scalar
types, including mixed `I32`, `F64` and boolean signatures.

`interpret_module` runs internal calls with an explicit frame stack, a shared
fuel counter and a default depth bound of 1,024. Imported calls require
`interpret_module_with_host(module, target, args, fuel, max_depth, host)`;
the host callback receives a `FuncRef` and `Vec[Datum]`, and its returned values
are checked against the declared signature. Fuel meters interpreter operations,
not work performed inside the host callback. `ir::rewrite` and `opt::simplify`
preserve module references. Before finalizing a builder, optimized bodies can
replace declarations; on a completed module, `Module::with_function(target,
replacement)` returns a verified replacement module and preserves its reference
identities.

## SSA optimization

`opt::simplify(function)` returns a separately owned, verified function with the
same signature. Optimization is explicit: pass the returned function to either
emitter, or interpret it alongside the original. Emission alone does not run
SSA simplification.

Executable-edge sparse conditional constant propagation (SCCP) tracks unknown,
constant and overdefined values, propagating constants through block parameters
and revisiting loop edges as facts change. Dominance-safe global value numbering
(GVN) shares equivalent pure expressions, normalizing commutative integer
operands without reusing values from sibling branches. Floating-point constant
keys include their exact bit pattern, and floating-point operands are not
reordered.

The pass also folds constant integer operations, comparisons and selections, removes
algebraic identities, folds constant or identical branches, and removes
unreachable blocks, unused instructions and unused non-entry block parameters.
Dead parameter cycles are removed by tracing dependencies from returned values
and branch conditions, including dependencies carried by control-flow edges.
Folding respects wrapping arithmetic and masked shift counts. Control-flow
cleanup repeats while the number of values, blocks or branches decreases.
Block storage order need not follow dominance order.

`Operation::effects()` reports memory reads, memory writes, potential traps and
calls. GVN excludes loads, stores and calls. Private scalar slots have no address
or escape operation, so mem2reg can promote them across branches, loops and
calls. Pruned slot liveness adds block parameters only where incoming contents
are read before being overwritten. Zero initialization becomes typed entry
constants, preserving F64 zero and NaN copy bits. Promotion removes slot accesses
and dead stores; when value or parameter growth exceeds the function's limits,
local forwarding and CFG-aware dead-store elimination retain slot storage.
Calls remain even if they return no values or their results are unused.
Scalar stack accesses are checked statically and non-trapping; calls
conservatively report all four effects.
Loops and divergence remain observable: dead arithmetic inside an infinite loop
can disappear, but the loop remains.
Optimization can change interpreter fuel consumption. Do not expect identical
fuel-exhaustion thresholds before and after optimization.

`opt::simplify_with_options` accepts an `opt::Options` work budget. Exceeding the
budget returns `LimitExceeded` and leaves the input unchanged. The optimizer
does not reassociate arithmetic chains. The pipeline runs scalar simplification
to a fixed point, promotes private slots, simplifies again, performs LICM from
inner natural loops outward, then simplifies again. LICM moves only existing
pure, non-trapping operations whose operands are invariant; it does not move
calls, loads/stores or potentially trapping integer operations. A unique jump
predecessor serves as a preheader; otherwise checked reconstruction creates one
and redirects incoming edges. Transformations that cannot fit the source's shape
or verifier limits fall back or skip hoisting; exhaustion of the optimizer's
shared work budget still returns `LimitExceeded`.

`ir::rewrite(source, blocks)` is the checked reconstruction boundary for custom
passes. It accepts edited block snapshots using existing source handles,
preserves the entry signature, slot types and original resource limits, assigns
fresh handles, and verifies the result. Obtain rewritten slot handles from the
new function; old slot handles belong to the source. Reconstruction can remove
or reorder definitions and blocks but cannot introduce new handles. Missing or duplicate definitions,
foreign handles, type errors and dominance violations return recoverable errors.

`ir::Reconstruction::new(verified_function)` supports passes that introduce
blocks, parameters, values and slots. It reserves fresh handles, retains the
source signature, module references and limits, and verifies the complete
replacement at `finish`. This is the reconstruction boundary used by mem2reg and
LICM.

## Shared analysis

`analysis::analyze` and `analyze_verified` return CFG successor/predecessor
snapshots, reachability, reverse postorder, dominance, natural loops, nesting
weights and per-value definition/use sites, including edge arguments.
`analysis::Graph::new` also accepts an indexed graph directly for machine passes.
Indices and handles are checked and work is charged to a caller-supplied budget.
Graphs allow unreachable nodes; verified SSA functions require every block to
be reachable. Natural loops use dominance backedges, without assuming that block
storage order follows control flow. Irreducible cycles do not receive a natural
loop header or LICM treatment.

`analysis::Context` caches the latest analysis by immutable function identity.
Rewrites have fresh identities and invalidate that cached result; analysis of
the same verified function can reuse it. `clear` releases the cached result and
retains scratch capacity. `statistics` reports builds, hits, invalidations and
retained capacity. Graphs and inspection vectors remain independent of later
context use. Context copies share mutable storage and require serialized use.

## ABI and native output

`abi::amd64_abi0` reports input/result `Slot` records and the TEXT argument size.
Each ABI slot contains its type, byte offset and size. `I64` and `F64` use eight
bytes with eight-byte alignment, `I32` uses four bytes with four-byte alignment,
and booleans use one byte. Results start after inputs rounded up to eight-byte alignment. The declared argument size ends at the last
result, without adding trailing result padding. For a function without results
it ends at the last input, without adding input padding.

`amd64::emit` returns assembly, a Go declaration, frame size and stack-slot
count. `amd64::emit_package` combines 1..256 functions into one assembly file and
one declaration file. Both lower SSA through `mach::lower` and
`regalloc::allocate`. The original implementation is available as
`amd64::emit_stack` and `amd64::emit_stack_package` for comparison.

Lowering selects encodable signed-32-bit immediate operations and constant
shifts with the IR's masking rule, uses `LEA` for multiplication by 3, 5 or 9,
and fuses eligible integer comparisons used only by a branch. Floating-point
operations use SSE2 scalar instructions. Selection separates pure matching from
committing machine instructions. An adjacent private-slot load with one use can
fold into an integer or float arithmetic memory operand when the opcode is legal;
stores, calls and multiple uses prevent the fold. These are explicit GoML matching
rules; there is no ISLE generator or general pattern-selection framework.

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
a cost combining next use and natural-loop use weights, splits lifetimes with
explicit spill/reload edits, and rematerializes integer/float constants when
spilled. Saved spill homes avoid repeated stores while still valid. A bounded
edge-coalescing pass prioritizes loop edges and checks segment interference.
The default `Algorithm::Global` starts with segmented linear allocation, uses a
priority queue to promote valuable spilled ranges and evict lower-cost conflicts,
then assigns regional homes using natural-loop costs and local interference.
The same value can use a register in a hot loop and a stack home on a cold path.
Live values survive calls, and live-through values and block parameters are
reconciled at edges using the destination block's home. `Algorithm::LinearScan`
preserves the earlier whole-function-home baseline. Parallel copies
use DX to break cycles and AX for memory copies.

`Allocation.blocks()` exposes per-operand bindings and ordered before/after,
terminal and edge edits; `Allocation.location(reg)` reports the whole-function
base home. `block_locations(block)` and `entry_location(block, reg)` report
regional homes. Actual operand bindings can differ from either home. The emitter consumes these exact
bindings and edits. `regalloc::verify` checks canonical location conflicts and
runs an independent symbolic dataflow checker. `regalloc::check` validates the
actual move stream, fixed/reused operands, call clobbers and simultaneous block
parameter assignments across a CFG fixed point, without trusting live intervals.

The integer register pool is BX, SI, DI and R8–R11; the floating-point pool is
X3–X9. AX, CX, DX and X0–X2 are reserved scratch locations. SP, BP, R14 and R15
are never allocated. With zero pool registers, constrained instructions use
scratch registers and reusable spill slots. Spill slots and explicit scalar
`StackSlot` storage each occupy eight bytes. Outgoing ABI0 arguments and results
occupy a separate area at the bottom of each caller frame. Reported frame sizes
include all three areas and exclude assembler-added frame-pointer/stack-split
code.

`regalloc::allocate_with_options` accepts 0..7 registers **per register class**
and a shared work budget. `Allocation.registers(class)` lists enabled locations.
Integer register indices are 0..6 and float indices are 7..13;
`register_count()` reports the physical index span, including disabled gaps
when a floating-point pool is present.
`amd64::emit_with_options`, `emit_package_with_options` and
`emit_module_with_options` accept these options. `amd64::emit_allocation`
verifies and emits an already computed allocation.

`allocate_with_algorithm`, `allocate_profiled_with_algorithm` and the amd64
`emit_*_with_algorithm` APIs select either algorithm. `Allocation.statistics()`
reports global/regional evictions, values split across blocks, spill edits,
loop-weighted spill traffic, edge moves and rematerializations. Weights are
heuristics rather than execution counts. The independent checker derives
cross-block definition origins from machine IR and checks regional entry homes
without trusting allocator liveness.

`amd64::Target::standard()` selects Linux amd64, Go 1.26, ABI0, SSE2 and seven
registers in each class. `validate` rejects unsupported OS/architecture/ABI/Go
versions and inconsistent CPU feature lists. Integer and float pool caps can be
configured independently; the current allocator option requests one count for
both classes, which must fit both caps. AVX/AVX2 profiles still emit SSE2 scalar
instructions. `emit_for_target`, `emit_module_for_target_with_algorithm` and
context target methods validate this profile before producing output.

## Reusable compilation context

`amd64::CompilerContext::new().compile(function)` verifies, optimizes, lowers,
allocates, checks and emits one function. Unlike `emit`, this entry point enables
SSA optimization by default. `compile_with_options(function, optimize,
regalloc_options)` selects optimization and allocation settings explicitly.
The returned `Compilation` contains the optimized function, machine IR,
allocation, shared `Analysis`, `Compiled` output and `PhaseTimings` in nanoseconds
for verification, optimization, analysis, lowering, liveness, allocation,
checking, emission and total time.
Optimization time includes verification of rewritten IR.

Reuse a context for successive functions to retain liveness scratch-array and
assembly-line capacities and the latest identity-keyed analysis. It does not
retain every temporary allocation or cache compiled functions. `compile_with_algorithm`
and `compile_for_target_with_algorithm` select allocation and target settings.
`analysis_statistics()` and `clear_analysis()` inspect or release the analysis
cache. `completed_functions()` counts successful calls and
`retained_capacity()` reports retained vector capacity. Results stay independent
of subsequent compilations. Context copies share storage and require serialized
use; they are not synchronized.

The `VerifiedFunction` capability, constructed by `ir::verified`, lets
`opt::simplify_verified`, `ir::rewrite_verified` and `mach::lower_verified`
avoid rechecking their input. Rewritten outputs are still verified, and the
allocation checker runs before context emission. Existing public entry points
continue to validate untrusted inputs.

## Go runtime boundary

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
general panic/defer lowering and JIT loading remain future work. Arithmetic traps
are supported within the scalar boundary.

The Go 1.26 runtime-boundary probe checks two concrete cases: a hand-written,
no-call ABI0 leaf reads a heap pointer supplied by a typed Go prototype while
other goroutines run GC; an ordinary package's `ABIInternal` selector is rejected
by the assembler ([ABI selector restriction](https://github.com/golang/go/blob/go1.26.0/src/cmd/asm/internal/asm/parse.go)).
The pointer probe is separate from the scalar IR and emitter.
It does not establish pointer liveness across calls, pointer-containing stack
frames, stack relocation or heap write-barrier support. Those require separate
runtime metadata and conformance work before adding managed references.

The [Go assembly guide](https://go.dev/doc/asm#runtime-coordination) describes the current integration
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
| Allocation registers per class | 7 | 7 |
| Explicit scalar stack slots | min(4,096, value limit) | min(4,096, value limit) |
| Module declarations, including imports | 256 | 256 |
| Interpreter call depth | 1,024 | 4,096 |

The entry parameters also count toward block/value limits. Dominance uses
O(blocks²) temporary storage. The amd64 emitter separately caps its local
frame at 32,768 bytes and returns `LimitExceeded` before producing assembly
when a function exceeds that bound.
Liveness stores bitsets and limits blocks × ceil(values / 64) to 1,048,576
words per matrix. Liveness, allocation, parallel-copy scheduling and the independent checker share
the allocation work budget. The checker caps blocks × physical locations at
1,048,576 state cells. Interpreter frames use at most 1,048,576 combined SSA
value and explicit-slot cells and share one fuel counter across the call tree.
Frontend SSA construction uses the verifier work limit. Module text input is
limited to 1 MiB.

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

The CLI accepts standalone function text or a complete module:

```sh
goml run --example cli -- check tests/data/loop.goir
goml run --example cli -- run tests/data/loop.goir --arg i64:10
goml run --example cli -- compile tests/data/loop.goir --out _artifact/generated
goml run --example cli -- dump tests/data/loop.goir --stage allocate --registers 1 --algorithm global
goml run --example cli -- snapshot tests/data/loop.goir --arg i64:10 --out tests/data/loop-stages
goml run --example cli -- bench tests/data/loop.goir --samples 15 --out _artifact
goml run --example cli -- fuzz 7 --out _artifact/seed-7
```

`compile` emits Go declarations, amd64 assembly, normalized module IR and a
target manifest with input SHA-256 and required imports. `snapshot` emits verify,
optimize, lower, allocate, compile and interpreter-run outputs; supply `--entry`
for a multi-definition module and one typed `--arg` for each parameter. Lower,
allocate and compile stages optimize by default; `--no-opt` selects original IR.
`--package`, `--registers`, `--algorithm`, `--target` and `--cpu` select emission
settings. Currently the only target profile is `linux-amd64-go1.26-abi0`.
F64 arguments can use `f64bits:U64` to preserve NaNs and signed zero.

`cli::parse` and `cli::execute` expose the command logic as library APIs;
execution returns text and artifacts before the executable writes files.
Compilation generates sources without invoking Go. Imported helpers must be
provided when building the generated package; interpreter commands require a
separate host harness to execute imports. Interpreter fuel does not bound native
execution.

`bench` compares both allocation algorithms with a reusable context and reports
each phase's P50/P95, frames, instructions, spill traffic and analysis-cache
statistics in `pipeline-bench.tsv`. Samples include validation and the first
context use; optimization produces fresh function identities, so analysis hits
are most visible with `--no-opt`. There are no timing thresholds.

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
exact input cases are retained when native tests fail. Recursive calls also
preserve private scalar slots across stack growth and GC.

The typed suite covers all integer operations on `I32` boundary pairs,
sign/zero extension and truncation, immediate and `LEA` selection, floating-point
NaNs, infinities, subnormals and signed zeros, mixed ABI signatures, floating-point
edge cycles and local memory read/write loops. It compares all six backend
configurations with the interpreter. Floating-point copies, selections and slot
loads are checked by bits, including NaN payloads. Arithmetic NaN results are
checked as NaN; other arithmetic results are checked by bits. Separate tests
exercise mixed floating-point calls and runtime-boundary probes.

The scalar extension suite checks signed/unsigned division and remainder,
checked shifts, exceptional paths and private-load folding against an independent
Go implementation in seven configurations. The allocator comparison checks
I64/F64 hot/cold pressure with both algorithms and 0/1/7 pool registers. Its
seven-register fixtures reduced heuristic weighted spill traffic from 293 to 50
and 243 to 15; one-register traffic can increase, and compile-time cost is reported
separately. These are fixture observations rather than general performance claims.

`fuzz::generate(seed)` creates a reproducible, bounded module with a helper call,
non-topological diamond/loop blocks, integer and F64 slots, dead stores, typed
selection, division and both shift contracts. `fuzz::inputs` covers both branch
directions, zero/15 loop iterations, integer boundaries, NaNs and signed zeros.
The seeded native suite checks 25 seeds and eight inputs per seed across nine
configurations: both allocators with 0/1/7 registers, optimized Global with 0/7
registers and the reference stack backend. It retains complete module text,
seed/entry/fuel and typed inputs on failure. Native mismatches trigger a bounded
module reduction against the same backend; compile failures retain the source
without being accepted as a reproduced miscompile. The reducer also preserves
reference identities and signatures.

`reader::read` accepts printed IR with single-token ASCII names;
`read_with_references` additionally accepts a module reference table for calls.
Slot declarations appear before blocks, for example `slot0: i64`.
`reader::display_module` and `read_module` round-trip complete modules, including
imports, declarations and mutually recursive definitions.

`filetest::run` and `run_module` provide verify, optimize, lower, allocate,
compile and interpreter-run stages. Lowering output includes operand constraints
and clobbers, parameters, terminators and edge copies; allocation output includes
regional homes, physical bindings, clobbers and before/after/terminal/edge edits.
`assert_output` checks required/forbidden substrings, `assert_ordered` checks
ordered occurrences and `assert_exact` compares complete output with a line
diagnostic. They do not implement a full FileCheck pattern language. Generated
six-phase goldens are under `tests/data/loop-stages/`; regenerate them with the
snapshot command above. Module run stages use the default
interpreter without a host callback, so executing an import requires a custom
interpreter harness. Fixtures live in `tests/data/`.

`filetest::reduce` and `reduce_module` make bounded, verified candidate edits and
retain only those accepted by a failure predicate. Module reduction preserves
signatures, imports and reference identities while reducing function bodies;
it does not minimize the import table or signatures. Native reproducer paths
preserve the complete source and inputs before attempting reduction. The call
suite stores `module.goir` and `failure.txt` and bounds reduction to 24 attempts
and 30 seconds; failed compilation is not treated as a reproduced miscompile.

The native test writes `_artifact/codegen.tsv` with per-function frame size and
`amd64::metrics` results before and after SSA optimization for both main backends.
Rows also include SSA value counts and optimization/emission times in nanoseconds
as medians of three samples, including validation. Timings are diagnostic rather
than test thresholds.
Metrics count emitted body
instructions, explicit stack reads/writes including ABI argument/result accesses,
and assembly-source bytes. A separate `_artifact/encoded-code.tsv` records
linked ABI0 symbol sizes from `go tool nm`, including assembler-generated code.
The typed suite additionally writes `_artifact/typed-phases.tsv` with three
per-function samples from `CompilerContext`. The runtime probe writes
`_artifact/runtime-boundary.txt`. New reports include `global-allocation.tsv`
with 21-sample allocator P50/P95, `scalar-native.txt`, `seeded-fuzz.tsv` and
`pipeline-bench.tsv`. Reports use actual tab separators. These measurements establish observations,
not performance guarantees or benchmark thresholds. Optional execution
benchmarks write `_artifact/benchmarks.txt`:

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
logs, generated performance/corpus reports and retained native reproducers.
From the sibling verification checkout, run:

```sh
python3 ci/ecosystem.py verify --libraries .. --module goir --goml /absolute/path/to/goml
```

## Source map and next steps

| Files | Responsibility |
| --- | --- |
| `model.goml`, `builder.goml`, `module.goml` | IR, signatures, modules, handles and checked construction |
| `verify.goml`, `verified.goml` | Structural/type validation, dominance and verified-function capabilities |
| `frontend.goml` | Variable-based SSA construction and completed-module body replacement |
| `rewrite.goml`, `reconstruct.goml`, `opt/` | Checked reconstruction, SCCP, GVN, mem2reg, LICM and dead-code/store elimination |
| `analysis/` | Shared CFG, dominance, natural loops, use-def sites and identity-keyed cache |
| `interp.goml`, `numeric.goml`, `display.goml` | Scalar/slot execution, bit semantics and deterministic diagnostics |
| `abi/layout.goml` | Go scalar layout, symbols and declarations |
| `mach/` | Virtual-register instructions, explicit edge copies and SSA lowering |
| `regalloc/` | Segmented liveness, priority/region allocation, splitting, statistics and independent checks |
| `amd64/registers.goml`, `amd64/context.goml`, `amd64/target.goml` | Operand emission, ABI0 calls, target validation and reusable context |
| `reader/`, `filetest/`, `cli/`, `fuzz/` | Text IR, phase snapshots, command logic, seeded modules and reducers |
| `amd64/emit.goml`, `amd64/metrics.goml` | Reference stack backend and source-code metrics |
| `examples/basic/` | Consumer example and native differential validation |

The current work stays within the independent goir library. GoML compiler/ANF
integration is deferred. Further native work can add richer pattern-selection
rules, per-class allocation requests and broader measured global-allocation
strategies. Arbitrary memory requires a new aliasing and effect contract beyond
the current private slots.
Managed references require a separate runtime-aware design for safepoints, stack maps,
stack movement and write barriers. ABIInternal and Go object emission should
be introduced with a pinned toolchain profile and independent ABI conformance
tests before adding JIT loading.
