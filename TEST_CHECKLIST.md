# Test Quality Checklist

A checklist for ensuring tests in this codebase are high quality and non-redundant. Run it against new and existing tests before merging.

Grounded in the [Go Wiki: Test Comments](https://go.dev/wiki/TestComments), [TableDrivenTests](https://go.dev/wiki/TableDrivenTests), [Code Review Comments](https://go.dev/wiki/CodeReviewComments), the [`testing` package docs](https://pkg.go.dev/testing), Go 1.24-1.26 release notes (`t.Chdir`, `testing.B.Loop`, `errors.AsType`), and this repo's `AGENTS.md` conventions.

## 1. Structure & Organization

- [ ] Test files end in `_test.go` and live alongside the code under test
- [ ] Use black-box (`package foo_test`) by default; white-box only when accessing unexported symbols
- [ ] Table-driven tests with `t.Run` for multiple inputs sharing the same logic
- [ ] Separate test functions when different cases need different assertion logic (don't force conditional logic into one table)
- [ ] Each test function tests one behavior/aspect (e.g., `TestFoo_Add` vs `TestFoo_Save`); split if >~200 lines
- [ ] Test naming follows `TestFoo_Bar` convention (underscore separator, descriptive)
- [ ] Descriptive, human-readable subtest names (spaces OK; runner escapes them); use `t.Log` for input details

## 2. Assertions & Failure Messages

- [ ] No assert libraries; use plain Go comparisons
- [ ] `t.Errorf` to keep going and collect multiple failures; `t.Fatalf` only for setup failures or when continuing would panic
- [ ] Failure messages follow "got X, want Y" (got before want)
- [ ] Include the function name and inputs in failure messages: `FuncName(%v) = %v, want %v`
- [ ] Compare full structs with `cmp.Diff` (not per-field `if` checks); `go-cmp` is already a dependency
- [ ] Prefer `cmp` over `reflect.DeepEqual` for new code; migrate older `reflect.DeepEqual` usage where practical
- [ ] Print diffs (with direction key like `diff -want +got`) for large output
- [ ] Compare stable/semantic results, not exact serialized bytes (e.g., parse JSON then compare structures)

## 3. Error Handling in Tests

- [ ] Test error semantics with `errors.Is`/`errors.As`/`errors.AsType` (Go 1.26+), not string matching on error messages
- [ ] String-matching only OK for checking a property (e.g., contains param name)
- [ ] Don't ignore returned errors in test setup/control flow
- [ ] Test both error paths and success paths

## 4. Test Helpers & Cleanup

- [ ] Helpers call `t.Helper()` first so failures point to the call site
- [ ] Use `t.Cleanup` for teardown (not `defer`) — runs even after `t.FailNow`
- [ ] Use `t.TempDir()` (Go 1.15+) instead of `os.MkdirTemp` + manual cleanup
- [ ] Use `t.Setenv()` (Go 1.17+) instead of `os.Setenv` + defer restore
- [ ] Use `t.Chdir()` (Go 1.24+) instead of `os.Chdir` + manual restore
- [ ] Helpers use `t.Fatalf` for setup failures (don't return errors from must-succeed helpers)
- [ ] No `init()` in test files; use setup helpers or `TestMain` for shared expensive resources

## 5. Isolation & Determinism

- [ ] Tests are deterministic — no reliance on wall-clock, random order, network, or external services
- [ ] Use `testutil` fakes (FakeLLM, FakeSession, etc.) instead of real providers
- [ ] Each test gets fresh state; no shared mutable global state between tests
- [ ] No flaky timing; use fakes/synthetic clocks over `time.Sleep`
- [ ] Honor context cancellation/deadlines in code under test
- [ ] Use `testing.Short()` to skip slow/integration tests when `-short` is passed

## 6. Concurrency & Race Safety

- [ ] Run `go test -race` (mandatory in this repo)
- [ ] Use `t.Parallel()` for independent tests/subtests (not with `t.Setenv`, `t.Chdir`, or shared FS state)
- [ ] Every goroutine in tests has a clear stop condition; no leaked goroutines
- [ ] Be explicit about shared mutable state ownership

## 7. Coverage & Boundaries

- [ ] Test behavior and boundaries, not just happy paths
- [ ] Cover edge cases: nil/zero/empty inputs, max/min, off-by-one, unicode
- [ ] Test public API contracts; don't over-couple to implementation details
- [ ] Coverage is breadth; pair with `-race` and fuzz for depth
- [ ] Add/update tests for every behavior change
- [ ] Consider `Example` functions for new packages to demonstrate intended usage

## 8. Fuzz, Benchmarks & Test Fixtures

- [ ] Fuzz tests (`FuzzXxx`) for parser/decoder/validation/string-processing boundaries
- [ ] Keep fuzz regression corpus under `testdata/fuzz/...`
- [ ] Benchmarks use `testing.B.Loop` (Go 1.24+); benchmark only stable scenarios
- [ ] Seed corpus covers meaningful initial shapes
- [ ] Store large test fixtures in `testdata/` (Go tool ignores this directory); use golden-file pattern with `-update` flag for large expected outputs

## 9. Non-Redundancy

- [ ] No two tests share the same preconditions + actions + expected result (one can go)
- [ ] Data variations that don't change expected behavior are collapsed into one case
- [ ] No "just in case" edge cases with no real risk justification and no caught bugs
- [ ] Tests that always fail alongside others covering the same module — review for consolidation
- [ ] Don't re-test stdlib/dependency behavior; test your code's integration with it
- [ ] Shared setup repeated across many tests → consolidate into a helper or `TestMain`
- [ ] Each test adds distinct information: a new branch, input class, or invariant — if removing a test loses no unique coverage, it's redundant

## 10. Pre-Commit Verification

- [ ] `go fmt ./...`
- [ ] `go vet ./...` (incl. shadow, nilness)
- [ ] `golangci-lint run ./...`
- [ ] `go test -race -count=1 ./...`
- [ ] Targeted benchmarks only when making performance claims
