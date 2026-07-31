# 类型映射

| Go | Dart |
| --- | --- |
| `bool`、整数、浮点、`string` | Dart 标量 |
| `[]T`、`[N]T` | `List<T>` |
| `[]byte` | `Uint8List` |
| `map[K]V`、`any` | `Map<K,V>`、`Object?`，走 standard codec |
| 可序列化结构体 | 生成的 value class |
| `*T` | 可空 `T?` |
| `time.Time` / `time.Duration` | `DateTime` / `Duration` |
| `math/big.Int` | `BigInt` |
| `net/netip.Addr` / `net/url.URL` | `InternetAddress` / `Uri` |
| `uuid.UUID` | `UuidValue`，需要时自动添加 `uuid` 依赖 |
| `error` 返回值 | `FgbPlatformException` |
| `chan<- T` / `fgb.StreamSink[T]` | `Stream<T>` 或 `StreamSink<T>` |
| `context.Context` | 不出现在 Dart 签名，由 runtime 创建 |
| `fgb.DartOpaque` | Dart `Object` 句柄 |
| 不可翻译/opaque 结构体 | `GoOpaque` 句柄 |

私有标识符不参与生成。泛型函数、可变参数、嵌套函数类型和复杂外部命名类型暂不支持。

