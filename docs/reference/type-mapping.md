# Type mapping

| Go | Dart |
| --- | --- |
| `bool`, integers, floats, `string` | Dart scalar |
| `[]T`, `[N]T` | `List<T>` |
| `[]byte` | `Uint8List` |
| `map[K]V`, `any` | `Map<K, V>`, `Object?` via standard codec |
| Serializable struct | Generated value class |
| `*T` | Nullable `T?` |
| `time.Time` / `time.Duration` | `DateTime` / `Duration` |
| `math/big.Int` | `BigInt` |
| `net/netip.Addr` / `net/url.URL` | `InternetAddress` / `Uri` |
| `uuid.UUID` | `UuidValue` (adds the `uuid` Dart package when needed) |
| `error` result | `FgbPlatformException` |
| `chan<- T` / `fgb.StreamSink[T]` | `Stream<T>` or `StreamSink<T>` |
| `context.Context` | Omitted from Dart signature; generated cancellation context |
| `fgb.DartOpaque` | `Object` handle |
| Unsupported/opaque struct | `GoOpaque` handle |

Private identifiers are ignored. Generic functions, variadic parameters, nested function types and
complex external named types are not supported.

