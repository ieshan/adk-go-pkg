# adk-go-pkg — AGENTS.md

> Extension library for Google's ADK-Go (`google.golang.org/adk/v2`).
> Go 1.26+ with `iter.Seq2`, range-over-func, and `google.golang.org/genai` types.
> `MEMORY.md` contains project-agnostic Go best practices — read it alongside this file.

## Scope

- Applies to the entire repository.
- Files under `internal/` are not part of the public API.
- Files under `docs/` are documentation — edit when behavior changes warrant updates.

## Project structure

```
agui/            — Generic AG-UI protocol server (zero ADK dependency)
aguiadk/         — ADK-Go to AG-UI bridge
artifact/        — Filesystem-backed artifact service
config/          — YAML/JSON agent loader with factory registry
docs/            — Package-level documentation
eval/            — Evaluation framework (metrics, LLM-as-judge, user simulation)
eval/simulation/ — LLM-backed and static user simulators
internal/        — Internal utilities (jsonutil)
model/           — Model providers (OpenAI, Anthropic)
planner/         — ReAct JSON and Thinking planners
prompt/          — text/template-based prompt templating engine
session/rewind/  — Session rewind to prior events
testutil/        — Fake implementations of all ADK-Go interfaces
```

## Build and test commands

```bash
# Build all packages
go build ./...

# Run all tests with race detector
go test -race -count=1 ./...

# Run tests for a specific package
go test -race -count=1 ./prompt/...
go test -race -count=1 ./config/...
go test -race -count=1 ./planner/...
go test -race -count=1 ./eval/...

# Run a single test
go test -race -run TestBuildDataFromReadonlyContext ./prompt/...

# Check coverage
go test -cover ./prompt/...
go test -coverprofile=cover.out ./... && go tool cover -html=cover.out

# Format
go fmt ./...

# Vulnerability scan (via gocheck — see "Code quality checks" below)
gocheck govulncheck ./...

# Modernize code to latest idioms (Go 1.26)
go fix ./...
```

## CI

GitHub Actions runs `.github/workflows/ci.yml` on every push to `main` and on all pull requests. Six jobs block merges: `lint` (golangci-lint v2), `test` (build + vet + shadow + nilness + test), `test-race`, `govulncheck`, `gosec`, and `nilaway`. The Go version is sourced from `go.mod` via `go-version-file`, so bumping the `go` directive is the only place to update it.

## Code quality checks

Run all of these before committing. Fix all reported issues unless explicitly documented as a known false positive.

### gocheck (preferred runner)

All quality tools below run via the `gocheck` Docker wrapper, so nothing needs to be installed on the host or added to `go.mod`. Prerequisites and setup live in the gocheck README (`/Users/u2/work/docker/tooling/gocheck/README.md`): build the image once with `docker build -t gocheck-image:latest /Users/u2/work/docker/tooling/gocheck`, then ensure the `gocheck` wrapper is on `PATH` (installed to `~/.local/bin`).

The image pins: golangci-lint v2.13.2, gosec v2.29.0, govulncheck v1.7.0, nilaway, and the `shadow`/`nilness` `go vet` vettools from `golang.org/x/tools` v0.49.0. CI installs the same tools directly in GitHub Actions, so local `gocheck` results match CI.

### go vet (standard)

```bash
gocheck go vet ./...
```

### shadow (variable shadowing detector)

Detects variable declarations that shadow outer-scope declarations. Shadowed variables are a common source of subtle bugs.

```bash
gocheck shadow ./...
```

**Known acceptable shadows**: Loop-scoped `t, err :=` inside `for` loops and `if err :=` scoped error checks in `config/builder.go` are standard Go patterns and do not need fixing.

### nilness (impossible nil conditions detector)

Detects impossible nil comparisons and redundant nil checks — conditions the type system proves can never be true.

```bash
gocheck nilness ./...
```

### golangci-lint (meta-linter)

Runs 30+ analyzers in parallel including `staticcheck`, `ineffassign`, `errcheck`, `gosimple`, `unused`, `goconst`, and more.

```bash
gocheck golangci-lint run ./...
```

If a `golangci-lint` config is needed, create `.golangci.yml` at the repo root. Default enabled linters are sufficient for most workflows.

### gosec (security scanner)

```bash
gocheck gosec ./...
```

### nilaway (nilability analyzer)

```bash
gocheck nilaway ./...
```

### govulncheck (vulnerability scan)

```bash
gocheck govulncheck ./...
```

## Code style guidelines

Follow `MEMORY.md` for project-agnostic Go best practices. Key rules:

- **Formatting**: `gofmt` is canonical. Run `go fmt ./...` before every commit.
- **Error handling**: Never ignore returned errors in normal control flow. Use `%w` for wrapping. Keep error strings lowercase and punctuation-light. Use `errors.Is`/`errors.As` for semantic checks, `errors.Join` for multi-failure flows, and `errors.AsType` (Go 1.26) for typed extraction.
- **Context**: First parameter where required. Never pass nil context — use `context.TODO()`. Honor cancellation and deadlines in I/O and concurrency paths. Use `context.Cause(ctx)` when cancellation reasons matter.
- **Concurrency**: Every goroutine needs a clear stop condition. Avoid leaks via blocked sends/receives. Be explicit about shared mutable state ownership.
- **API design**: Exported identifiers need doc comments. Keep interfaces in consumer packages. Prefer small, composable types. Preserve backward compatibility unless explicitly changing major behavior.
- **Tests**: Table-driven with `t.Run`. Test behavior and boundaries, not just happy paths. Keep tests deterministic and isolated. Add tests alongside implementation changes.
- **Modules**: Treat `go.mod` as source of truth. Use `go get`/`go mod tidy` over manual editing. Keep `go.mod` minimal and accurate.

### Go language features (1.26)

This project uses Go 1.26 features. Use them where appropriate:

- `iter.Seq2` and range-over-func for streaming (ADK-Go model returns `iter.Seq2[*LLMResponse, error]`).
- `min`, `max`, `clear` built-ins.
- `errors.Join` for multi-error aggregation.
- `errors.AsType[E error]` for typed generic error extraction.
- `log/slog` for structured logging.
- `testing.B.Loop` for benchmarks (Go 1.24+).
- `os.OpenRoot`/`os.Root` for safer filesystem scoping (Go 1.24+).

### Prompt templating conventions

- Use Go `text/template` for all prompt templates (not `strings.ReplaceAll`).
- Template data accessed via `{{.Input.*}}` for structured input, `{{.State.*}}` for state, `{{.Agent.Name}}` for agent metadata.
- Use `prompt.BuildData()` for simple input-only templates, `prompt.BuildDataFromReadonlyContext()` for agent context templates, `prompt.BuildDataFromInvocationContext()` for full invocation context.
- `TemplateRef` validates exactly one of `Name`, `Inline`, or `Path` is set.
- Template functions (`tojson`, `fromjson`, `truncate`, `indent`, `join`, `lower`, `upper`, `trim`, `default`, `contains`, `hasprefix`, `hassuffix`) are pipeline-friendly with the pipeline value as the last argument.
- `TemplateEngine` is immutable after construction. Use `New()` + `WithFuncs()` for custom functions.
- `TemplateRegistry` is thread-safe and parses templates once for reuse.
- `NewInstructionProviderFromTemplate()` creates `llmagent.InstructionProvider` closures that render templates with context data.

**Exception**: `eval/simulation/per_turn_user_simulator_quality_prompts.go` uses `strings.ReplaceAll` with `{placeholder}` syntax. This is intentional — those templates are consumed by `PerTurnUserSimulatorQualityV1Evaluator` in `eval/` which uses the same `{placeholder}` format. Converting only the simulation copy would create an inconsistency with the evaluator.

## Testing instructions

- Use `testutil` package fakes (FakeLLM, FakeAgent, FakeSession, FakeArtifactService, FakeMemoryService, FakeSessionService, RunnerBuilder) for deterministic testing without external providers.
- Run `go test -race ./...` — race detector is mandatory for concurrency-related changes.
- Add or update tests for every behavior change, even if not explicitly requested.
- After moving files or changing imports, run `go vet ./...` to verify.
- Do not delete or weaken existing tests without explicit direction.
- Use fuzz tests (`FuzzXxx`) for parser/decoder/validation/string-processing boundaries. Keep regression corpus under `testdata/fuzz/...`.
- Use `testing.B.Loop` for benchmarks (Go 1.24+). Benchmark only stable scenarios.

## Security and supply chain

- Keep Go toolchain and dependencies up to date.
- Run `gocheck govulncheck ./...` routinely for vulnerability scanning.
- Treat external input as untrusted; validate at boundaries.
- Never commit secrets, API keys, or credentials.
- Use `os.OpenRoot`/`os.Root` (Go 1.24+) for filesystem boundary constraints where applicable.

## Agent change checklist

### Before coding
- Read `go.mod`, key package docs, and existing tests.
- Identify whether change affects public API, behavior contracts, or compatibility.

### While coding
- Keep deltas minimal and style-consistent.
- Add tests for new behavior and edge cases.
- Be explicit about error wrapping policy (`%w` for inspectable causes).
- Ensure context and goroutine lifecycle correctness.

### Before finishing
- `go fmt ./...`
- `gocheck go vet ./...` (including `gocheck shadow` and `gocheck nilness`)
- `gocheck golangci-lint run ./...`
- `gocheck gosec ./...`
- `gocheck nilaway ./...`
- `gocheck govulncheck ./...`
- `go test -race -count=1 ./...`
- Run targeted benchmarks only when performance claims are made.

## Git / PR conventions

- Keep PRs small (target ≤300 net LOC).
- Commit messages: `type(scope): subject` (Conventional Commits).
- Always run `go fmt`, `gocheck go vet` (with `shadow`/`nilness`), `gocheck golangci-lint`, `gocheck gosec`, `gocheck nilaway`, `gocheck govulncheck`, and `go test -race` before committing.

## Boundaries

### Always do
- Run all code quality checks before finishing a task.
- Add tests for new behavior and edge cases.
- Keep changes minimal and style-consistent with existing code.
- Preserve public API contracts unless explicitly asked to break them.
- Follow `MEMORY.md` Go best practices.

### Ask first
- Adding new production dependencies.
- Changing public API signatures.
- Large refactors exceeding ~300 LOC.
- Modifying `go.mod` Go version directive.

### Never do
- Commit secrets, API keys, or credentials.
- Ignore returned errors in normal control flow.
- Use `panic` for expected failures.
- Edit generated or vendored code.
- Delete or weaken tests without explicit direction.

## Further reading

- [Go release notes](https://go.dev/doc/devel/release)
- [ADK-Go docs](https://pkg.go.dev/google.golang.org/adk/v2)
- [GenAI types](https://pkg.go.dev/google.golang.org/genai)
- [AGENTS.md format](https://agents.md/)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Race detector](https://go.dev/doc/articles/race_detector)
- [Fuzzing](https://go.dev/doc/security/fuzz/)
- [govulncheck](https://go.dev/doc/tutorial/govulncheck)
