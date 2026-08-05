---
name: fgb-develop-feature
description: Develop, fix, document, or refactor flutter_go_bridge with change-aware validation and required documentation synchronization. Use for changes to the Go parser/generator/runtime, generated Dart API, CLI, integration templates, examples, docs, skills, or bridge behavior. Documentation-only changes (docs/, README, skills) run no code validation at all; code behavior changes and new features must update the development documentation and keep a git-ignored plan under .plans/; test and coverage follow-ups stay on the branch that owns the code.
---

# FGB Feature Development

Classify the final diff before selecting validation. Code changes follow the staged analysis and test
workflow. Documentation-only changes -- `docs/`, the README files, and Skill content -- run no code
validation at all.

A new feature or a behavior change also needs a git-ignored plan under `.plans/`, kept current as the
work proceeds. A bug fix does not.

## 0. Classify the Change

Classify from the files and behavior in the final diff, not only from the task title.

### Documentation-only

A change is documentation-only when every changed file is prose that ships no behavior:

- anything under `docs/`;
- `README.md`, `README.zh-CN.md`, and other repository-level Markdown such as
  `THIRD_PARTY_NOTICES.md`;
- `.agents/skills/**`, including a Skill's own metadata and resources.

Sections 4 through 7 do not apply. **Run no code validation at all**: no `gofmt`/`go vet`/`gopls`, no
`go test`, no coverage gate, no `dart analyze`, no generated-code compile, no integration run, and no
smoke run. There is nothing for them to prove, and a green code gate on a prose diff only adds noise
to the report.

Do instead:

- read the changed pages end to end for accuracy against the current behavior, not just for typos;
- keep English and Chinese pages in sync when both cover the changed area;
- check links and anchors that the change introduces or moves;
- run the docs site's own type-check/build when the change touches `docs/` and the tooling is
  available (this repository uses Bun: `bun run typecheck`, `bun run build`);
- for a Skill, review its instructions and frontmatter for internal consistency, and run
  Skill-specific validation only if it exists and works.

When a documentation tool is unavailable or deliberately disabled, say it was skipped. Never
substitute code tests for it.

### Code or product behavior

All other changes use the applicable analysis and test phases below. This includes parser,
generator, runtime, CLI, template, example, configuration, and generated API behavior changes.

Source comments, `example/**`, and `template/**` are **not** documentation-only: comments live in
compiled files, and examples and templates are inputs to generation. A diff touching them runs the
validation its area requires.

If a documentation-only task also changes code or product behavior, the whole diff loses the
exemption: run the code validation required by the affected area.

## 1. Prepare Git Before Editing

`main` is protected: never develop or commit on it. Everything else depends on whether the task
starts new work or continues work that is already on the current branch.

### Stay on the current branch

Do not create a branch when the task only supplements or repairs the verification of work that is
already on the current branch, for example:

- adding or extending tests for code this branch introduced;
- raising coverage that a patch-coverage gate flagged;
- fixing a test that this branch's changes made fail or made flaky;
- following up on review feedback about this branch's own commits.

Committing that work anywhere else would separate it from the change it validates. The only
exception is `main`: if the current branch is `main`, create a branch first even for a test-only
follow-up.

### Create a branch

Create a fresh branch from the latest remote `main` when the task starts new work: a new feature, a
behavior change, a bug fix in code the current branch does not own, or maintenance unrelated to the
current branch.

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

If work already exists before this workflow was applied, stop and preserve it before updating
`main`; do not risk losing changes merely to satisfy the branch sequence. When a fix depends on an
unmerged commit that only exists on the current branch, branching from `main` would leave nothing to
fix: stay on that branch and say so in the report.

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

### Plan a feature or a behavior change first

A new feature or a change to existing behavior needs a written plan before the first edit. A bug fix
does not: go straight to the implementation.

Write the plan to `.plans/<short-name>.md`. That directory is git-ignored on purpose — the plan is a
working aid for this task, not a deliverable, so it must never be staged or committed. Do not put it
under `docs/`, and do not add it to a commit "for context".

The plan lists the work as checkable items, each small enough to finish and verify on its own:

```markdown
# <short-name>

Goal: one or two sentences on the behavior being added or changed.

## Items

- [ ] 1. <the smallest change that stands on its own>
      Files: internal/generator/builder.go
      Done when: <the observable result, e.g. a named test passes>
- [ ] 2. ...

## Decisions

- <choice made, and the reason, so a later item does not relitigate it>

## Open questions

- <anything that needs the user's answer before a specific item can start>
```

Update the file as the work proceeds, not at the end:

- tick an item the moment it is finished and verified;
- record a decision when you make one, especially when you rejected an alternative;
- add an item you discovered rather than silently widening an existing one;
- when an item turns out to be wrong or unnecessary, strike it and say why.

The plan is also the handover: if the session ends mid-task, the next one must be able to read it and
continue without re-deriving the design.

### Implement

Implement the smallest complete change. Preserve user edits and avoid unrelated refactors.

When adding a feature or changing any code behavior, update the development documentation under
`docs/` in the same change. This requirement applies even when the implementation already has tests
or comments. Update both English and Chinese pages when both versions cover the changed area.

Document:

- supported syntax and types;
- generated Dart/Go behavior;
- limitations and ownership rules;
- migration or compatibility impact when existing behavior changes;
- a concise usage example when useful.

Choose the page that owns the behavior instead of appending unrelated notes to a convenient file.
Create a new page only when no existing page can explain the feature clearly. Update navigation or
cross-links when a new page is added.

Update `README.md` and `README.zh-CN.md` as well when the change affects installation, the quick start,
the high-level feature list, or other repository-front-page information. README updates do not replace
the required detailed development documentation under `docs/`.

Regenerate checked-in generated output when the repository treats it as source. Never hand-edit generated files when the generator is the source of truth.

## 4. Run Code Analysis

Skip this section for a documentation-only diff.

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

Skip this section for a documentation-only diff.

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

After the unit tests pass, measure aggregate Go statement coverage across every Go package except
`cmd` and `template` and all of their subpackages. Store the profile under ignored build output, not
next to source files.

On PowerShell:

```powershell
$packages = go list ./... | Where-Object {
  $_ -notmatch '/cmd(?:/|$)' -and $_ -notmatch '/template(?:/|$)'
}
$profile = Join-Path (Resolve-Path build) 'coverage.out'
& go test '-count=1' '-coverprofile' $profile @packages
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& go tool cover '-func' $profile
```

On POSIX shells:

```sh
packages=$(go list ./... | rg -v '/(cmd|template)(/|$)')
go test -count=1 -coverprofile=build/coverage.out $packages
go tool cover -func=build/coverage.out
```

The final `total:` statement coverage must be at least 95.0%. Coverage below 95.0%, a missing
profile, or a failed package test is a blocking failure: add meaningful tests and rerun the gate.
Never weaken, skip, or delete existing assertions merely to get green results or inflate coverage.
Do not proceed to integration tests, smoke tests, review, or commit until all unit tests pass and this
coverage threshold is met.

## 6. Integration Test Gate

Skip this section for a documentation-only diff.

After unit tests pass, validate the complete generation/build boundary rather than only inspecting strings.

For parser, generator, codec, type-mapping, or runtime changes:

1. Generate from a realistic Go package through the CLI:

```text
go run ./cmd/flutter_go_bridge_codegen generate --config-file <fixture>/flutter_go_bridge.yaml
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

Skip this section for a documentation-only diff.

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

After every applicable validation phase passes:

1. Run `git diff --check`.
2. Review `git diff` and `git status --short`.
3. Confirm the current branch is correct: a task branch for new work, the existing branch for a
   test or coverage follow-up, and never `main`.
4. For code changes, confirm the coverage gate excluded `cmd/**` and `template/**` and reported at
   least 95.0% total statement coverage.
5. Ensure build artifacts, DLLs, caches, temporary smoke fixtures, and `.plans/**` are ignored.
6. Stage only intended files. A feature plan is never one of them; check that `git status --short`
   shows no `.plans/` entry before staging.
7. Commit with a focused conventional message, for example:

```text
git commit -m "feat: support <behavior>"
git commit -m "fix: handle <failure>"
```

Push only when the user requests remote publication or the requested workflow explicitly includes it. When pushing a new branch:

```text
git push -u origin <branch-name>
```

Report the branch and commit when created, and say which branch rule applied when the work stayed on
an existing branch. For code changes, report analysis commands, unit tests, the exact aggregate
coverage percentage and exclusions, integration tests, smoke results, documentation updates, and
residual platform coverage gaps. For a feature or behavior change, report the plan path and which
items are now ticked. For a documentation-only change, state plainly that this workflow required no
code validation, and list only the documentation or Skill checks actually performed.

## Failure Rules

- Stop at the failed gate, diagnose, fix, and rerun that gate before continuing.
- Re-read files after formatters, generators, package managers, or timed-out commands because they may have partially modified the worktree.
- Do not claim a test passed when it was skipped, timed out, or only compiled.
- Keep generated-code compile tests and runtime smoke tests separate in the final report.
- Do not use the documentation-only exemption when the same diff changes code, templates,
  examples, configuration behavior, or generated API behavior.
- Do not finish a code behavior change or new feature without updating its owning development
  documentation under `docs/`.
- Do not commit a plan file, and do not move it out of `.plans/` to keep it.
- Do not start a feature or behavior change without its plan, and do not leave the plan behind the
  implementation: an item ticked before it is verified is worse than no plan at all.
- Do not branch away from work this branch owns. A test, coverage, or flake follow-up belongs on the
  branch that introduced the code it verifies.
