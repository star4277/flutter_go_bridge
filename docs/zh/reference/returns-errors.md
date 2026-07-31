# 返回值与 error

单个非 error 返回值保持原类型。多个非 error 返回值生成 Dart record：Go 结果有名字时生成命名
record，否则生成位置 record。

`error` 可以出现在任意位置，并且可以有多个。所有非 nil error 会汇总为
`FgbPlatformException`；多个 error 可从异常的 `goErrors` 字段读取。只有一个 error 时直接使用
`message`。

```go
func Read() (value string, count int, err error) { return "ok", 2, nil }
```

```dart
final (value: text, count: count) = read();
```

只返回 `error` 的函数成功时是 Dart `void`。

