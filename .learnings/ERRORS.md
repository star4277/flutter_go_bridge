## [ERR-20260729-001] combined-rg-discovery

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
A combined PowerShell discovery command returned exit code 1 when an optional `rg` search had no matches.

### Error
```
Script error: Exit code: 1
```

### Context
- The command combined directory listing, an optional `AGENTS.md` search, and source searches.
- `rg` returns exit code 1 for no matches, which marked the whole tool call as failed.

### Suggested Fix
Run optional searches separately or explicitly handle the no-match exit code.

### Metadata
- Reproducible: yes
- Related Files: none

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Subsequent discovery commands handle optional results explicitly. On Windows, pass directories to `rg` with `-g` filters instead of shell-style path globs such as `internal/*.go`.

---

## [ERR-20260730-004] wait-without-running-exec-cell

**Logged**: 2026-07-30T07:46:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
The wait tool was called with a placeholder cell id even though no asynchronous exec cell existed.

### Error
```
exec cell x not found
```

### Context
- The failed waits occurred while switching from a completed generation command to parallel build commands.
- No repository process or file was affected.

### Suggested Fix
Only call `wait` after an exec result explicitly returns `Script running with cell ID ...`; use an exec script with `Promise.all` for parallel shell commands.

### Metadata
- Reproducible: yes
- Related Files: none

### Resolution
- **Resolved**: 2026-07-30T07:46:00+08:00
- **Notes**: Subsequent parallel build/analyze commands used one exec script with `Promise.all`.

---

## [ERR-20260730-003] cgo-smoke-rebuild-workdir

**Logged**: 2026-07-30T07:44:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
The post-regeneration CGo rebuild ran from the repository root instead of the smoke fixture's Go module.

### Error
```
no Go files in D:\Projects\a\flutter-go-bridge-gokit
```

### Context
- Generation completed successfully.
- A chained command reused the generator command's repository-root working directory for `go build`.

### Suggested Fix
Run generation and module compilation as separate commands with explicit working directories.

### Metadata
- Reproducible: yes
- Related Files: build/cgo_smoke/go/go.mod

### Resolution
- **Resolved**: 2026-07-30T07:44:00+08:00
- **Notes**: Re-ran `go build` from `build/cgo_smoke/go`.

---

## [ERR-20260730-002] cgo-smoke-nullable-error-message

**Logged**: 2026-07-30T07:43:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
The first CGo Dart smoke script treated `FgbPlatformException.message` as non-nullable.

### Error
```
Method 'contains' cannot be called on 'String?' because it is potentially null.
```

### Context
- Command: `dart run bin/smoke.dart cgo_smoke.dll`
- The generated dynamic library had already built successfully.
- The failure was in the ignored smoke harness, before loading the DLL.

### Suggested Fix
Use a null-aware assertion when checking generated platform exception messages.

### Metadata
- Reproducible: yes
- Related Files: build/cgo_smoke/dart/bin/smoke.dart

### Resolution
- **Resolved**: 2026-07-30T07:43:00+08:00
- **Notes**: Changed the assertion to `error.message?.contains(...) ?? false`.

---

## [ERR-20260730-001] powershell-temporary-go-module-newlines

**Logged**: 2026-07-30T07:23:01+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
A PowerShell diagnostic created an invalid temporary `go.mod` because a single-quoted string preserved the literal `` `n `` characters.

### Error
```
go.mod:1: usage: module module/path
```

### Context
- The command used `Set-Content` with a single-quoted PowerShell string containing backtick newline escapes.
- The diagnostic was outside the repository and did not affect project files.

### Suggested Fix
Prefer repository tests created with `apply_patch`; when a temporary PowerShell file is unavoidable, use a here-string or a double-quoted string and run the command from the temporary module directory.

### Metadata
- Reproducible: yes
- Related Files: internal/generator/features_test.go

### Resolution
- **Resolved**: 2026-07-30T07:23:01+08:00
- **Notes**: Switched the investigation to a permanent Go regression test instead of shell-authored fixture files.

---

## [ERR-20260729-004] multi-file-special-type-patch

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary
A large multi-file patch for InternetAddress and UUID mappings failed context verification.

### Error
```
apply_patch verification failed: Failed to find expected lines in render_go_cst.go
```

### Context
- No part of the patch was applied.
- The DCO renderer used different indentation/text than the patch context assumed.

### Suggested Fix
Apply the model and each renderer change in small patches using exact local context.

### Metadata
- Reproducible: yes
- Related Files: internal/generator/render_go_cst.go

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Switched to small, independently verifiable patches.

---

## [ERR-20260729-003] mihomoui-flutter-analyze-timeout

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
Full Flutter analysis of `example/mihomoui` exceeded the 120-second command timeout without producing output.

### Error
```
command timed out after 120221 milliseconds
```

### Context
- Command: `fvm flutter analyze`
- The generated Go bridge had already compiled successfully.

### Suggested Fix
Run a focused Dart analysis on `lib/src` with a larger timeout, then retry broader validation only if necessary.

### Metadata
- Reproducible: unknown
- Related Files: example/mihomoui/lib/src/_generated.dart

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: User ran focused format and analysis successfully; only existing informational lints were reported.

---

## [ERR-20260729-005] flutter-pub-add-partial-write

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: config

### Summary
`flutter pub add uuid` timed out after modifying `pubspec.yaml`, leaving a successful partial write despite a failed command result.

### Error
```
mapping key "uuid" already defined
```

### Context
- The timed-out command inserted `uuid: ^4.6.0`.
- A subsequent manual dependency edit added a second UUID entry.

### Suggested Fix
After a pub-add failure or timeout, re-read `pubspec.yaml` before retrying or applying a fallback edit.

### Metadata
- Reproducible: unknown
- Related Files: example/mihomoui/pubspec.yaml, command/main.go

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Kept the pub-added `uuid: ^4.6.0` entry and removed the duplicate.

---

## [ERR-20260729-002] go-test-cache-miss

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary
Go tests failed because the shared Windows build cache referenced missing compiled source-list entries.

### Error
```
loading compiled Go files from cache: reading srcfiles list: cache entry not found
```

### Context
- `go test ./internal/generator ./internal/parser`
- Failures occurred inside `go/packages` while loading temporary fixture modules.

### Suggested Fix
Run validation with an isolated `GOCACHE` inside the writable workspace.

### Metadata
- Reproducible: unknown
- Related Files: internal/parser/parser.go

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Full validation passed with `GOCACHE` pointed at an isolated temporary directory.

---
