# Learnings

## [LRN-20260802-003] correction

**Logged**: 2026-08-02T23:20:00+09:00
**Priority**: medium
**Status**: resolved
**Area**: config

### Summary

Audit reports moved out of the worktree are intentionally excluded from an "all files" commit.

### Details

The user moved `CODE_AUDIT.md` and `CODE_AUDIT_ROUND2.md` out of the repository specifically so they would not be committed. Their absence is deliberate and must not be treated as a missing artifact to restore.

### Suggested Action

When committing all current changes, use the current worktree as the source of truth and do not recreate files the user deliberately removed before staging.

### Metadata

- Source: user_feedback
- Related Files: CODE_AUDIT.md, CODE_AUDIT_ROUND2.md
- Tags: git, staging, user-intent

### Resolution

- **Resolved**: 2026-08-02T23:20:00+09:00
- **Notes**: The reports remain outside the worktree and are excluded from the commit.

---

## [LRN-20260802-002] correction

**Logged**: 2026-08-02T10:55:00+08:00
**Priority**: high
**Status**: pending
**Area**: backend

### Summary

Recursive atomic-state checks must be resolved before runtime lifecycle fixes because wrappers can bypass outer-kind guards and make generated Go uncompilable.

### Details

The updated round-two audit added N15: `containsAtomic` recurses through pointers and named types, but builder rejection switched only on the mapped type's outer `Kind`. A named or pointer-wrapped atomic collection therefore bypasses the guard, while `usesPointerCodec` still changes the element codec signature. The generated collection copies atomic elements and passes values to pointer-taking encoders, so it fails compilation and `go vet` despite generator unit tests passing.

### Suggested Action

When a generator invariant is recursive, enforce it with a recursive blocker/result rather than an outer-kind switch. Add generated-module `go vet` fixtures for named, pointer-wrapped, and anonymous collection shapes before continuing lower-priority runtime work.

### Metadata

- Source: user_feedback
- Related Files: CODE_AUDIT_ROUND2.md, internal/generator/builder.go, internal/generator/model.go
- Tags: atomic, codegen, copylocks, regression-test
- See Also: LRN-20260801-002, LRN-20260801-003

---

## [LRN-20260802-001] knowledge_gap

**Logged**: 2026-08-02T09:40:00+08:00
**Priority**: high
**Status**: pending
**Area**: backend

### Summary

The bridge has two unrelated 30-second timeouts, and the one that cuts long-lived streams is the Dart-side call timeout, not the callback timeout.

### Details

`fgbCallbackTimeout` bounds how long Go waits for a Dart closure to reply. Separately, `fgbInvokeCstAsync` and `_asyncBytes` wrapped `await port.first` in `.timeout(Duration(seconds: 30))`, which bounds the *whole Go call*. A long-running `//fgb:async` stream producer hits the second one: the Future throws, `fgbInternalStartStream`'s onError closes the controller, and the stream disconnects at 30s. The same timeout also lets the generated `finally { arena.close(); }` free the CST argument arena while a Go worker is still reading it.

### Suggested Action

Never bound a whole bridge call by wall clock — a legitimate Go call may run for minutes. Use the allocation-failure sentinel for the "reply can never arrive" case, and a call-scoped context for cancellation. When diagnosing a stream that dies on a fixed schedule, check the call-level timeout before the callback timeout.

### Metadata

- Source: user_feedback
- Related Files: internal/generator/dart_runtime.go, internal/generator/render_dart_cst.go, CODE_AUDIT_ROUND2.md
- Tags: timeout, stream, use-after-free, ffi

---
## [LRN-20260801-003] best_practice

**Logged**: 2026-08-01T12:30:00+09:00
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary

Representative generated Go fixtures must run `go vet`, not only source substring assertions.

### Details

During the audit fixes, generator unit tests and repository `go vet ./...` both passed while a generated
CST decoder still emitted `fmt.Errorf("%%s...", path)`. Running `go vet ./...` inside the temporary
generated module caught the invalid format call and also validates copylocks and unused locals.

### Suggested Action

Keep `TestGeneratedGoCompilesForMultipleSinksAndAtomicField` as a generated-module vet gate and add
representative shapes there when generator signatures change.

### Metadata

- Source: error
- Related Files: internal/generator/features_test.go, internal/generator/render_go_cst.go
- Tags: codegen, go-vet, regression-test
- See Also: LRN-20260801-002

### Resolution

- **Resolved**: 2026-08-01T12:35:00+09:00
- **Notes**: Added generated-module vet coverage and fixed the malformed generated format strings.

---

## [LRN-20260801-001] best_practice

**Logged**: 2026-08-01T10:10:00+08:00
**Priority**: high
**Status**: pending
**Area**: backend

### Summary

The generated runtime protects streams against Dart isolate hot restarts with a generation guard, but the opaque, DartOpaque, and callback registries have no equivalent.

### Details

`go_runtime.go` keys stream state by `fgbStreamKey{generation, handle}` and retires a whole generation in `fgb_stream_port`, because a hot restart kills the old isolate without a cancel and Dart restarts its handle counter at 1. The same collision applies to `fgbHandles` (opaque), `_dartOpaqueObjects` (DartOpaque), and the shared callback registry, none of which carry a generation. A stale Go cleanup can therefore release or invoke a live object belonging to the new isolate, and `fgbHandles` is never cleared at all, so every hot restart leaks its entries permanently.

### Suggested Action

When touching any Dart-side handle registry, treat "which isolate generation issued this handle" as part of the key. Extract the stream generation mechanism into one shared registry helper rather than adding a second ad-hoc copy. See CODE_AUDIT.md items C3, H1, H2, H3.

### Metadata

- Source: conversation
- Related Files: internal/generator/go_runtime.go, internal/generator/dart_runtime.go, CODE_AUDIT.md
- Tags: ffi, isolate, hot-restart, memory-leak, handle-registry

---
## [LRN-20260801-002] best_practice

**Logged**: 2026-08-01T10:10:00+08:00
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary

Generator tests assert on substrings of the generated source, so generated code that does not compile still passes.

### Details

`internal/generator/features_test.go` checks `strings.Contains(goSource, ...)`. A call with two `fgb.StreamSink` parameters plus a `context.Context` emits `handle1 := ...` with no second reference, which is a hard `declared and not used` compile error; every existing test passed regardless. `sync/atomic` types are likewise emitted by value, which trips the copylocks vet check in user projects.

### Suggested Action

Add a test tier that type-checks or builds the generated Go for representative fixtures, or at minimum asserts every generated local has more than one occurrence. Substring assertions alone cannot catch this class.

### Metadata

- Source: conversation
- Related Files: internal/generator/features_test.go, internal/generator/render_go.go
- Tags: codegen, testing, compile-check
- See Also: LRN-20260801-001

---
## [LRN-20260731-001] correction

**Logged**: 2026-07-31T09:30:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: docs

### Summary

The documentation site uses Bun, not npm.

### Details

The repository contains `docs/bun.lock`, and Bun 1.3.13 is available. Documentation dependency, development, type-check, and build commands should use `bun install` and `bun run ...`.

### Suggested Action

Use Bun for all docs commands and document Bun as the package manager in contributor instructions.

### Metadata

- Source: user_feedback
- Related Files: docs/bun.lock, docs/package.json
- Tags: bun, vitepress, docs

### Resolution

- **Resolved**: 2026-07-31T09:30:00+08:00
- **Notes**: Switched validation and authored commands to Bun.

---
## [LRN-20260731-001] correction

**Logged**: 2026-07-31T12:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: docs

### Summary

A request to rewrite bilingual README files means redesigning their content, not migrating the old README verbatim.

### Details

The initial interpretation planned to preserve the existing long Chinese README as the Chinese version and translate its structure into English. The user explicitly required both README files to be rewritten around the project's current positioning and documentation site instead.

### Suggested Action

For documentation rewrite requests, first define a new information architecture and remove material better served by the full documentation site. Treat translation/migration as a separate request that requires explicit wording.

### Metadata

- Source: user_feedback
- Related Files: README.md, README.zh-CN.md
- Tags: readme, rewrite, bilingual

### Resolution

- **Resolved**: 2026-07-31T12:00:00+08:00
- **Notes**: Replaced the migration approach with fresh English and Chinese README outlines and content.

---
## [LRN-20260730-002] correction

**Logged**: 2026-07-30T09:14:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: infra

### Summary
Codecov `ignore` filters uploaded coverage paths but does not remove a Go package from `go test ./...` or its CI log.

### Details
The workflow still enumerated `github.com/star4277/flutter_go_bridge/template` because the Go command selected every package before Codecov processed the coverage profile. Excluding a package from test execution requires filtering the package list passed to `go test`.

### Suggested Action
Keep `.codecov.yml` ignore rules for uploaded paths, and separately build the Go package list with `go list ./... | grep -v '/template$'` before running tests.

### Metadata
- Source: user_feedback
- Related Files: .github/workflows/test.yml, .codecov.yml
- Tags: codecov, go-test, coverage, github-actions

### Resolution
- **Resolved**: 2026-07-30T09:14:00+08:00
- **Notes**: The workflow now passes an explicitly filtered package array to `go test`.

---
