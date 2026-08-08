---
name: fgb-comprehensive-code-review
description: Exhaustive repository-wide code audit for flutter_go_bridge and similar Go/Dart bridge projects. Use when reviewing every project code file rather than only a diff, finding bugs, dead or unreachable code, performance defects, resource leaks, races, deadlocks or unintended blocking, validating generated Go/Dart, or proving that the standard codec has identical observable behavior on native and Wasm. Requires gopls and go vet for Go, dart analyze for generated Dart, runnable native and Wasm artifacts, and explicit treatment of cgo/syscall/js target-only behavior.
---

# FGB Comprehensive Code Review

Perform a slow, evidence-driven, repository-wide audit. Do not sample files, stop after the first
finding, or substitute a green build for code inspection. Finish only after every auditable source,
test, generated file, template, script, and build configuration is accounted for.

## Required reading

Before inspecting code, read these files completely:

- [review-checklist.md](references/review-checklist.md) — mandatory review dimensions.
- [native-wasm-parity.md](references/native-wasm-parity.md) — standard-codec parity boundary,
  target execution matrix, and exception rules.

Use the repository's `code-review` skill for a fixed-point diff review. Use this skill for a full
project audit or any request that says every file or every bug category must be scanned.

## Completion contract

Treat every unmet item as `BLOCKED`; do not dilute it into a minor caveat:

1. Generate a fingerprinted ledger with `scripts/audit_ledger.py`. Review every row and record either
   finding IDs or `none`. A changed fingerprint invalidates the previous review.
2. Apply every relevant checklist dimension to every code area. A risk scan may order work but may
   not remove low-risk files from the deep review.
3. Run all required tools successfully. Missing tools, packages, platforms, or runners remain named
   blockers.
4. All Go code passes formatting, package loading/compilation, `gopls check`, and `go vet ./...`.
   Relevant tests and the race-enabled suite must also pass where the target supports `-race`.
5. Every generated Dart package passes `dart analyze` or the repository's pinned equivalent.
6. Fresh generated native and Wasm artifacts both build and execute representative calls. Compilation
   without runtime calls is not execution proof.
7. Compare only the observable behavior of the standard codec shared by both targets. Native may use
   CST/DCO and fall back to the standard codec; Wasm may expose only the standard codec. Do not demand
   identical internal codecs or transport implementations. When the native standard-codec path and
   the Wasm standard-codec path receive equivalent inputs, their public results, errors, ordering,
   side effects, and ownership semantics must match except for documented behavior that necessarily
   depends on `cgo` or `syscall/js`.
8. Report every discovered problem. Attach an actionable solution and regression verification when
   one can be established. If a reliable solution cannot currently be established or applied, keep
   the finding, mark its resolution `BLOCKED`, and explain the exact reason, missing evidence or
   authority, rejected unsafe approaches, and the next step needed to unblock it.

Do not claim “no bugs” or “fully correct.” Say “no findings in the audited scope” only when all rows
and gates are complete, then list residual environmental limits.

## Workflow

### 1. Freeze scope and evidence

- Record repository root, commit, branch, dirty files, toolchain versions, enabled build tags,
  supported targets, and available native/Wasm/Dart runners. Preserve user changes.
- Read project instructions, architecture and development docs, generator configuration, build
  scripts, tests, examples, and platform templates before judging implementation details.
- Store temporary ledgers, generated packages, logs, profiles, and harnesses under ignored output
  such as `build/review/<timestamp>/`.
- Include executable submodule/vendor code when it contributes to produced artifacts. Otherwise
  record the pinned revision and an explicit scope justification.

### 2. Create and maintain the exhaustive ledger

From the repository root, run:

```powershell
py -3 .agents/skills/fgb-comprehensive-code-review/scripts/audit_ledger.py init `
  --repo . --ledger build/review/audit-ledger.tsv
```

Use `python3` or `python` when appropriate. The ledger includes tracked source-like files, tests,
templates, generated output, executable scripts, build configuration, and gitlinks. Do not delete a
row because it appears trivial.

For each row, read the whole file, trace relevant callers and callees, and set `status` to `reviewed`.
Set `findings_or_reason` to comma-separated finding IDs or the literal `none`. For a submodule used in
the artifact, create and verify a child ledger at its pinned commit, then cite that ledger in the
parent row.

After any edit or regeneration, verify again:

```powershell
py -3 .agents/skills/fgb-comprehensive-code-review/scripts/audit_ledger.py verify `
  --repo . --ledger build/review/audit-ledger.tsv
```

Zero missing, extra, duplicate, stale, pending, or evidence-empty rows is mandatory.

### 3. Map the complete execution model

Trace public calls in both directions and across both targets:

```text
Dart API -> generated encode/transport -> generated Go -> user Go
user Go result -> generated encode/transport -> Dart decode/API
```

Map native CST, DCO, and standard-codec fallback separately from Wasm standard codec. Identify public
entry points, generated/source-of-truth boundaries, callbacks, streams, queues, locks, channels,
goroutines, ports, finalizers, ownership transfers, build tags, `cgo`, and `syscall/js` branches.

### 4. Run mechanical gates

Capture the exact command, version, exit status, and relevant output. Run gates before deep review to
find broad failures, then repeat affected gates after fixes or regeneration.

Go, covering all tracked Go files and all packages:

```powershell
gofmt -l (git ls-files '*.go')
go vet ./...
go test -count=1 ./...
go test -count=1 -race ./...
gopls check <every tracked .go file>
```

`gofmt -l` must print nothing. Batch `gopls check` by package if the argument list is too long. Use
`go list ./...` rather than assuming package paths. If `-race` is unsupported for a target, run it on
a supported host against the same portable code and record the uncovered target-specific code as a
blocker or targeted manual/runtime review.

Run configured analyzers such as `staticcheck` when available, but never use them as a substitute for
manual reachability, ownership, concurrency, blocking, or performance analysis.

Dart/Flutter, from every generated or consuming package:

```powershell
dart format --output=none <generated Dart roots>
dart analyze
```

Use `fvm dart`/`fvm flutter` when pinned. Analyzer errors and warnings block completion; report info
diagnostics separately. Inspect generated text for stale symbols and unbraced control-flow bodies
even if the active analyzer configuration accepts them.

### 5. Perform complete manual passes

Read every ledger file end to end and apply all relevant sections of `review-checklist.md`. Make
separate passes for:

- correctness, ABI, serialization, errors, boundaries, and cancellation;
- dead, unreachable, stale, platform-shadowed, or generated-but-unused code;
- algorithmic cost, allocations, copies, FFI crossings, I/O, queues, and backpressure;
- memory/resource ownership, cleanup, handles, pointers, ports, timers, and finalizers;
- races, atomicity, goroutine/isolate lifetime, channels, lock ordering, callback re-entry, blocking,
  and deadlock;
- security, malformed input, trust boundaries, tests, build scripts, and generated/source parity.

Search globally for every public symbol, error path, lock, channel, queue, callback, and generated
entry point. Continue through the entire pass after finding a defect.

### 6. Record findings with solutions

Every finding must contain:

- severity: `P0` release blocker, `P1` correctness/data-loss, `P2` material risk, or `P3` improvement;
- exact file/line and the source-of-truth/generated relationship;
- evidence: reproducer, trace, failing vector, counterexample, or precise reasoning;
- impact and affected targets/codecs;
- resolution status: `PROPOSED`, `VERIFIED`, or `BLOCKED`;
- an actionable code/design solution plus a regression test or target vector when known;
- when `BLOCKED`: why a safe solution cannot yet be determined or applied, required missing evidence
  or authority, unsafe/rejected alternatives, and the concrete next investigation or decision.

Never suppress an issue because its solution is difficult or unknown. Never invent a confident fix
without evidence merely to fill the solution field.

### 7. Regenerate and run Native and Wasm

Use the real generator and a realistic fixture covering scalar/composite types, nil/empty/boundary
values, errors, callbacks, async calls, streams, cancellation, concurrent calls, and ownership.
Generate twice to test idempotency and inspect changed generated output.

- Native: generate, compile the actual `c-shared`/plugin artifact, analyze generated Dart, start a
  real Dart/Flutter process, and invoke representative APIs.
- Wasm: generate the web artifact using the supported `GOOS=js GOARCH=wasm` flow, analyze generated
  Dart, start the actual browser/Node/project runner, and invoke the equivalent supported APIs.

If either target cannot execute, the review remains `BLOCKED`. Follow
[native-wasm-parity.md](references/native-wasm-parity.md) to compare the shared standard-codec corpus.

### 8. Close and report

- Re-run affected analysis/tests after fixes or generation.
- Run `audit_ledger.py verify` and `git diff --check`.
- Report gate status, ledger counts, every finding, resolution state, native/Wasm artifacts, parity
  vectors, explicit exceptions, unavailable environments, and remaining hypotheses.
- If the user asked only for review, do not modify production code. If fixes are authorized, fix and
  verify findings individually, then repeat all affected gates and parity checks.

## Blocking and timeout policy

Flag unbounded or unintended waiting: lock/channel/queue waits without cancellation, waiting while
holding locks, callback re-entry deadlocks, goroutines without exit paths, `WaitGroup` lifecycle
errors, timer/ticker leaks, and FFI calls that block a Dart isolate or native worker.

Do not flag a timeout merely because a business API allows the caller to choose a deadline; that
policy belongs to the caller. Do flag internal timeouts that change public semantics, mask deadlock,
leak timers, abandon uncancelled work, or act as the only guard against a permanent internal wait.

## Report skeleton

```markdown
# Exhaustive Code Review

Scope/commit: ...
Ledger: ... (verified: yes/no; reviewed/total: ...)
Toolchain: ...

## Gates
| Gate | Evidence | Status |
| Go gopls/vet/tests/race | ... | PASS/BLOCKED |
| Generated Dart analyze | ... | PASS/BLOCKED |
| Native build and runtime | ... | PASS/BLOCKED |
| Wasm build and runtime | ... | PASS/BLOCKED |
| Shared standard-codec parity | corpus ..., exceptions ... | PASS/BLOCKED |

## Findings
### [P1] FGB-001 ...
- Location/evidence/impact: ...
- Resolution: PROPOSED/VERIFIED/BLOCKED
- Solution: ...
- Regression verification: ...
- If blocked: reason, missing prerequisite, rejected options, next step

## Coverage and limitations
...
```
