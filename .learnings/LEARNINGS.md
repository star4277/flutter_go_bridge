# Learnings

## [LRN-20260804-001] knowledge_gap

**Logged**: 2026-08-04T11:30:00+08:00
**Priority**: high
**Status**: pending
**Area**: backend

### Summary

An interface field is nil in every struct's zero value, but the generated encoder rejects nil interfaces, so the zero value of any struct with an interface field cannot cross the bridge.

### Details

`renderEncoder`'s kindInterface branch emits `cannot send a nil <Iface> to Dart`, and the Dart field is non-nullable. Confirmed at runtime for both an input-package interface and a dependency interface: `ZeroSlot()` and `ZeroReport()` both fail with `FgbPlatformException(encode_error, ...)`. Pointers, slices and maps all normalize their nil form automatically; interfaces are the only nilable-without-pointer kind that does not. The `fgb:"nullable"` tag is a working escape hatch but is opt-in. The problem predates the third-round interface work, which widened its reach by turning external-interface fields from a GoOpaque downgrade into a value-class field.

### Suggested Action

Decided approach (not yet implemented): encode a nil interface as `null` and decode it on the Dart side into a generated per-interface "absent" implementation whose methods throw, marked with a shared `GoAbsent` interface so it stays detectable. Non-breaking, removes the encode failure, keeps methods unusable. The Dart encoder must map `GoAbsent` back to `null` or round trips break. Dependency interfaces are marker-only, so the throwing-method guarantee is vacuous there and `is GoAbsent` is the only signal. Full design and checklist in .audit/CODE_AUDIT_ROUND4.md.

### Metadata

- Source: conversation
- Related Files: internal/generator/render_go.go, internal/generator/render_dart_split.go, internal/generator/dart_runtime.go, .audit/CODE_AUDIT_ROUND4.md
- Tags: interface, nil, null-object, codegen, dart

---

## [LRN-20260804-002] best_practice

**Logged**: 2026-08-04T11:30:00+08:00
**Priority**: high
**Status**: pending
**Area**: tests

### Summary

`dart analyze` catches generated-code bugs that `go vet` cannot, but only a runtime smoke test catches semantic ones like the nil-interface encode failure.

### Details

Every severe finding in rounds 2-4 passed `go vet` and the full `go test` suite. The round-3 `dart analyze` gate now catches the compile-level class (ambient name shadowing). The round-4 finding is a level deeper: the generated code compiles, analyzes clean, and only fails when Go actually returns a struct whose interface field is nil. Substring assertions and static analysis both miss it.

### Suggested Action

Keep a fixed integration fixture in the repository and run generate, `-buildmode=c-shared` build, `dart analyze`, and a Dart smoke run in CI. The round-4 fixture under `build/r4` covers error positions, both interface flavours, ambient-name types, time, opaque, async and channel streams, plus a concurrency stress script; promote it out of the ignored `build/` directory.

### Metadata

- Source: conversation
- Related Files: .audit/CODE_AUDIT_ROUND4.md
- Tags: testing, integration, smoke, ci

---

## [LRN-20260803-011] knowledge_gap

**Logged**: 2026-08-03T14:10:00+08:00
**Priority**: critical
**Status**: resolved
**Area**: backend

### Summary

`names.Sanitize` guards Dart keywords but not `dart:core` type names, so a Go type named `List` or `Duration` generates Dart that does not compile.

### Details

`dart:core` is implicitly imported, so a generated top-level class shadows the core type of the same name in that library. The generator itself emits `List<T>`, `Map<K,V>`, `Duration(microseconds: ...)`, `DateTime`, `BigInt`. Measured: `type List struct{...}` plus a `[]string` field yields `final List<String> names;` alongside `final class List {}` (0 type parameters) — a hard compile error; `type Duration struct{...}` plus a `time.Duration` field breaks `Duration(microseconds:)` and `.inMicroseconds` the same way. Names the generator does not use (`Comparable`, `Sink`, `Iterator`, `Pattern`) shadow silently instead. The third-party interface feature widens the blast radius because it names Dart classes after dependency types the user cannot rename (`error` becomes `class Error`).

### Suggested Action

Add a `dartCoreTypes` set alongside `dartReserved`, prefix colliding upper-camel names (`GoList`), warn, and seed the existing `uniqueName` table with core type names. Add a `dart analyze` gate over generated fixtures — the Go side already has a `go vet` gate and this class of bug only shows up there. See .audit/CODE_AUDIT_ROUND3.md item C1.

### Metadata

- Source: conversation
- Related Files: internal/names/names.go, internal/generator/render_dart_split.go, .audit/CODE_AUDIT_ROUND3.md
- Tags: dart, naming, shadowing, codegen, compile-failure

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Ambient Dart names are renamed with a `Go` prefix, warnings are emitted, and generated Dart fixtures cover the collision.

---

## [LRN-20260803-012] knowledge_gap

**Logged**: 2026-08-03T14:10:00+08:00
**Priority**: critical
**Status**: resolved
**Area**: backend

### Summary

A dependency interface's union tags are a function of the input package's whole transitive import graph, not of the user's API.

### Details

`collectImplementors` scans every loaded package for types implementing a non-input-package interface and assigns positional tags. Measured twice (before and after the JSON rollback, identical result): a `fmt.Stringer` field produced 7 union members; adding one unrelated function that pulls `net/url` into the graph produced 16, shifting every tag (`*os.ProcessState` 0 to 8, the opaque fallback 6 to 15). Dart packages and native libraries are usually built separately, so any graph difference between the two builds decodes a tag as the wrong type — silently, when both types are field-compatible maps.

### Suggested Action

Restrict dependency-interface candidates to named types already bridged for another reason, and make the wire tag a stable content identifier (package path + type name, or its hash) rather than a position. Add a regression test that generates the same API under two different import graphs and asserts identical tags. See .audit/CODE_AUDIT_ROUND3.md item S1.

### Metadata

- Source: conversation
- Related Files: internal/generator/builder.go, .audit/CODE_AUDIT_ROUND3.md
- Tags: interface, wire-abi, tag-stability, codegen

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Dependency candidates now come only from callable-reachable types, dedicated mappings stop traversal, and content tags replace positional tags.

---

## [LRN-20260804-001] knowledge_gap

**Logged**: 2026-08-04T00:00:00+08:00
**Priority**: critical
**Status**: resolved
**Area**: backend

### Summary

Generated Go must never register a package already owned by the fixed runtime import set under a second `fgbextN` alias.

### Details

Dependency-interface implementor discovery could register `time`, `runtime`, `reflect`, `os`, `math/big`, or `sync/atomic` as an external package even though generated Go already imports it under its fixed name. `goType` continued emitting the fixed name, so the new alias was unused and generated code failed compilation. An ordinary `error` field naturally exposed the issue because the old open-world interface scan pulled fixed-package implementations into the union.

### Suggested Action

Treat fixed runtime imports and the generated support package as reserved in `registerExternalPackage`, and run `go vet` on a fixture whose dependency-interface implementor comes from a fixed package.

### Metadata

- Source: user_feedback
- Related Files: internal/generator/builder.go, internal/generator/round3_audit_test.go
- Tags: codegen, imports, alias, interface, compile-failure
- See Also: LRN-20260803-012, LRN-20260801-003

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Fixed imports are skipped during external registration; all reserved paths have unit coverage and an `os.ProcessState` generated-module vet regression.

---

## [LRN-20260804-002] best_practice

**Logged**: 2026-08-04T00:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: frontend

### Summary

Dart nullability widening must be idempotent because some dedicated mappings are already nullable.

### Details

The dedicated ordinary-`error` mapping uses `String?`. Generic encoder, field-tag, and parameter nullability helpers appended another `?`, producing invalid `String??` that source substring tests missed but `dart format` rejected.

### Suggested Action

Route every widening operation through one helper that returns an already-nullable Dart spelling unchanged, and keep a real generated Dart analysis gate.

### Metadata

- Source: error
- Related Files: internal/generator/model.go, internal/generator/render_dart_split.go, internal/generator/round3_audit_test.go
- Tags: dart, nullability, codegen, analyzer

### Resolution

- **Resolved**: 2026-08-04T00:00:00+08:00
- **Notes**: Added `nullableDartType`, regression coverage, and a successful generated-package `dart analyze` run.

---

## [LRN-20260803-001] best_practice

**Logged**: 2026-08-03T02:22:00+08:00
**Priority**: high
**Status**: resolved
**Area**: backend

### Summary

Third-party Go interface serialization needs both deterministic named-implementor discovery and an interface-level opaque fallback.

### Details

Walking the loaded dependency graph finds exported named structs and enables field serialization, stable union tags, and pointer/value handling. It cannot name unexported implementations, types behind inaccessible `internal` packages, generic runtime instantiations, or wrappers registered at runtime. The real mihomo `constant.ProxyAdapter` graph returned `*outbound.autoCloseProxyAdapter`, proving that enumeration alone can generate successfully yet fail at runtime.

The complete pattern appends a final union member that boxes the interface value itself in the Go opaque registry. Named implementations keep their precise Dart classes; every unnameable implementation uses the fallback class and can be passed back to Go with identity intact.

### Suggested Action

Whenever bridging an open-world interface, enumerate safely nameable implementations in deterministic order, then add a last-match opaque fallback and exercise both directions with an unexported runtime implementation.

### Metadata

- Source: error
- Related Files: internal/generator/builder.go, internal/generator/render_go.go, internal/generator/generator_test.go
- Tags: go, interfaces, codegen, serialization, opaque, open-world
- Pattern-Key: harden.external_interface_open_world
- Recurrence-Count: 1
- First-Seen: 2026-08-03
- Last-Seen: 2026-08-03

### Resolution

- **Resolved**: 2026-08-03T02:22:00+08:00
- **Notes**: Implemented dependency-graph discovery plus interface-level `GoOpaque`, validated against clash_ui/mihomo and an isolated bidirectional smoke fixture.

---

## [LRN-20260802-004] correction

**Logged**: 2026-08-02T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: frontend

### Summary

Dart `library` declarations belong to the plugin public entrypoint, not every generated bridge implementation file.

### Details

Flutter app projects do not need a named `library` declaration. Plugin projects already receive `library <plugin_name>;` from `template/shared/plugin/lib/REPLACE_ME_DART_PACKAGE_NAME.dart`, which exports the generated bridge. The generator's central `bridge_generated.dart` is shared by both project types and must not hardcode `library flutter_go_bridge;`.

### Suggested Action

Keep project-type-specific Dart package declarations in the integration templates. Generate the internal bridge implementation without a named library directive.

### Metadata

- Source: user_feedback
- Related Files: internal/generator/render_dart_split.go, template/shared/plugin/lib/REPLACE_ME_DART_PACKAGE_NAME.dart
- Tags: dart, plugin, app, codegen

### Resolution

- **Resolved**: 2026-08-02T00:00:00+08:00
- **Notes**: The analyzer cleanup preserves the plugin entrypoint declaration and removes the hardcoded central declaration.

---

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
- Pattern-Key: generator.generated_printf_literals
- Recurrence-Count: 2
- First-Seen: 2026-08-01
- Last-Seen: 2026-08-04

### Resolution

- **Resolved**: 2026-08-01T12:35:00+09:00
- **Notes**: Added generated-module vet coverage and fixed malformed generated format strings. The pattern recurred in round three; literal `%` source now uses renderer `raw` calls.

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

## [LRN-20260802-TIME] correction

**Logged**: 2026-08-02T11:15:00+08:00
**Priority**: high
**Status**: resolved
**Area**: backend

### Summary
Map `time.Time` to Dart `DateTime` with signed Unix microseconds instead of RFC3339 text.

### Details
`DateTime` has microsecond precision and exposes `microsecondsSinceEpoch` plus `DateTime.fromMicrosecondsSinceEpoch`. An integer wire value preserves the instant without RFC3339 parsing or offset normalization bugs and lets the CST path use a native `int64`. Go sub-microsecond nanoseconds and the original location are not representable by Dart `DateTime` and are therefore intentionally not preserved.

### Suggested Action
Use `time.Time.UnixMicro`/`time.UnixMicro` in generated Go and `DateTime.microsecondsSinceEpoch`/`DateTime.fromMicrosecondsSinceEpoch` in generated Dart. Document the precision and location boundary.

### Metadata
- Source: user_feedback
- Related Files: internal/generator/render_go.go, internal/generator/render_go_cst.go, internal/generator/render_dart_split.go, internal/generator/render_dart_cst.go
- Tags: time, datetime, serialization, timezone, cst

### Resolution
- **Resolved**: 2026-08-02T11:15:00+08:00
- **Notes**: Generator codecs and type-mapping documentation now use Unix microseconds.

---
