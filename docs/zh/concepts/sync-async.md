# 同步与异步

未标记函数和 `//fgb:sync` 只生成同步 Dart 方法；只有 `//fgb:async` 生成 `Future`。
同一函数不会同时生成两个版本，方法名也不增加 `Sync`/`Async` 后缀。

```go
func Add(a, b int) int { return a + b }

//fgb:async
func LoadValue() (int, error) { return 42, nil }
```

```dart
final sum = add(a: 20, b: 22);
final value = await loadValue();
```

Stream 和 Dart 回调参数要求异步模式，因为调用期间必须保持 Dart 事件循环可用。

