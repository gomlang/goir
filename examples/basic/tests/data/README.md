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
