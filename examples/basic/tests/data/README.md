# Native Go reference

`reference_test.go` is a handwritten oracle, copied into the generated test
module by `native_test.goml`. It checks results using Go operators with explicit
64-bit shift masking and exercises Go callers, GC and stack growth.
It is test input rather than a generated golden file.
