# Sync and async

Unmarked functions and functions marked `//fgb:sync` generate synchronous Dart methods. Only
`//fgb:async` generates a `Future` method; a function never gets both variants and no `Sync` or
`Async` suffix is added.

```go
func Add(a, b int) int { return a + b }

//fgb:async
func LoadValue() (int, error) { return 42, nil }
```

```dart
final sum = add(a: 20, b: 22);
final value = await loadValue();
```

Async calls use Dart API DL for DCO delivery or the standard-codec async entrypoint. Stream and
callback parameters require async mode because the Dart event loop must remain available.

