# Dart closure callbacks

An async Go function or method may accept a direct function parameter. Dart supplies either a sync
closure or an async closure:

```go
//fgb:async
func Transform(input string, mapper func(string) string) string {
	return mapper(input) + "!"
}
```

```dart
await transform(input: 'go', mapper: (text) => text.toUpperCase());
await transform(input: 'go', mapper: (text) async => loadReplacement(text));
```

## Execution model

When Go invokes the synthesized function, it encodes arguments, posts a callback request to the Dart
event loop, parks that Go goroutine, and resumes it after Dart replies. This requires
`//fgb:async`; a synchronous FFI call would block the event loop that must run the closure.

Dart types use `FutureOr<R> Function(...)`. `Future.sync` and `await` normalize immediate values,
Futures, synchronous throws, and Future errors.

## Supported signatures

Callbacks may have any number of bridgeable parameters and one of these result shapes:

- `func()`;
- `func(A, B)`;
- `func(A) R`;
- `func() error`;
- `func(A) (R, error)`.

There may be at most one non-error result, and error must be final. Dart never returns a Go error
value directly: throwing maps to the trailing error when present.

Without a trailing error, Dart/transport/codec failure panics inside the synthesized callback. The
outer async dispatcher recovers it and fails the original Dart call with `FgbPlatformException`.
With a trailing error, Go receives the zero result plus a non-nil error and can handle it normally.

## Nullable callbacks

```go
//fgb:async, nullable = "onProgress"
func Download(url string, onProgress func(int64, int64)) error
```

Dart gets an optional nullable closure. Omitted/null becomes a nil Go func, which user code must
check before calling.

## Lifetime, concurrency, and blocking

The Go function value keeps the Dart closure registered, so Go may retain it after the outer call.
When the last Go reference becomes unreachable, cleanup releases the Dart registry entry. Clear
long-lived global references explicitly when no longer needed.

Multiple goroutines may invoke the same callback; each request has its own ID and waiter. Every call
blocks its initiating goroutine until Dart completes. There is no built-in callback timeout or
cancellation, so apply timeouts in the Dart closure or design a cancellation protocol.

Callbacks belong to the Dart isolate that created them. Re-register callbacks after hot restart
instead of invoking handles retained from the destroyed isolate.

## Restrictions

Callbacks are supported only as direct function/method parameters. They cannot be:

- returned from Go;
- nested in slices or maps;
- struct fields;
- callback parameters of another callback;
- generic or variadic function types.

A callback supports at most one non-error result, with an optional final error. Its parameter and
result types must otherwise be bridgeable. Prefer Stream for high-frequency one-way events; use a
callback when Go needs a Dart-computed reply.

