# Native/Wasm Standard-Codec Parity

## Scope

Compare observable behavior only where Native and Wasm both use the standard codec for equivalent
public operations.

Native may use CST or DCO for supported paths and fall back to the standard codec. Wasm may expose
only the standard codec. Therefore:

- do not compare CST/DCO byte layout, helper functions, transport frames, or performance with Wasm;
- do not require Wasm to implement CST/DCO;
- force or select the native standard-codec path for the shared parity corpus;
- compare results after public decoding, not incidental transport representation;
- audit native CST/DCO independently for correctness, but keep those findings outside the parity
  verdict unless their fallback behavior changes the shared standard-codec contract.

## Required target matrix

| Stage | Native | Wasm |
| --- | --- | --- |
| Generate | Real native configuration | Real web/Wasm configuration |
| Analyze Go | gopls, go vet, tests | gopls/go vet where applicable; Wasm compile tests |
| Analyze Dart | `dart analyze` in generated package | `dart analyze` in generated web package |
| Build | Actual c-shared/plugin artifact | Actual `GOOS=js GOARCH=wasm` artifact |
| Execute | Real Dart/Flutter process loads artifact | Real supported browser/Node/project runner |
| Compare | Native standard-codec run | Wasm standard-codec run |

A compiled artifact without a successful runtime call leaves that target `BLOCKED`.

## Shared corpus

Use deterministic vectors that both targets support. Include, where applicable:

- booleans, signed/unsigned integer boundaries, floating-point normal/NaN/infinity policy, strings
  including non-ASCII, bytes, enums, timestamps, and pointer-free value types;
- empty, singleton, nested, large, nil/null, optional, map, list, struct, interface, and recursive or
  cyclic rejection cases;
- success and declared errors, malformed requests, unknown methods/tags, overflow, decode failure,
  panic/error conversion, and unsupported values;
- sync and async calls, callbacks, streams, cancellation, concurrent calls, ordering, exactly-once
  completion, and cleanup after delivery failure;
- handle/resource lifecycle that is publicly observable on both targets.

Use the same logical input fixtures and assertions. Record skipped vectors with a precise unsupported
feature reason; do not silently shrink one target's corpus.

## Comparison model

Compare normalized public observations:

- decoded value and type;
- error category/code/message/details according to the public contract;
- callback/event count, content, and required ordering;
- completion/cancellation state;
- documented side effects and resource/handle state;
- process/runtime survival and absence of leaks detectable by the harness.

Normalize only explicitly nondeterministic fields such as stack addresses, platform path separators,
or timing values that are not part of the contract. Document every normalization. Never normalize a
value merely because it differs.

## Permitted target-specific exceptions

An exception is valid only when all of these are true:

1. The difference is caused by code that necessarily depends on `cgo` or `syscall/js`, not merely by
   being located in a target-specific file.
2. The report cites exact source lines/build tags and explains why a shared standard-codec behavior
   cannot or should not be identical.
3. The exception is narrow: name the affected operation, field, error, or side effect.
4. Both target-specific branches are still tested for their own documented behavior.

Do not exempt differences in standard-codec value conversion, error semantics, callback counts,
ordering guarantees, cancellation, ownership, or shared public API shape unless the contract itself
explicitly defines a cgo/syscall/js-specific outcome.

Maintain an exception table:

| ID | Operation/vector | Native observation | Wasm observation | cgo/syscall/js source evidence | Reason allowed |
| --- | --- | --- | --- | --- | --- |

An empty table is preferred. Any unexplained row is a parity finding and blocks a pass.

## Harness rules

- Generate both targets from the same Go API fixture and commit/configuration.
- Use machine-readable output (for example canonical JSON) from each runner and compare it with a
  deterministic script or assertions.
- Keep target invocation separate from comparison so crashes/timeouts cannot look like empty results.
- Fail on missing vectors, duplicate vector IDs, unexpected extra events, wrong errors, or resource
  cleanup failures.
- Run the corpus repeatedly for concurrency/order-sensitive vectors and record iteration counts.
- Save commands, versions, artifacts, raw observations, normalized observations, and comparison output
  under ignored review output.

## Verdict

Report `PASS` only when both targets executed every shared supported vector and all normalized public
observations match after documented normalization and valid exceptions. Otherwise report `BLOCKED`
or `FAIL` with findings and their solution or blocked-resolution explanation.
