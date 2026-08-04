# Errors

## [ERR-20260803-001] resumed-exec-cell-expired

**Logged**: 2026-08-03T01:48:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

A running coverage command could not be resumed after the user continued the task across an environment-context refresh because its execution cell had expired.

### Error

```text
exec cell 111 not found
```

### Context

- Operation: resume the aggregate Go coverage gate after a long-running tool call.
- The continuation crossed into a refreshed environment context and the prior cell registry was no longer available.

### Suggested Fix

When a continuation cannot find a previously running cell, rerun the idempotent gate command and do not infer pass or failure from the missing cell.

### Metadata

- Reproducible: unknown
- Related Files: build/coverage.out

### Resolution

- **Resolved**: 2026-08-03T01:49:00+08:00
- **Notes**: Reran the complete coverage gate and used only the new command's reported result.

---

## [ERR-20260802-014] generator-test-polluted-worktree

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary

`TestGenerateMainPackageDirect` omitted `DartOutput`, so full tests overwrote tracked Dart files in the generator package.

### Error

```text
M internal/generator/bridge_generated.dart
M internal/generator/main.dart
git diff --check: new blank line at EOF
```

### Context

- Operation: final worktree review after the full suite and coverage gate.
- An empty configured Dart output resolves to relative `bridge_generated.dart`, and mirrored `main.go` resolves to `main.dart` in the test process working directory.
- A composed review command continued after `git diff --check`, so its later successful diff output masked the nonzero check status.

### Suggested Fix

Every successful generation test must route all outputs into `t.TempDir()`. Run `git diff --check` as an authoritative standalone gate.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/generate_err_test.go, internal/generator/bridge_generated.dart, internal/generator/main.dart

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: Added a temporary Dart output path, restored the overwritten tracked files, and reran the standalone diff check.

---

## [ERR-20260802-015] shell-command-explicit-workdir

**Logged**: 2026-08-02T19:54:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

The first PowerShell command failed before execution when the tool rejected the repository's explicit working-directory argument, even though the session was already located in that directory.

### Error

```text
execution error: Io(Os { code: 267, kind: NotADirectory, message: "目录名称无效。" })
```

### Context

- Operation: read the feature-development and self-improvement Skill files.
- Explicit working directory: `D:\Projects\a\flutter-go-bridge-gokit`.
- A following `Get-Location` confirmed that the session current directory was the same valid repository path.

### Suggested Fix

When the session is already rooted in the repository, omit the redundant explicit `workdir`; if an explicit directory is required, verify it with a separate read-only location probe first.

### Metadata

- Reproducible: no
- Related Files: .agents/skills/fgb-develop-feature/SKILL.md, .agents/skills/self-improvement/SKILL.md
- See Also: ERR-20260802-006

### Resolution

- **Resolved**: 2026-08-02T19:54:00+08:00
- **Notes**: Retried from the session current directory and both Skill files were read successfully.

---

## [ERR-20260802-016] clash-ui-go-module-root

**Logged**: 2026-08-02T20:04:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

A combined example-project probe assumed `D:\Projects\clash_ui` was the Go module root, so reading `go.mod` and running `go doc` failed even though the Flutter project and bridge config were present there.

### Error

```text
Cannot find path 'D:\Projects\clash_ui\go.mod' because it does not exist.
doc: no required module provides package github.com/metacubex/mihomo/constant.Proxy
```

### Context

- Operation: inspect the real `constant.Proxy` interface and likely implementation before reproducing generation.
- The bridge config is at the Flutter project root, while the Go module is nested under `D:\Projects\clash_ui\go`.

### Suggested Fix

Discover `go.mod` with `rg --files` before module-aware commands, then run `go doc`, `go test`, and `go build` from the discovered module directory.

### Metadata

- Reproducible: yes
- Related Files: D:\Projects\clash_ui\flutter_go_bridge.yaml, D:\Projects\clash_ui\go\go.mod
- See Also: ERR-20260802-006, ERR-20260802-MOD01

### Resolution

- **Resolved**: 2026-08-02T20:05:00+08:00
- **Notes**: Located the nested module and reran the interface documentation probes from `D:\Projects\clash_ui\go`.

---

## [ERR-20260802-017] exec-javascript-quote-boundary

**Logged**: 2026-08-02T20:48:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

A read-only source-location command never reached PowerShell because unescaped double quotes inside the JavaScript command string terminated it early.

### Error

```text
SyntaxError: Unexpected identifier 'internal'
```

### Context

- Operation: inspect generated Dart interface decoders and CST definition helpers after analyzer errors.
- The JavaScript wrapper and embedded PowerShell ripgrep patterns both used double quotes.

### Suggested Fix

Use single-quoted ripgrep patterns inside JavaScript double-quoted PowerShell command strings, or split complex inspection commands before execution.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/cst_wire.go, internal/generator/render_dart_split.go

### Resolution

- **Resolved**: 2026-08-02T20:49:00+08:00
- **Notes**: Reissued the inspection with single-quoted ripgrep patterns.

---

## [ERR-20260802-018] external-interface-dart-analysis

**Logged**: 2026-08-02T20:47:00+08:00
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary

The first real-project analyzer run found nullable concrete interface decoder returns and standard-codec-only types leaking empty CST `ffi.Struct` declarations.

### Error

```text
A value of type 'AdapterProxy?' can't be returned from a function with return type 'Proxy'.
The class '_FgbCstType72' can't be empty because it's a subclass of 'Struct'.
```

### Context

- Operation: analyze generated output for `D:\Projects\clash_ui\go\api\proxies-test.go`.
- Go DLL compilation passed, but Dart type narrowing and CST reachability were incomplete for newly discovered interface implementations.

### Suggested Fix

Reject a null implementation payload before returning it as a non-null interface, and emit CST definitions only for types reachable from calls that actually select CST/DCO.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/render_dart_split.go, internal/generator/cst_wire.go, internal/generator/generator_test.go

### Resolution

- **Resolved**: 2026-08-02T20:53:00+08:00
- **Notes**: Added non-null interface decoder narrowing, CST call reachability filtering, and focused regression assertions; static analysis, full tests, and the 95.0% coverage gate passed afterward.

---

## [ERR-20260802-019] powershell-finally-masked-native-exit

**Logged**: 2026-08-02T21:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary

`Pop-Location` in a PowerShell `finally` block overwrote a failed native `go build` exit code, so the command wrapper reported success while the build output contained cgo errors.

### Error

```text
could not determine what C.FgbCstType3 refers to
Exit code: 0
```

### Context

- Operation: compile the generated clash_ui bridge after changing CST reachability.
- The script used `try { go build ... } finally { Pop-Location }` without saving `$LASTEXITCODE`.

### Suggested Fix

Save `$LASTEXITCODE` immediately after native commands, leave the directory, then `exit` with the saved code. Prefer a tool `workdir` when it is reliable.

### Metadata

- Reproducible: yes
- Related Files: D:\Projects\clash_ui\go\bridge_generated.go
- See Also: ERR-20260802-COV01

### Resolution

- **Resolved**: 2026-08-02T21:02:00+08:00
- **Notes**: All following module-directory commands preserve and restore the native exit code explicitly.

---

## [ERR-20260802-020] source-edit-used-set-content

**Logged**: 2026-08-02T21:03:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: infra

### Summary

A mechanical helper rename used PowerShell `Set-Content` on three Go source files instead of the required `apply_patch` editing path.

### Error

```text
Repository source edits must use apply_patch.
```

### Context

- Operation: rename `cstDefinitionTypes` to `cstDcoReachableTypes`.
- The replacement was limited to a single identifier and did not change encoding according to `git diff --check` and `git diff --numstat`.

### Suggested Fix

Use `apply_patch` even for small mechanical source changes; reserve formatters for whitespace-only rewrites.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/cst_wire.go, internal/generator/render_dart_cst.go, internal/generator/render_go_cst.go

### Resolution

- **Resolved**: 2026-08-02T21:04:00+08:00
- **Notes**: Verified the narrow diff, applied the subsequent comment correction through `apply_patch`, ran gofmt, and passed focused tests.

---

## [ERR-20260802-013] smoke-patch-after-format

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

The first smoke simplification patch used the pre-format source layout after `dart format` had rewritten the file.

### Error

```text
apply_patch verification failed: Failed to find expected lines in D:\Projects\clash_ui\build\dart_analyze_smoke\smoke.dart
```

### Context

- Operation: remove the nonterminating stream call from the smoke fixture.
- The formatter had wrapped the error and output strings since the file was created.

### Suggested Fix

Re-read generated or formatted files before applying follow-up patches, as required by the feature-development workflow.

### Metadata

- Reproducible: no
- Related Files: D:\Projects\clash_ui\build\dart_analyze_smoke\smoke.dart

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: Reapplied the simplification against the formatted source.

---

## [ERR-20260802-012] nonterminating-stream-smoke

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary

The first real-project smoke test used an intentionally nonterminating Go ticker stream and did not exit within the command timeout.

### Error

```text
command timed out after 124023 milliseconds
```

### Context

- Operation: initialize the generated DLL, call `getMihomoVersion()`, then await `streamTime(...).first`.
- `go/api/timer.go` loops forever and has no context or cancellation branch.
- The timed-out Dart process tree was identified by the smoke script command line and terminated.

### Suggested Fix

Use a terminating public call for this analyzer-focused smoke. Test the infinite stream separately only after the API exposes a cancellation-aware implementation.

### Metadata

- Reproducible: yes
- Related Files: D:\Projects\clash_ui\go\api\timer.go, D:\Projects\clash_ui\build\dart_analyze_smoke\smoke.dart

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: The smoke now validates DLL initialization and the synchronous structured `getMihomoVersion()` CST/DCO call.

---

## [ERR-20260802-011] parser-ordering-patch-context

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

The first deterministic-ordering patch assumed a different import block shape in `parser_extra_test.go`.

### Error

```text
apply_patch verification failed: Failed to find expected lines in internal/parser/parser_extra_test.go
```

### Context

- Operation: add stable cross-file position ordering and its regression test.
- The combined patch was rejected before changing either file.

### Suggested Fix

Patch production code and tests separately after reading the exact test imports.

### Metadata

- Reproducible: no
- Related Files: internal/parser/parser.go, internal/parser/parser_extra_test.go

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: Applied the source and test patches against exact contexts.

---

## [ERR-20260802-010] generated-output-non-idempotent

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: backend

### Summary

Back-to-back generation of the real `clash_ui` API changes the generated Go bridge and every generated Dart API file.

### Error

```text
NON_IDEMPOTENT
D:\Projects\clash_ui\go\bridge_generated.go
D:\Projects\clash_ui\lib\src\bridge_generated.dart
D:\Projects\clash_ui\lib\src\_generated.dart
D:\Projects\clash_ui\lib\src\api\*.dart
```

### Context

- Operation: required second-generation hash gate after analyzer cleanup.
- `go/internal/fgb/fgb_generated.go` remained stable, while model-dependent output changed.
- This suggests unstable parser or generator ordering rather than formatter-only drift.

### Suggested Fix

Diff consecutive snapshots, identify the unordered declaration/type source, sort it before assigning generated IDs, and add a deterministic-generation regression test.

### Metadata

- Reproducible: yes
- Related Files: internal/parser/, internal/generator/, D:\Projects\clash_ui\go\api\

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: Parser ordering now sorts by normalized source filename before within-file token position. Two consecutive real-project generations produced identical hashes for all 18 generated files.

---

## [ERR-20260802-009] idempotency-hash-wrong-workdir

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary

The first idempotency hash check resolved generated-project paths against the generator repository and printed a false success marker.

### Error

```text
Get-ChildItem: Cannot find path '...flutter-go-bridge-gokit\lib\'
Resolve-Path: Cannot find path '...flutter-go-bridge-gokit\go\bridge_generated.go'
IDEMPOTENT_GENERATION_OK
```

### Context

- Operation: compare `D:\Projects\clash_ui` generated file hashes across a second generation.
- Relative paths were evaluated from the generator repository working directory.
- PowerShell path errors were non-terminating, so the empty comparison set reached the success marker.

### Suggested Fix

Build an explicit absolute file list under the generated project, set `ErrorActionPreference = 'Stop'`, and reject an empty path set before comparing hashes.

### Metadata

- Reproducible: yes
- Related Files: D:\Projects\clash_ui\lib\src\, D:\Projects\clash_ui\go\bridge_generated.go

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: The idempotency gate is rerun with absolute paths and terminating errors.

---

## [ERR-20260802-008] lint-doc-test-patch-context

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

A combined test-and-documentation patch used a guessed test function as its insertion context.

### Error

```text
apply_patch verification failed: Failed to find expected lines in internal/generator/render_split_coverage_test.go
```

### Context

- Operation: add escaping coverage and bilingual analyzer-output documentation.
- The guessed `TestSourceDartPathFallbacks` function does not exist in the coverage test file.

### Suggested Fix

Read the target test file and anchor additions to an existing function before applying multi-file patches.

### Metadata

- Reproducible: no
- Related Files: internal/generator/render_split_coverage_test.go, docs/guide/output-structure.md

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: Reapplied the change against the actual final test function.

---

## [ERR-20260802-007] combined-repository-scan-assumed-example-directory

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

A combined repository scan returned a failing status because it included the nonexistent `example/` directory.

### Error

```text
rg: example: The system cannot find the file specified. (os error 2)
```

### Context

- Operation: discover analyzer and generation commands before diagnosing generated Dart output.
- The useful searches completed, but the guessed optional path made the combined probe fail.

### Suggested Fix

Use `rg --files` to discover repository directories first, and search only paths that exist.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/, docs/
- See Also: ERR-20260802-004, ERR-20260802-001

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: Subsequent inspection uses the discovered repository paths.

---

## [ERR-20260802-005] generated-stream-rollback-assertion

**Logged**: 2026-08-02T23:14:00+09:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

The first regression assertion expected the renderer's pre-format single-line control flow instead of the generated `gofmt` output.

### Error

```text
Go bridge missing "if err != nil { transfer.rollback(); continue }"
```

### Context

- The renderer emits a compact line, but generated Go is formatted before the fixture returns it to the test.
- `gofmt` expands the statement into a multiline `if` body.

### Suggested Fix

Assert the distinctive formatted sequence containing `transfer.rollback()` followed by `continue`.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/features_test.go, internal/generator/render_go.go

### Resolution

- **Resolved**: 2026-08-02T23:14:00+09:00
- **Notes**: Updated the assertion to match the formatted generated source.

---

## [ERR-20260802-COV01] powershell-go-cover-arguments

**Logged**: 2026-08-02T11:10:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary
An unquoted PowerShell `go test` coverage argument produced a spurious `.out` package.

### Error
```text
FAIL .out [setup failed]
no required module provides package .out
```

### Context
- Operation: aggregate coverage for all packages except `cmd/**` and `template/**`.
- The command used `go test -count=1 -coverprofile=build/coverage.out $packages` instead of the repository skill's explicitly quoted native arguments.
- Every real package completed, but the aggregate command correctly remained failed.

### Suggested Fix
Pass PowerShell native arguments separately and quoted: `& go test '-count=1' '-coverprofile' $profile @packages`.

### Metadata
- Reproducible: yes
- Related Files: build/coverage.out

### Resolution
- **Resolved**: 2026-08-02T11:10:00+08:00
- **Notes**: The coverage gate will be rerun with the documented PowerShell invocation.

---

## [ERR-20260802-RG01] powershell-rg-glob

**Logged**: 2026-08-02T11:05:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
PowerShell passed a wildcard path literally to `rg`, causing a Windows path syntax error.

### Error
```text
rg: internal/generator/*_test.go: IO error for operation on internal/generator/*_test.go
```

### Context
- Operation: parallel search of generator tests for `time.Time` coverage.
- Command used `rg ... internal/generator/*_test.go` from PowerShell.

### Suggested Fix
Search the directory directly (`rg ... internal/generator`) or let PowerShell expand files before invoking `rg`.

### Metadata
- Reproducible: yes
- Related Files: internal/generator/*_test.go

### Resolution
- **Resolved**: 2026-08-02T11:05:00+08:00
- **Notes**: Re-running searches against the directory avoids shell wildcard handling.

---

## [ERR-20260802-004] audit-round2-example-path

**Logged**: 2026-08-02T23:06:00+09:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

The prescribed repository-example validation path does not exist in this checkout.

### Error

```text
rg: example: IO error for operation on example: The system cannot find the file specified. (os error 2)
```

### Context

- The integration gate calls for a maintained example such as `example/mihomoui`.
- This repository has no `example/` directory and no additional `flutter_go_bridge.yaml` beyond the shared template; the dedicated generated smoke module is therefore the available end-to-end coverage.

### Suggested Fix

Keep the generated smoke fixture as the validation target, or add a maintained repository example when product-level example coverage is required.

### Metadata

- Reproducible: yes
- Related Files: build/audit_round2_smoke/

### Resolution

- **Resolved**: 2026-08-02T23:06:00+09:00
- **Notes**: Located all candidate configurations and confirmed the repository does not include the expected example tree.

---

## [ERR-20260802-003] audit-round2-coverage-gate

**Logged**: 2026-08-02T12:20:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary

The first aggregate coverage run after the N15 implementation fell below the required threshold.

### Error

```text
total: (statements) 94.4%
required: at least 95.0%
```

### Context

- Packages under `cmd/**` and `template/**` were correctly excluded.
- New recursive atomic classification and synthetic opaque naming branches expanded production code beyond the main-path regression fixtures.
- A later simplification run stopped at compile time because the accompanying recursion test retained an unused local `builder`; it was removed before rerunning the gate.
- A follow-up PowerShell coverage scan used `"*$p:*"`; PowerShell parsed `$p:` as a scoped variable and rejected the command. The scan was rerun with regex matching.
- A proposed `os.ReadDir(file)` error test was not portable: Windows classifies `ERROR_DIRECTORY` as `fs.ErrNotExist`, which the idempotent cleanup intentionally ignores. The test was removed instead of making platform error mapping part of the contract.

### Suggested Fix

Add meaningful generated-module tests for synthetic-name collisions, external atomic wrappers, pointer-element collections, nested collection shapes, and rejection boundaries; remove the now-unused `containsAtomic` helper.

### Metadata

- Reproducible: yes
- Recurrence-Count: 4
- Related Files: internal/generator/builder.go, internal/generator/model.go, internal/generator/features_test.go

### Resolution

- **Resolved**: 2026-08-02T23:11:00+09:00
- **Notes**: Added meaningful generated-module coverage for atomic collection shapes and collision handling; the prescribed aggregate coverage gate now reports 95.0% after excluding `cmd/**` and `template/**`.

---

## [ERR-20260802-002] audit-round2-first-build

**Logged**: 2026-08-02T10:37:00+08:00
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary

The first narrow test run after the round-two runtime patch found one missing import and stale creator expectations.

### Error

```text
internal/generator/render_go.go:773:18: undefined: bridgemodel
removeFilesInDir on a missing dir should error
```

### Context

- The renderer used `bridgemodel.CallModeAsync` without importing the internal model package.
- Creator tests encoded the old behavior that missing optional Flutter scaffold directories are errors, which N10 intentionally changes.

### Suggested Fix

Import the model package, make callback CST generation avoid synthetic `err` declarations, and preserve creator error coverage with real non-`ErrNotExist` filesystem failures.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/render_go.go, internal/generator/render_go_cst.go, internal/integrate/coverage_extra_test.go

### Resolution

- **Resolved**: 2026-08-02T10:40:00+08:00
- **Notes**: Applied the missing import and updated tests to the new idempotent cleanup contract.

---

## [ERR-20260802-001] generator-source-scan-missing-file

**Logged**: 2026-08-02T10:15:00+08:00
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary

A generator source scan failed because it included a guessed file that does not exist.

### Error

```text
rg: internal/generator/render_go_dispatch.go: The system cannot find the file specified. (os error 2)
```

### Context

- Operation: locate callback context and async dispatch generation code while implementing the round-two audit fixes.
- The dispatch renderer is implemented in `internal/generator/render_go.go`, not a separate `render_go_dispatch.go` file.
- A second directory scan repeated the same mistake with `render_go_dco.go`; DCO rendering lives in `render_go_cst.go`.

### Suggested Fix

Use `rg --files internal/generator` before naming an uncertain source file, or search the directory with a file glob.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/render_go.go
- Recurrence-Count: 2

### Resolution

- **Resolved**: 2026-08-02T10:15:00+08:00
- **Notes**: Continued with directory-scoped searches and the existing renderer file.

---

## [ERR-20260801-007] git-global-ignore-sandbox-warning

**Logged**: 2026-08-01T00:00:00+09:00
**Priority**: low
**Status**: pending
**Area**: infra

### Summary

Git repository inspection succeeded but repeatedly warned that the restricted sandbox could not read the user-level ignore file.

### Error

```text
warning: unable to access 'C:\Users\Administrator/.config/git/ignore': Permission denied
```

### Context

- Operation: inspect branch, status, and recent commits before code review.
- Repository-local Git data remained readable and the commands exited successfully.

### Suggested Fix

For sandboxed repository inspection, override `core.excludesFile` with a readable path or accept the warning when global ignore behavior is irrelevant.

### Metadata

- Reproducible: yes under the restricted permission profile
- Related Files: .git/

---

## [ERR-20260801-004] audit-smoke-working-directory

**Logged**: 2026-08-01T12:45:00+09:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

The first integration-smoke commands used the fixture root for repository build paths and Dart bin paths.

### Error

```text
stat ...\build\audit_smoke\cmd\flutter_go_bridge_codegen: directory not found
Could not find file `bin\smoke.dart`
```

### Context

- The codegen binary belongs to the repository module, while the smoke script belongs to the nested Dart package.

### Suggested Fix

Build codegen from the repository root, run generation/Go validation from the fixture root, and run Dart commands from the Dart package root.

### Metadata

- Reproducible: yes
- Related Files: build/audit_smoke/ (temporary)
- Recurrence-Count: 2
- Last-Seen: 2026-08-01

### Resolution

- **Resolved**: 2026-08-01T12:50:00+09:00
- **Notes**: The corrected flow passed two generations, Go vet, Dart analyze, DLL build, and runtime smoke.

---

## [ERR-20260801-006] powershell-go-coverprofile-argument

**Logged**: 2026-08-01T19:20:00+09:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

PowerShell passed the inline relative `go test -coverprofile=build/coverage.out` value incorrectly,
causing Go to treat `.out` as an extra package.

### Error

```text
FAIL .out [setup failed]
no required module provides package .out
too many arguments
```

### Context

- Operation: aggregate coverage across all packages except `cmd/**` and `template/**`.
- All intended package tests passed, but the malformed invocation made the coverage gate fail.
- The same inline-value form also broke `go tool cover -func=build/coverage.out` under PowerShell.

### Suggested Fix

Resolve the build directory, pass `-coverprofile`/`-func` and the absolute profile path as separate
native command arguments, and check `go tool cover` only after `go test` succeeds.

### Metadata

- Reproducible: yes
- Related Files: .agents/skills/fgb-develop-feature/SKILL.md

### Resolution

- **Resolved**: 2026-08-01T19:21:00+09:00
- **Notes**: Updated the skill command and reran the gate successfully at 95.1% total coverage.

---

## [ERR-20260801-003] go-test-sandbox-cache

**Logged**: 2026-08-01T11:00:00+09:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

The initial Go test could not write the default build cache while filesystem permissions were restricted.

### Error

```text
open C:\Users\Administrator\AppData\Local\go-build\...: Access is denied.
```

### Context

- Operation: `go test ./internal/generator`.
- The workspace was writable, but Go's default cache directory was outside the allowed roots.

### Suggested Fix

Use an approved unrestricted test run or place `GOCACHE` under a writable temporary directory.

### Metadata

- Reproducible: yes under the restricted permission profile
- Related Files: internal/generator/

### Resolution

- **Resolved**: 2026-08-01T11:05:00+09:00
- **Notes**: The user enabled unrestricted filesystem access and the same test passed.

---

## [ERR-20260731-001] powershell-rg-scan

**Logged**: 2026-07-31T09:20:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

A repository inspection script exited with code 1 because a later `rg` query returned no matches or was truncated by the pipeline, even though the useful scan output was produced.

### Error

```text
Script error: Exit code: 1
```

### Context

- The command combined several independent scans in one PowerShell invocation.
- Ripgrep uses exit code 1 for “no matches”, which PowerShell propagated as the command result.

### Suggested Fix

Run independent scans separately, exclude generated dependency folders explicitly, and normalize expected `rg` exit code 1 before returning.

### Metadata

- Reproducible: yes
- Related Files: docs/
- Recurrence-Count: 2
- Last-Seen: 2026-07-30

### Resolution

- **Resolved**: 2026-07-31T09:20:00+08:00
- **Notes**: Subsequent scans use focused commands and explicitly handle expected no-match results.

---

## [ERR-20260801-001] ripgrep-windows-glob

**Logged**: 2026-08-01T10:30:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

Ripgrep rejected a wildcard embedded in a Windows path argument.

### Error

```text
rg: internal/generator/*_test.go: 文件名、目录名或卷标语法不正确。 (os error 123)
```

### Context

- Operation: search generator tests for atomic-related coverage.
- PowerShell passed the wildcard path literally to ripgrep.

### Suggested Fix

Search the directory and filter files with `-g '*_test.go'`.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/
- See Also: ERR-20260731-002
- Recurrence-Count: 3
- Last-Seen: 2026-08-02

### Resolution

- **Resolved**: 2026-08-01T10:31:00+08:00
- **Notes**: Re-ran the search with a ripgrep glob filter.

---

## [ERR-20260801-002] apply-patch-context

**Logged**: 2026-08-01T10:34:00+08:00
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary

A combined patch used the wrong indentation context for the generated nullable-field branch.

### Error

```text
apply_patch verification failed: Failed to find expected lines in internal/generator/render_go.go
```

### Context

- Operation: add atomic field-specific codec generation in two distant sections.
- The patch mixed a nullable branch line with a non-nullable branch line.

### Suggested Fix

Read the exact local sections and apply small independent patches.

### Metadata

- Reproducible: no
- Related Files: internal/generator/render_go.go

### Resolution

- **Resolved**: 2026-08-01T10:36:00+08:00
- **Notes**: Split the edit into exact decoder and encoder patches.

---

## [ERR-20260730-006] skill-validator-missing-pyyaml

**Logged**: 2026-07-30T08:03:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
The skill validator could not import its required `yaml` module, and a later successful command masked the failure exit code.

### Error
```
ModuleNotFoundError: No module named 'yaml'
```

### Context
- `quick_validate.py` and `gopls version` were initially run in one PowerShell command.
- `gopls version` succeeded, making the combined shell command exit successfully despite the validator traceback.

### Suggested Fix
Install `PyYAML` for the selected Python interpreter and run required validation commands separately so each exit code is authoritative.

### Metadata
- Reproducible: yes
- Related Files: .agents/skills/fgb-develop-feature/SKILL.md

### Resolution
- **Resolved**: 2026-07-30T08:03:00+08:00
- **Notes**: Installed `PyYAML` and reran `quick_validate.py` independently.

---

## [ERR-20260730-005] skill-init-python-command

**Logged**: 2026-07-30T08:01:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
The skill initializer could not run because this Windows environment exposes Python through `py`, not `python`.

### Error
```
python : The term 'python' is not recognized
```

### Context
- Command: `python .../skill-creator/scripts/init_skill.py`
- Python 3.14 was installed and available through the Windows Python Launcher.

### Suggested Fix
On Windows, check `py -3 --version` when `python` is unavailable and invoke Python scripts with `py -3`.

### Metadata
- Reproducible: yes
- Related Files: .agents/skills/fgb-develop-feature/SKILL.md

### Resolution
- **Resolved**: 2026-07-30T08:01:00+08:00
- **Notes**: Initialized the skill successfully with `py -3`.

---

## [ERR-20260729-004] multi-file-special-type-patch

**Logged**: 2026-07-31T09:52:00+08:00
**Priority**: low
**Status**: resolved
**Area**: config

### Summary

A ripgrep regex containing escaped HTML double quotes was again parsed as PowerShell syntax.

### Error

```text
src=\/ : The module 'src=' could not be loaded.
```

### Context

- Operation: scan documentation for root-absolute links before GitHub Pages deployment.

### Suggested Fix

Use single-quoted regex arguments and separate searches rather than embedding escaped double quotes.

### Metadata

- Reproducible: yes
- Related Files: docs/.vitepress/shared.ts
- See Also: ERR-20260731-005

### Resolution

- **Resolved**: 2026-07-31T09:52:00+08:00
- **Notes**: Replaced with focused single-quoted searches.

---

## [ERR-20260731-005] powershell-nested-quote

**Logged**: 2026-07-31T09:42:00+08:00
**Priority**: low
**Status**: resolved
**Area**: frontend

### Summary

A PowerShell source-inspection command failed because a nested quoted ripgrep pattern was not terminated correctly.

### Error

```text
The string is missing the terminator: ".
```

### Context

- The command searched VitePress component sources for quoted HTML class names.

### Suggested Fix

Use single-quoted PowerShell patterns or split source inspection into focused commands.

### Metadata

- Reproducible: yes
- Related Files: docs/.vitepress/theme/style.css

### Resolution

- **Resolved**: 2026-07-31T09:42:00+08:00
- **Notes**: Replaced the combined query with simple literal-pattern searches.

---

## [ERR-20260731-004] ui-ux-skill-python-missing

**Logged**: 2026-07-31T09:39:00+08:00
**Priority**: low
**Status**: resolved
**Area**: frontend

### Summary

The optional UI/UX recommendation search script could not run because Python is not installed or not on PATH.

### Error

```text
python: The term 'python' is not recognized as the name of a cmdlet, function, script file, or operable program.
```

### Context

- Operation: UI/UX design-system lookup for the VitePress home-page overlap fix.
- The skill's embedded responsive-layout guidance remains available without the script.

### Suggested Fix

Proceed using the skill's documented layout rules. Install Python only if database-backed UI recommendations are needed later.

### Metadata

- Reproducible: yes
- Related Files: docs/.vitepress/theme/style.css

### Resolution

- **Resolved**: 2026-07-31T09:39:00+08:00
- **Notes**: Continued with direct CSS inspection and viewport validation.

---

## [ERR-20260731-003] volta-npm-installation

**Logged**: 2026-07-31T09:28:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: infra

### Summary

The system `npm` launcher is broken because its Volta image cannot locate npm's bundled `npm-prefix.js`.

### Error

```text
Could not determine Node.js install directory
Error: Cannot find module 'C:\Users\Administrator\AppData\Local\Volta\tools\image\npm\11.10.1\bin\node_modules\npm\bin\npm-prefix.js'
```

### Context

- Operation: `npm run build` inside `docs/`.
- Node itself runs (`v25.6.1`), and project dependencies are already installed.

### Suggested Fix

Use the repository's actual package manager: Bun. The npm installation is outside this project's documentation workflow.

### Metadata

- Reproducible: yes
- Related Files: docs/package.json, docs/bun.lock

### Resolution

- **Resolved**: 2026-07-31T09:30:00+08:00
- **Notes**: The user clarified that docs use Bun. `bun run typecheck` and `bun run build` both pass.

---

## [ERR-20260731-002] ripgrep-windows-glob

**Logged**: 2026-07-31T09:24:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

Ripgrep rejected a Unix-style wildcard embedded in a Windows path argument.

### Error

```text
rg: template/plugin/gokit/docs/*.md: 文件名、目录名或卷标语法不正确。 (os error 123)
```

### Context

- PowerShell passed the wildcard path through without expanding it.
- Ripgrep treated `*` as part of a Windows filename.

### Suggested Fix

Pass the directory as the search root and use `-g '*.md'` for file filtering.

### Metadata

- Reproducible: yes
- Related Files: template/plugin/gokit/docs/
- See Also: ERR-20260731-001

### Resolution

- **Resolved**: 2026-07-31T09:24:00+08:00
- **Notes**: All later ripgrep scans use `-g` globs instead of wildcard path arguments.

---

## [ERR-20260801-005] parallel-verification-exit-state

**Logged**: 2026-08-01T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

A parallel final-verification script failed as a whole because one PowerShell child check intentionally returned exit code 1.

### Error

```text
Script error:
Exit code: 1
```

### Context

- Operation: verify PR metadata, worktree status, and confirm `CODE_AUDIT.md` was absent from the commit in parallel.
- The file check used `Select-String` plus `$LASTEXITCODE`; PowerShell cmdlet match state is not reliably represented by `$LASTEXITCODE`.

### Suggested Fix

Capture `Select-String` output in a variable and branch on whether the result is `$null`, without deliberately returning a failing process exit code.

### Metadata

- Reproducible: yes
- Related Files: .learnings/ERRORS.md

### Resolution

- **Resolved**: 2026-08-01T00:00:00+08:00
- **Notes**: Replaced the combined check with independent read-only commands that do not use `$LASTEXITCODE` for PowerShell cmdlet state.

---
## [ERR-20260802-006] diagnostic-parallel-git-probe

**Logged**: 2026-08-02T11:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

A parallel diagnostic probe aborted because it assumed the discovered Flutter application directory was a Git repository.

### Error

```text
fatal: not a git repository (or any of the parent directories): .git
Script error: Exit code: 1
```

### Context

- Operation: inspect Git state, Stream usage, and project files under `D:\Projects\clash_ui` in parallel.
- The application directory exists and is running, but it has no `.git` directory.
- The rejected Git command caused the composed parallel tool call to return before the useful file scans were reported.

### Suggested Fix

Test for `.git` before invoking Git in a newly discovered project, and keep optional repository metadata probes separate from source-file diagnostics.

### Metadata

- Reproducible: yes
- Related Files: .learnings/ERRORS.md

### Resolution

- **Resolved**: 2026-08-02T11:00:00+08:00
- **Notes**: Continued with repository-independent file inspection and treated Git metadata as optional.

---

## [ERR-20260802-MOD01] go-run-cross-module

**Logged**: 2026-08-02T11:16:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
The integration fixture attempted to run the codegen package from the fixture's separate Go module.

### Error
```text
directory ..\..\cmd\flutter_go_bridge_codegen outside main module or its selected dependencies
stat ...\build\time_fixture\cmd\flutter_go_bridge_codegen: directory not found
```

### Context
- Operation: invoke the repository CLI against an ignored integration fixture under `build/time_fixture`.
- `go run` resolves package paths in the current module, so changing into the fixture before running the repository command crossed module boundaries.

### Suggested Fix
Run `go run ./cmd/flutter_go_bridge_codegen` from the repository module with an absolute config path, then run fixture build/analyze commands from the fixture directory.

### Metadata
- Reproducible: yes
- Related Files: cmd/flutter_go_bridge_codegen, build/time_fixture/go.mod

### Resolution
- **Resolved**: 2026-08-02T11:16:00+08:00
- **Notes**: Split code generation and fixture validation into commands with the correct module working directory.

---

## [ERR-20260802-PATCH01] docs-context-mismatch

**Logged**: 2026-08-02T11:20:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary
A bilingual documentation patch assumed wording that differed from the Chinese page.

### Error
```text
apply_patch verification failed: Failed to find expected lines in docs/zh/reference/type-mapping.md
```

### Context
- Operation: add the `time.Time` wire-format migration note to both language variants.
- The Chinese UUID and pointer-transition text did not match the assumed context.

### Suggested Fix
Read both exact table tails before applying a multi-file bilingual documentation patch.

### Metadata
- Reproducible: no
- Related Files: docs/reference/type-mapping.md, docs/zh/reference/type-mapping.md

### Resolution
- **Resolved**: 2026-08-02T11:20:00+08:00
- **Notes**: Reapplied the note using the exact text from each page.

---
## [ERR-20260804-001] powershell-rg-glob-and-quoting

**Logged**: 2026-08-04T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary

Parallel diagnostics failed when a Unix-style `*_test.go` path glob and Markdown backticks were embedded directly in PowerShell commands.

### Error

```text
rg: internal/generator/*test.go: 文件名、目录名或卷标语法不正确。
The string is missing the terminator: ".
```

### Context

- Operation: search generated-runtime tests and documentation text on Windows.
- PowerShell passed the wildcard as a literal path, while backticks inside a double-quoted command changed PowerShell parsing.

### Suggested Fix

Use `rg -g '*_test.go' PATTERN directory` for file globs and avoid Markdown backticks inside double-quoted PowerShell command strings.

### Metadata

- Reproducible: yes
- Related Files: internal/generator, docs/reference, docs/zh/reference

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Re-ran the searches with `rg -g` and simpler PowerShell commands.

---

## [ERR-20260804-002] bilingual-doc-patch-context

**Logged**: 2026-08-04T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

A combined bilingual documentation patch failed because it assumed the English error-table wording also appeared in the Chinese page.

### Error

```text
apply_patch verification failed: Failed to find expected lines in docs/zh/reference/type-mapping.md
```

### Context

- Operation: update round-three interface, error, and time behavior in both language variants.
- The failed patch was atomic and did not partially modify code or documentation.

### Suggested Fix

Read the exact section in each language and apply smaller page-specific patches.

### Metadata

- Reproducible: yes
- Related Files: docs/reference/type-mapping.md, docs/zh/reference/type-mapping.md
- See Also: ERR-20260802-PATCH01

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Split the patch and updated each page against its actual text.

---
## [ERR-20260804-003] round3-generated-output-gates

**Logged**: 2026-08-04T00:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary

Real generated-code gates caught invalid Dart nullability and malformed Go format strings that repository compilation could not see.

### Error

```text
bridge_generated.dart: Expected to find ')' at String??
bridge_generated.go: fmt.Errorf call needs 0 args but has 1 arg
aggregate coverage: 94.9% (required: 95.0%)
```

### Context

- Operation: generate the round-three integration fixture, run `dart format`/`dart analyze`, run generated-module `go vet`, and enforce aggregate coverage.
- `error -> String?` was widened twice by generic nullability helpers.
- Renderer lines containing literal `%s` used the formatted writer incorrectly and emitted `%%s`.
- New branches initially lowered aggregate statement coverage by 0.1 percentage point.

### Suggested Fix

Keep generated Go vet, generated Dart analysis, runtime smoke, and aggregate coverage as separate required gates.

### Metadata

- Reproducible: yes
- Related Files: internal/generator/model.go, internal/generator/render_dart_split.go, internal/generator/render_go.go, internal/generator/render_go_cst.go
- See Also: ERR-20260804-002

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Nullability widening is idempotent, literal generated format source uses `raw`, focused branch tests raised aggregate coverage to 95.0%, and the full integration smoke passed.

---
