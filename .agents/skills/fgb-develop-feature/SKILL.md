---
name: fgb-develop-feature
description: Develop, fix, or refactor flutter_go_bridge features with the project's required Git branching, code analysis, documentation, and staged validation workflow. Use for changes to the Go parser/generator/runtime, generated Dart API, CLI, integration templates, examples, or bridge behavior that must progress through unit tests, integration tests, smoke tests, and a feature-branch commit.
---

# FGB Feature Development

Follow every gate in order. Do not commit code that has not passed unit, integration, and smoke testing.

## 1. Prepare Git Before Editing

Create a fresh branch from the latest remote `main` before changing files.

1. Inspect the worktree with `git status --short`.
2. If unrelated or user-owned changes exist, preserve them. Do not reset, discard, or stash them without explicit permission.
3. Fetch and update `main`:

```text
git fetch origin
git switch main
git pull --ff-only origin main
```

4. Create a new branch from the updated `main`:

```text
git switch -c feature/<short-name>  # new behavior
git switch -c fix/<short-name>      # bug fix
git switch -c chore/<short-name>    # maintenance only
```

Never develop or commit directly on `main`. If work already exists before this workflow was applied, stop and preserve it before updating `main`; do not risk losing changes merely to satisfy the branch sequence.

## 2. Understand the Change

Read the relevant parser, generator IR, Go/Dart renderers, runtime, CLI, tests, configuration, templates, and real examples before editing. Trace both bridge directions when types or codecs change:

```text
Dart -> CST/standard decode -> Go call
Go result -> DCO/standard encode -> Dart
```

Use existing repository patterns and keep the change scoped. Add a regression test that fails for the original behavior before or alongside the implementation.

For generated-code changes, determine whether these areas also need updates:

- `internal/parser`
- `internal/generator`
- `internal/integrate` and `template`
- generated Go and Dart examples
- command/config behavior

## 3. Implement and Document

Implement the smallest complete change. Preserve user edits and avoid unrelated refactors.

When adding a feature or changing user-visible behavior, update `README.md` in the same branch. Document:

- supported syntax and types;
- generated Dart/Go behavior;
- limitations and ownership rules;
- a concise usage example when useful.

Regenerate checked-in generated output when the repository treats it as source. Never hand-edit generated files when the generator is the source of truth.

## 4. Run Code Analysis

Run analysis before declaring unit tests ready, and repeat it after relevant fixes.

### Go

Format changed Go files with `gofmt`. Run `go vet ./...` when the change can affect compilation or runtime behavior.

Use `gopls` to check changed Go files. Install it when unavailable:

```text
go install golang.org/x/tools/gopls@latest
```

On PowerShell, resolve the installed binary explicitly when `go install` does not update `PATH`:

```powershell
$gopls = (Get-Command gopls -ErrorAction SilentlyContinue).Source
if (-not $gopls) {
  go install golang.org/x/tools/gopls@latest
  $gopls = Join-Path (go env GOPATH) "bin\gopls.exe"
}
$goFiles = git diff --name-only --diff-filter=ACMR | Where-Object { $_ -like "*.go" }
if ($goFiles) { & $gopls check @goFiles }
```

On POSIX shells:

```sh
command -v gopls >/dev/null 2>&1 || go install golang.org/x/tools/gopls@latest
go_files=$(git diff --name-only --diff-filter=ACMR -- '*.go')
test -z "$go_files" || "$(go env GOPATH)/bin/gopls" check $go_files
```

Treat `gopls`, compiler, and `go vet` errors as blockers.

### Dart and Flutter

Format changed Dart files and run analysis from each affected Dart or Flutter package:

```text
dart format <changed-files-or-directories>
dart analyze
```

Prefer `fvm dart` / `fvm flutter` when the project pins Flutter with FVM. For Flutter packages, also run `fvm flutter analyze` or `flutter analyze`. Analyzer errors and warnings are blockers; report pre-existing info-level lints separately.

## 5. Unit Test Gate

Write focused unit tests for the changed behavior. Cover success, failure, boundary, nullability, range, and platform-sensitive cases that apply.

Run the narrowest affected tests first, then the full unit suite:

```text
go test ./internal/<affected-package>
go test ./...
```

For Dart or Flutter logic, run the applicable suite:

```text
dart test
fvm flutter test
```

Do not weaken, skip, or delete existing assertions to get green results. Do not proceed to integration tests until all unit tests pass.

## 6. Integration Test Gate

After unit tests pass, validate the complete generation/build boundary rather than only inspecting strings.

For parser, generator, codec, type-mapping, or runtime changes:

1. Generate from a realistic Go package through the CLI:

```text
go run ./command generate --config-file <fixture>/flutter_go_bridge.yaml
```

2. Compile the generated Go package, using `-buildmode=c-shared` when FFI ABI behavior is involved:

```text
go build -buildvcs=false -buildmode=c-shared -o <output-library> .
```

3. Run `dart analyze` against the generated Dart package.
4. Regenerate a second time and rebuild to catch stale-output and idempotency problems.
5. Validate at least one real repository example such as `example/mihomoui` when the change can affect it.

For Flutter-facing changes, run the relevant `integration_test` flow on an available target. If an integration environment is unavailable, state the exact missing prerequisite; do not relabel a unit test as integration coverage.

Do not proceed to smoke tests until integration generation, compilation, and analysis pass.

## 7. Smoke Test Gate

Run an end-to-end smoke test with a real dynamic library and a real Dart process:

1. Generate the bridge.
2. Build the platform library (`.dll`, `.so`, or `.dylib`).
3. Initialize the generated Dart bridge with that library path.
4. Call representative public APIs from Dart.
5. Assert returned values, struct round-trips, errors, and the feature-specific behavior.

Typical Dart command:

```text
dart run bin/smoke.dart <dynamic-library-path>
```

If no smoke fixture exists, create a minimal one under ignored `build/` output. It must fail on a wrong value or missing exception, print an unambiguous success marker, and avoid modifying production data or external systems.

For type or codec features, include both bridge directions and at least one failure path. A successful compile without a Dart runtime call is integration coverage, not a smoke test.

## 8. Review and Commit

After every test gate passes:

1. Run `git diff --check`.
2. Review `git diff` and `git status --short`.
3. Confirm the current branch is the new task branch, never `main`.
4. Ensure build artifacts, DLLs, caches, and temporary smoke fixtures are ignored.
5. Stage only intended files.
6. Commit with a focused conventional message, for example:

```text
git commit -m "feat: support <behavior>"
git commit -m "fix: handle <failure>"
```

Push only when the user requests remote publication or the requested workflow explicitly includes it. When pushing a new branch:

```text
git push -u origin <branch-name>
```

Report the branch, commit, analysis commands, unit tests, integration tests, smoke result, and any residual platform coverage gaps.

## Failure Rules

- Stop at the failed gate, diagnose, fix, and rerun that gate before continuing.
- Re-read files after formatters, generators, package managers, or timed-out commands because they may have partially modified the worktree.
- Do not claim a test passed when it was skipped, timed out, or only compiled.
- Keep generated-code compile tests and runtime smoke tests separate in the final report.
