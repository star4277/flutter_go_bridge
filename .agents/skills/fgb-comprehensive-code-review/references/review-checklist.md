# Exhaustive Review Checklist

Apply every relevant section to every ledger row. Use global searches and call-graph/data-flow tracing
to connect local code to its callers, generated counterparts, platform variants, and cleanup paths.

## 1. Correctness and boundaries

- Verify success, error, nil/null, empty, minimum, maximum, overflow, malformed, duplicate, reordered,
  cancellation, and partial-progress paths.
- Check integer widths/sign, floating-point special values, UTF-8, binary data, timestamps, enums,
  pointer-sized values, collection lengths, recursion depth, and serialization limits.
- Trace every error conversion across Go, generated native/Wasm transport, and Dart. Preserve error
  identity, code, message, details, stack expectations, and exactly-once completion.
- Check that public contracts, documentation, tests, and generated APIs agree. Flag silently ignored
  inputs, default-value ambiguity, lossy conversions, stale ABI declarations, and incompatible changes.
- Validate build tags and platform files independently; code compiling on one target does not prove
  another target's branch.

## 2. Reachability and dead code

- Find unused declarations, unreachable branches, impossible conditions, redundant nil/error checks,
  shadowed platform implementations, stale exports, orphaned templates, and generated members that no
  caller can reach.
- Search symbol references across source, tests, templates, reflection/string registration, and build
  scripts before declaring code unused.
- Check build-tag combinations and generated variants. A branch reachable only under an unsupported
  tag is stale unless the project explicitly preserves that target.
- Identify conditions that are always true/false because of earlier validation, type ranges, enum
  exhaustiveness, or state transitions.
- Treat commented-out code and compatibility shims without an active consumer as findings when they
  add maintenance risk.

## 3. Performance and scalability

- Analyze asymptotic cost, repeated scans/sorts/parses, nested loops over unbounded inputs, excessive
  FFI/Wasm crossings, synchronous I/O, and serialized work that could safely run concurrently.
- Inspect allocations, copies, string/byte conversions, reflection, boxing, arena use, buffer growth,
  cache retention, and generated temporary objects.
- Check queue bounds, backpressure, batching, wakeups, goroutine/isolate creation, contention, false
  sharing, and work performed while holding locks.
- Use benchmarks, `-benchmem`, profiles, traces, or allocation evidence for material performance claims.
  Do not report micro-optimizations without an impact path.
- Ensure optimizations preserve errors, ordering, cancellation, ownership, native/Wasm standard-codec
  parity, and deterministic generation.

## 4. Concurrency, races, and atomicity

- Inventory shared mutable state and its owner. Verify every read/write uses the same synchronization
  discipline across goroutines, callbacks, C calls, Dart ports, isolates, and finalizers.
- Check map/slice/object aliasing, publication, double initialization, once semantics, atomics and
  memory ordering, check-then-act races, non-atomic compound state, and close/send races.
- Verify `WaitGroup` Add/Done/Wait ordering, channel ownership and closure, select defaults, context
  propagation, cancellation cleanup, and goroutine termination on every path.
- Run `go test -race` with stress/repetition on concurrency-sensitive packages. A clean race run does
  not replace the manual synchronization audit.
- Check callbacks re-entering code that still holds a lock or owns a non-reentrant state machine.

## 5. Blocking and deadlock

- Build a lock-order graph and inspect cycles, lock upgrade/downgrade, nested locks, deferred unlocks,
  early returns, panic paths, and waits while holding a lock.
- Inspect blocking sends/receives, full queues, nil channels, missing consumers, lost wakeups, callback
  completion, Dart-port delivery, isolate blocking, and foreign calls with unknown duration.
- Flag permanent or unintended internal waits and missing cancellation. Do not flag a caller-selected
  business deadline simply because its value is configurable.
- Do flag internal timeout use that masks deadlock, alters the API contract, leaks timers, abandons
  work without cancellation, or substitutes for a real lifecycle/ownership fix.
- Demand a reproducer or a precise schedule/lock cycle for confirmed deadlocks; label incomplete but
  credible schedules as suspected findings, not facts.

## 6. Memory and resource ownership

- Trace allocation, transfer, borrow, release, and finalization for Go memory, C memory, Dart FFI
  pointers, Wasm memory views, handles, ports, streams, files, processes, timers, and goroutines.
- Check leaks, double free/release, use-after-free, stale handles, finalizer races, pointer retention,
  ownership after errors, and cleanup when message delivery fails.
- Verify zero-length allocations, alignment, struct layout, pointer width, endianness, and lifetime
  across native ABI and Wasm memory.
- Ensure cleanup is idempotent where retry/finalizer overlap is possible.

## 7. Generated code and source of truth

- Review the generator/parser/runtime source and the generated result. Do not fix generated output
  directly when the generator owns it.
- Generate twice and compare output for determinism, stale-file removal, ordering, stable names,
  imports, formatting, and braces around control-flow bodies.
- Compile generated Go for each supported target, run `gopls`/`go vet`, and analyze every generated
  Dart package.
- Check identifier collisions, reserved words, duplicate exports, unsupported types, cyclic types,
  generic/alias behavior, package boundaries, and error messages that identify the user declaration.
- Exercise both call directions, failure paths, callbacks, streams, sync/async operations, and cleanup.

## 8. Native and Wasm behavior

- Keep codec layers distinct: Native may use CST/DCO and standard fallback; Wasm may use only standard
  codec. Do not require CST/DCO internals or byte layouts to exist on Wasm.
- Compare the public behavior of equivalent operations that both targets execute through the standard
  codec. Follow `native-wasm-parity.md` for normalization and exceptions.
- Inspect `cgo` and `syscall/js` branches for their own correctness even when their necessarily
  target-specific behavior is excluded from parity.
- Treat any non-allowlisted standard-codec difference as a finding: values, error classification,
  ordering, callback count, cancellation, resource lifecycle, or observable side effects.

## 9. Security and robustness

- Validate external lengths, indexes, handles, method IDs, message tags, paths, commands, environment
  values, and generated identifiers before use.
- Check injection, path traversal, unsafe process execution, untrusted deserialization, resource
  exhaustion, panic/abort exposure, information leaks, and denial-of-service inputs.
- Verify secrets and pointer values are not logged unintentionally; check temporary file permissions
  and cleanup.
- Distinguish library-owned validation from caller-owned business policy.

## 10. Tests and evidence quality

- Map each public behavior and each finding to tests covering success, failure, boundaries, platform
  variants, concurrency schedules, and resource cleanup.
- Reject tests that only compile, never assert a value, swallow errors, depend on ordering accidentally,
  or cannot fail when the bug is present.
- Separate unit, race/stress, generation, compile, runtime smoke, and native/Wasm parity evidence.
- Reproduce flaky failures with repetition and capture seeds/schedules when possible.

## Finding solution rule

For every finding, provide the smallest safe remediation that addresses the cause rather than only
the symptom, plus a regression test or runtime vector. If no reliable solution can yet be stated or
implemented, retain the finding as `BLOCKED` and document:

1. why the root cause or safe fix is still uncertain;
2. what evidence, environment, decision, or authorization is missing;
3. which tempting approaches were rejected and why they are unsafe or incomplete;
4. the next concrete experiment, trace, design decision, or platform run needed to unblock it.
