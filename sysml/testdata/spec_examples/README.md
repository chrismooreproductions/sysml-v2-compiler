# Spec examples

These `.sysml` files are vendored, unmodified, from the OMG reference
implementation's example corpus:

https://github.com/Systems-Modeling/SysML-v2-Release/tree/master/sysml/src/examples

Licensed under EPL-2.0 by the Systems Modeling project; included here
under that license for use as test fixtures only.

They're used by `spec_examples_test.go` to track this parser's coverage
against real, spec-conformant SysML v2 syntax rather than hand-picked
snippets. Don't edit their contents -- if you need different syntax to
test, add a new fixture instead so these stay a faithful mirror of the
upstream files (and so line numbers in the corresponding test's expected
error messages stay accurate).
