# Stream

Go can push a sequence of values to Dart through a direct `chan<- T` or
`fgb.StreamSink[T]` parameter on an async function.

| Form | Best for | Error events | Produce after return | Close |
| --- | --- | --- | --- | --- |
| `chan<- T` | Values produced before the call returns | No | No | Bridge closes it |
| `fgb.StreamSink[T]` | Explicit lifecycle and background producers | `AddError` | Yes | `Close` |

Both forms require `//fgb:async`, cannot be returned to Dart, cannot be nullable, and require a
bridgeable element type.

## Send-only channel

```go
//fgb:async
func Ticks(count int, out chan<- int) error {
	for i := 0; i < count; i++ { out <- i }
	return nil
}
```

With one stream parameter and no non-error result, Dart gets
`Stream<int> ticks({required int count})`. The bridge creates a buffered channel (capacity 16),
drains it on a goroutine, and closes it when the function returns. Do not retain it for a background
producer; use `StreamSink` instead.

Only `chan<- T` is accepted. Bidirectional and receive-only channels are rejected. After Dart
cancels, the drain goroutine keeps consuming and drops values so a producer cannot block forever.
The channel form has no error-return path; an item that cannot be encoded is dropped.

## `fgb.StreamSink<T>`

```go
//fgb:async
func Ticks(count int, sink fgb.StreamSink[int]) error {
	defer sink.Close()
	for i := 0; i < count; i++ {
		if err := sink.Add(i); err != nil {
			if errors.Is(err, fgb.ErrStreamClosed) { return nil }
			return err
		}
	}
	return nil
}
```

The generated support package provides:

- `Add(T) error`: encode and post an item without waiting for the listener;
- `AddError(error) error`: add a `stream_error` `FgbPlatformException` without closing;
- `Close()`: idempotently complete the stream;
- `IsClosed() bool`: report Close, cancellation, controller disposal, or retired isolate.

`ErrStreamClosed` means production should stop. Sink copies share one state and are safe across
goroutines. The last Go reference being collected also completes the Dart stream, but normal code
should close explicitly.

## Ownership and API shape

The generated call returns `Stream<T>` when it has exactly one direct sink/channel parameter and no
non-error result. The sink parameter is hidden. This is a cold, single-subscription stream: Go starts
on first listen, not when the function returns the Stream. Cancellation retires the sink.

If the call returns a value, has multiple stream parameters, or stores a sink in a struct, Dart
creates the `StreamController` and passes its `sink` into a `Future<R>` API. The same Dart sink
passed more than once reuses one handle. Only `StreamSink<T>`, not a channel, may be a struct field.

## Cancellation with context

An exact `context.Context` parameter is omitted from Dart and supplied by the bridge. Cancelling the
subscription or closing the sink cancels that context. At most one context is supported. Producers
must cooperate by selecting on `ctx.Done()`; cancellation cannot forcibly stop a goroutine.

With multiple streams, the call context is associated with the first stream parameter.

## Errors and completion

- `AddError` adds an in-band stream error and leaves the stream open.
- A failed Go-owned call is surfaced as a stream error and closes the controller.
- A Dart-owned call still reports ordinary function failure through its returned Future.
- `Close`, channel return, Dart cancellation, or last-sink cleanup complete/retire the stream.

## Hot restart

Each Dart isolate attachment gets a stream generation. A hot restart increments it, cancels previous
contexts, rejects old sink posts with `ErrStreamClosed`, and drops old channel events. Old producers
cannot deliver into a new isolate that reused the same numeric handle.

## Restrictions

- `//fgb:async` is required;
- only `chan<- T` is accepted;
- sinks cannot be results or nullable parameters;
- channels cannot be struct fields;
- nested stream sinks are unsupported;
- Go-owned streams are single-subscription;
- channel producers must finish before the outer function returns.

