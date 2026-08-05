---
name: fgb-develop-feature
description: Develop, fix, document, or refactor flutter_go_bridge with change-aware validation and required documentation synchronization. Use for changes to the Go parser/generator/runtime, generated Dart API, CLI, integration templates, examples, docs, skills, or bridge behavior. Documentation-only changes (docs/, README, skills) run no code validation at all; code behavior changes and new features must update the development documentation and keep a git-ignored plan under .plans/. A branch owns one independent feature and commits each finished item locally as a rollback checkpoint; multiple independent features use separate branches and are published individually, never batched into one branch. After the final commit, keep a local completion record under .plans/ as merge-conflict context and completion proof; merge-conflict resolutions must use the code-review skill. Test and coverage follow-ups stay on the branch that owns the code.
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

One branch owns one independent feature or behavior change. If the incoming request contains
multiple independent features, split them before editing: create one branch per feature, with its
own plan, checkpoints, validation, completion record, and pull request. Do not combine the features
onto one branch and do not wait for all of them to finish before publishing one of them.

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

The plan lists the work as checkable items, each small enough to finish, verify, and commit on its
own:

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

- tick an item the moment it is finished, verified, and committed, and record its commit hash;
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

### Checkpoint each finished item with a local commit

A branch that carries several changes must not carry them all as one uncommitted pile. Commit to the
local repository as soon as a piece of work is finished and verified, so a later edit that goes wrong
cannot damage what already worked and the branch can be returned to a known-good state.

This applies to every branch that accumulates more than one separable change, plan or no plan:

- with a plan, each item is a checkpoint: tick it and commit it in the same step;
- without a plan, checkpoint whenever the work reaches a self-contained verified state -- the fix
  itself, then its regression test, then a follow-up correction;
- a change that is only meaningful together with the next one stays uncommitted until both are done.
  Never create a checkpoint that cannot compile on its own.

Before each checkpoint commit, run the validation that is narrow enough to be worth repeating per
item:

1. analysis on the files this item changed (Section 4);
2. the item's own `Done when` test and the tests of the packages it touched (Section 5).

On a documentation-only branch the per-item check is the documentation review from Section 0 instead;
committing prose in steps never justifies running a code gate.

A checkpoint must build, and must not knowingly break a test that passed before it. The expensive
gates -- the full unit suite with the coverage threshold, integration, and smoke -- still run over
the finished branch in Sections 5 through 7, not once per item; the fixes they force become their own
checkpoints.

Commit each checkpoint with the conventional message its own change deserves, so the history reads as
the sequence of items rather than one squashed lump. Stage only that item's files: `.plans/**` is
ignored and must never appear in a checkpoint. Commit locally only -- the remote for that branch is
published independently under Section 9, not once for a batch of unrelated feature branches.

Record the checkpoint next to the item it finished, so the rollback target survives the session:

```markdown
- [x] 1. <the smallest change that stands on its own>
      Files: internal/generator/builder.go
      Done when: TestBuilderRejectsUnsupportedType passes
      Commit: 1a2b3c4
```

### Roll back to a checkpoint

When a later item breaks earlier work, return to the last good checkpoint instead of debugging a
worktree that mixes several states:

```text
git diff <sha>                       # what changed since the checkpoint
git restore --source <sha> -- <path> # take specific files back to the checkpoint
git revert <sha>                     # undo an already committed item, keeping history
```

`git reset --hard`, `git clean -f`, and anything else that discards commits or uncommitted work
require the user's explicit permission first. Report which checkpoint you returned to, what was
undone, and whether the plan item went back to unticked.

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
Do not proceed to integration tests, smoke tests, final review, or publication until all unit tests
pass and this coverage threshold is met. A per-item checkpoint commit from Section 3 is not blocked by
this gate -- it carries its own narrow verification and never leaves the local repository.

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
2. Review the branch as a whole, not only what is still uncommitted:

```text
git status --short
git diff
git log --oneline origin/main..HEAD
git diff origin/main...HEAD
```

3. Confirm the current branch is correct: a task branch for new work, the existing branch for a
   test or coverage follow-up, and never `main`.
4. For code changes, confirm the coverage gate excluded `cmd/**` and `template/**` and reported at
   least 95.0% total statement coverage.
5. Ensure build artifacts, DLLs, caches, temporary smoke fixtures, and `.plans/**` are ignored.
6. Stage only intended files. A feature plan or completion record is never one of them; check that
   `git status --short` shows no `.plans/` entry before staging.
7. Commit whatever remains with a focused conventional message, for example:

```text
git commit -m "feat: support <behavior>"
git commit -m "fix: handle <failure>"
```

The worktree must be clean when this section finishes: no finished work left uncommitted. Prefer a
new commit over `git commit --amend`, and never rewrite a checkpoint that already validated, so each
item stays a reachable rollback target.

After the final commit, write the local completion record to `.plans/completed/<short-name>.md`
before publishing the branch. It is git-ignored and must stay untracked. Include the branch name,
the final commit hash, a saved copy of the final diff/patch, the changed files and behaviors, the
validation that ran, and any notes that will help resolve a later merge conflict. This record is the
completion proof and the local context for the `code-review` step in Section 9.

## 9. Publish the Branch

Publish each feature branch independently when its own task is actually finished: every plan item
ticked or struck with a reason, every applicable gate in Sections 4 through 7 green, the
documentation updated, the completion record written to `.plans/completed/`, and the worktree clean.
In a multi-feature request, do not wait for the other branches and do not create an integration
branch just to publish them together. A mid-task checkpoint is never published, and `main` is never
pushed.

Once that holds, push the branch to the remote without waiting to be asked -- the only exception is a
user who asked to keep the work local:

```text
git push -u origin <branch-name>   # first push of a new branch
git push                           # branch already tracks a remote
```

If the push is rejected as non-fast-forward, fetch and rebase or merge `origin/main`, rerun the gates
the rebase could invalidate, then push again. `git push --force` and `--force-with-lease` need the
user's explicit permission.

If the remote rejects the push for access reasons -- read-only clone, or a fork without write
permission -- stop pushing, say so, and still write the pull request document below so the work is not
lost.

### Merge conflicts and code review

When merging the branch into `main` produces conflicts, resolve them locally and then call the
`code-review` skill before completing the merge. Use the same fixed point the pull request was
reviewed against, normally `origin/main`, and use the `.plans/completed/<short-name>.md` record as
the local reference for what this branch changed. Address the review findings in new commits, rerun
the applicable gates if the conflict resolution touched code, and rerun `code-review` on the resolved
merge before finishing.

### Open the pull request with `gh`

Check for a usable GitHub CLI: it must exist *and* be authenticated.

```powershell
$gh = Get-Command gh -ErrorAction SilentlyContinue
if ($gh) { gh auth status }
```

```sh
command -v gh >/dev/null 2>&1 && gh auth status
```

When both succeed, write the body to `.plans/pr-<short-name>.md` and open the pull request:

```text
gh pr create --base main --head <branch-name> --title "<conventional title>" --body-file .plans/pr-<short-name>.md
```

Keep the title under about 70 characters and put the detail in the body. Add `--draft` when a gate was
skipped for a missing prerequisite, and name that gate in the body. Report the URL `gh pr create`
prints. Do not open a second pull request for a branch that already has one: `gh pr view` first, and
push the new commits to update the existing one instead.

### Fall back to a pull request document

When `gh` is missing, unauthenticated, or fails, write the same body to `.plans/pr-<short-name>.md` and
stop there. That document is the handover: a developer must be able to open the pull request from it by
hand, pasting the title and body unchanged. Give it a `Title:` line at the top, then the body.

`.plans/**` is git-ignored, so the document is never staged and never committed. Report its path and
the compare URL the developer can open:

```text
https://github.com/<owner>/<repo>/compare/main...<branch-name>?expand=1
```

Derive `<owner>/<repo>` from `git remote get-url origin` rather than guessing it.

### Pull request body

```markdown
## Summary

<what changed and why, in two or three sentences>

## Changes

- <area>: <change>

## Validation

- Analysis: <commands run, or "not required: documentation-only">
- Unit tests and coverage: <commands, exact total statement coverage, exclusions>
- Integration: <what ran, or the exact missing prerequisite>
- Smoke: <what ran, or the exact missing prerequisite>

## Documentation

- <pages updated, English and Chinese>

## Checkpoints

- <sha> <subject>

## Risks and rollback

- <what could regress, and the checkpoint to return to>
```

Report a gate that did not run as not run, together with the prerequisite that was missing. A pull
request that claims validation it never performed is worse than one that admits the gap.

## 10. Report

Report the branch and commit when created, and say which branch rule applied when the work stayed on
an existing branch. When the branch carries several changes, list its checkpoint commits in order so
the user can see what is already safe to roll back to. For code changes, report analysis commands,
unit tests, the exact aggregate coverage percentage and exclusions, integration tests, smoke results,
documentation updates, and residual platform coverage gaps. For a feature or behavior change, report
the plan path, the `.plans/completed/` completion record path, and which items are now ticked. For a
documentation-only change, state plainly that this workflow required no code validation, and list only
the documentation or Skill checks actually performed.

Close with the publication outcome: the pushed branch and its pull request URL, or the pull request
document path plus the compare URL when `gh` was unavailable, or the reason nothing was published.

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
- Do not commit a completion record: keep it under `.plans/completed/` as local, untracked proof of
  the branch's finished work.
- Do not start a feature or behavior change without its plan, and do not leave the plan behind the
  implementation: an item ticked before it is verified is worse than no plan at all.
- Do not branch away from work this branch owns. A test, coverage, or flake follow-up belongs on the
  branch that introduced the code it verifies.
- Do not combine multiple independent features into one branch or publish them as one batch. Each
  feature branch is published when its own gates pass.
- Do not let several finished items pile up as one uncommitted change. Work that was never committed
  has no checkpoint to roll back to.
- Do not commit a checkpoint that fails to build, or that knowingly breaks a test that passed before
  it. Fix it first, or leave the item open.
- Do not discard commits or uncommitted work to recover from a bad item without the user's explicit
  permission, and do not rewrite a checkpoint that already validated.
- Do not publish an unfinished branch: pushing before the applicable gates are green turns a private
  mistake into a public one. Force pushing needs the user's explicit permission.
- Do not complete a merge-conflict resolution without invoking `code-review` on the resolved merge.
- Do not finish a completed branch without publishing it -- pushed with a pull request when `gh` works,
  or pushed with a pull request document under `.plans/` when it does not. If neither was possible, say
  why.
- Do not open a pull request that claims a gate which was skipped, and do not commit the pull request
  document.
