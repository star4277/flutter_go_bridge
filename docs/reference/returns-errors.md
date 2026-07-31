# Returns and errors

One non-error return value stays unchanged. Multiple non-error values become a Dart record; named Go
results become named record fields, and unnamed results become positional fields.

`error` may appear anywhere in the result list. Non-nil errors are collected and surfaced as
`FgbPlatformException`; multiple errors are available through the exception's `goErrors` field.
With one error, use the exception `message` as the compatible short form.

```go
func Read() (value string, count int, err error) { return "ok", 2, nil }
```

```dart
final (value: text, count: count) = read();
```

A function returning only `error` is `void` on success. A function with no non-error values and no
error is also `void`.

