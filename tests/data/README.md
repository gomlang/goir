# IR stage fixtures

The `.goir` files are handwritten input programs in `goir::display` syntax.
The reader accepts one function, consecutive block indices, explicit typed SSA
definitions, and the scalar operations. It resolves instruction dependencies
independently of block layout. Calls require `reader::read_with_references` with
the function and callee references from the same module.
This initial text format requires single-token ASCII function names; arbitrary
IR labels containing whitespace, punctuation delimiters or Unicode are not escaped.

The reader caps input at 1 MiB, tokens at 128 bytes, blocks at 256, value indices
and instruction count at 4096, and dependency resolution at one million steps.
Malformed input, exhausted limits and invalid SSA return structured IR errors.
Formatting normalizes value numbering according to construction order.

`filetest::run` exposes verification, optimization, compilation and interpreter
execution as separate stages. Fixtures exercise loops and a join block laid
out before its predecessors. Reader tests cover malformed IR and round trips.

`filetest::reduce` accepts a reproducing function, an attempt limit and a failure
predicate. It tries removing pure instructions and replacing scalar definitions
with zero, verifies each candidate independently, and retains only candidates for
which the predicate still reports the failure. Calls remain effectful. The
predicate can compare interpreter and native execution; the reducer never
executes native code or writes files without the caller doing so explicitly.
