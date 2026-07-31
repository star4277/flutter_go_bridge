# Returns and errors

The generator removes every Go `error` result first, then chooses the Dart return type from the
remaining values. Errors never appear as Dart values or record fields. Any non-nil error fails the
whole call with `FgbPlatformException`.

| Non-error Go results | Dart return type |
| --- | --- |
| 0 | `void` |
| 1 | The mapped Dart type directly |
| 2 or more, unnamed | Positional record `(T1, T2, ...)` |
| 2 or more, named | Named record `({T1 name1, T2 name2, ...})` |

`//fgb:async` preserves that shape and wraps it in `Future`, such as `Future<int>` or
`Future<(String, int)>`.

## Zero and one value

```go
func Reset() {}
func Flush() error { return nil }
func Add(a, b int) int { return a + b }
func Find(id int64) (User, error) { /* ... */ }
```

```dart
void reset()
void flush()
int add({required int a, required int b})
User find({required int id})
```

A function returning only `error` is `void` on success and throws on failure. One non-error value
always stays plain, even when the Go result is named; it never becomes a one-field record.

## Positional records

Two or more unnamed values become a positional record:

```go
func Divide(value, divisor int) (int, int, error) {
	if divisor == 0 {
		return 0, 0, errors.New("division by zero")
	}
	return value / divisor, value % divisor, nil
}
```

```dart
(int, int) divide({required int value, required int divisor})

final (quotient, remainder) = divide(value: 11, divisor: 4);
```

The values travel as one wire list. Dart validates its length and reconstructs the record in the
original non-error Go result order.

## Named records

When all non-error results are named, the generator emits a named record:

```go
func Split(value string) (head string, tail string, err error) { /* ... */ }
```

```dart
({String head, String tail}) split({required String value})

final result = split(value: 'a/b');
print(result.head);
```

Only non-error names affect this decision. Names become lowerCamelCase and are uniquified after
conversion if necessary. If any non-error result is unnamed or `_`, the multi-value result is a
positional record instead.

## Error positions

`error` may appear anywhere, and several error slots are allowed:

```go
func Load(id int64) (error, User) { /* ... */ }
func Analyze(input string) (error, string, int, error) { /* ... */ }
```

Their Dart results are `User` and `(String, int)`. Error slots are removed while all other results
keep their relative order.

## One error result

For a call declaring one error result:

- nil means the normal result is returned;
- non-nil throws `FgbPlatformException`;
- `code == 'go_error'`;
- `message == err.Error()`;
- `goErrors == null`, so use `message` directly.

```dart
try {
  final file = open(path: path);
} on FgbPlatformException catch (error) {
  if (error.code == 'go_error') {
    print(error.message);
  }
}
```

## Several error results

For a function declaring several error results, the bridge inspects every slot in declaration order,
omits nil errors, and succeeds only when all are nil. Otherwise it throws one exception whose:

- `code` is `go_error`;
- `message` joins messages with `; `;
- `goErrors.messages` contains each non-nil error separately and in order.

Even when only one of several declared errors is non-nil, `goErrors` contains that one message.

```dart
try {
  final normalized = validate(input: text);
} on FgbPlatformException catch (error) {
  if (error.code != 'go_error') rethrow;

  print(error.message);
  for (final message in error.goErrors?.messages ?? const <String>[]) {
    print(message);
  }
}
```

## `FgbPlatformException` and `FgbGoErrors`

```dart
final class FgbPlatformException implements Exception {
  final String code;
  final String? message;
  final Object? details;
  final FgbGoErrors? goErrors;
}
```

`FgbGoErrors` exposes an unmodifiable `messages` list, `length`, and `operator []`. It is populated
for calls that declare several error results.

Prefer `goErrors` over parsing `details`. The standard codec sends a map similar to
`{method: ..., errors: [...]}`, while the CST/DCO path has no map type and sends the message list
directly. The generated runtime normalizes both into `goErrors`.

## No partial success

If any error is non-nil, all ordinary values are discarded. A call such as:

```go
func Import() (created int, skipped int, err error)
```

returns `({int created, int skipped})` on success and only an exception on failure. To return partial
data with non-fatal warnings, put the warnings in an ordinary result struct and reserve `error` for
fatal failure.

## Synchronous and asynchronous throws

A synchronous generated function throws during the call. An async function completes its `Future`
with an error, so the exception is observed at `await`:

```dart
try {
  final value = readNow();
} on FgbPlatformException catch (error) {
  // Immediate synchronous failure.
}

try {
  final value = await readLater();
} on FgbPlatformException catch (error) {
  // Asynchronous failure at await.
}
```

Without `await`, the surrounding synchronous `try/catch` does not catch a later Future failure.

## Business errors versus bridge errors

Not every `FgbPlatformException` is an explicit Go business error:

| Code | Source |
| --- | --- |
| `go_error` | A Go function or method returned non-nil `error` |
| `panic` | Go code or a callback path panicked and the dispatcher recovered |
| `invalid_argument` | An argument, receiver, or handle could not decode to the target Go type |
| `encode_error` | A normal Go result could not be encoded back to Dart |
| `invalid_request` | The low-level method envelope or FFI input was invalid |
| `method_not_found` | The loaded Go bridge does not know the generated method/call id |
| `bridge_error` | An internal bridge step failed while constructing a response |
| `stream_error` | Go added an error event to a stream; see [Stream](/reference/stream) |

Check `code == 'go_error'` before treating an exception as a business validation failure. Other
codes generally indicate integration or runtime faults. Standard-codec panic details may include a
Go stack and should be used for diagnostics, not shown directly to end users.

## `FormatException` and protocol mismatches

Some failures happen in local Dart decoding rather than in a Go error envelope, so they throw
`FormatException`. Examples include a wrong multi-result list length, malformed envelope values,
an unknown interface implementation tag, a nil named-interface result, or a Go library and Dart tree
generated at different times.

Regenerate both sides and deploy the matching dynamic library and Dart files together before treating
these as application errors.

## Stream and callback special cases

An async function with exactly one owned stream sink/channel and no ordinary value returns
`Stream<T>` rather than `Future<void>`. See [Stream](/reference/stream) for error events and lifecycle.

Dart closure callback results use their callback protocol. If a callback signature has no trailing
error result, Dart/transport failure may panic in the synthesized Go callback and then be recovered by
the outer async dispatcher as `FgbPlatformException`. See [Dart closure callbacks](/reference/callbacks).

## Common issues

| Symptom | Resolution |
| --- | --- |
| Expected a record but received a plain value | There is only one non-error result |
| Named results produced a positional record | A non-error result is unnamed or `_` |
| Need individual messages from several errors | Read `error.goErrors?.messages`; do not split `message` |
| `goErrors` is null | The call declares only one error; use `message` |
| Normal results disappeared | Any non-nil error discards every ordinary result |
| Async error was not caught | `await` the Future inside `try` |
| `method_not_found` or `FormatException` | Regenerate and deploy matching Go and Dart artifacts |
| Need warnings plus data | Return warnings as ordinary structured data, not `error` |
