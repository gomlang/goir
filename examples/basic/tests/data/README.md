# Native Go reference

`reference_test.go` is a handwritten oracle, copied into the generated test
module by `native_test.goml`. It checks results using Go operators with explicit
64-bit shift masking and exercises Go callers, GC and stack growth.
It is test input rather than a generated golden file.

The benchmark functions compare register and stack emission, and compare a
foldable arithmetic chain before and after SSA simplification. Native test
generation also checks optimized IR against the interpreter and six native
configurations. Program generation uses fixed seeds; temporary directory names
are randomized only to isolate concurrent test runs.

`call_helpers.go.txt` supplies same-package Go helper definitions for every
generated calling backend. `call_runtime_test.go` checks recursive assembly
frames during Go stack growth and GC, with live Go pointers in the caller and
scalar values live across calls. The differential call corpus covers imported
helpers, zero and multiple results, nested calls, recursion, side effects and
bounded loops containing calls.

Successful runs remove their temporary modules. Failed runs retain the module,
one textual `.goir` reproducer per function, the exact generated Go input cases,
and any assembled test executable under `_artifact/goir-native-*` or
`_artifact/goir-calls-*`. Seeds appear in generated function names.

A scalar differential mismatch automatically starts a separate native
reproduction module and invokes `filetest::reduce`, bounded by 24 candidates
and a 30-second budget between attempts. The predicate requires a runtime
value mismatch; compiler failures are not accepted as the same failure.
Successful reduction saves `reduced/reduced.goir`, the exact Go case, and a
reduction report. Call-module failures retain the complete module without
automatic reduction. A native test injects an arithmetic emission fault to
exercise reduction and checks that compilation failures are rejected.

`_artifact/codegen.tsv` reports assembly-source metrics and median optimization
and emission times from three samples. `_artifact/encoded-code.tsv` reports
linked ABI0 symbol sizes from `go tool nm -size`; these include instructions
inserted by the assembler and are distinct from assembly source byte counts.
The linker may discard unused symbols, so this report covers retained symbols.
