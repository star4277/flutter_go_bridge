# 类型映射

本页列出生成器当前支持的 Go → Dart 类型映射。表格中的类型是生成 API 里实际出现的 Dart 类型；
指针、nullable tag、Stream 和 callback 还会根据所在位置改变参数是否可空、是否 required，以及函数
最终返回 `Future` 还是 `Stream`。

## 基础类型

| Go | Dart | 说明 |
| --- | --- | --- |
| `bool` | `bool` | 直接映射 |
| `string` | `String` | 直接映射 |
| `int8`<br>`int16`<br>`int32`<br>`int64`<br>`int` | `int` | Dart → Go 时按目标 Go 类型做有符号范围检查 |
| `uint8`<br>`uint16`<br>`uint32` | `int` | Dart → Go 时拒绝负数和超出目标位宽的值 |
| `uint64`<br>`uint`<br>`uintptr` | `BigInt` | 使用无符号整数语义，避免 Dart `int` 范围或平台位宽差异 |
| `float32`<br>`float64` | `double` | `float32` 输入在 Go 端转换为 32 位精度 |

不支持 `complex64`、`complex128` 和 `unsafe.Pointer` 作为普通桥接类型。

### 命名基础类型

Go 命名类型不会直接退化成底层 Dart 标量，而是生成 Dart extension type，以保留 API 中的类型名称：

```go
type UserID int64
type Status string
```

```dart
extension type const UserID(int value) {}
extension type const Status(String value) {}
```

命名类型上导出的 Go 方法会生成在对应 extension type 中；同类型常量也会生成静态常量或静态 final
值。其 wire 编码仍使用底层类型。

## slice、数组与 typed list

| Go | Dart | 说明 |
| --- | --- | --- |
| `[]byte`<br>`[]uint8` | `Uint8List` | `byte` 是 `uint8` 的别名，因此两种写法相同 |
| `[]int32` | `Int32List` | 使用 Dart typed data |
| `[]int64` | `Int64List` | 使用 Dart typed data |
| `[]float64` | `Float64List` | 使用 Dart typed data |
| `[]T` | `List<T>` | 上面四种专用 slice 除外，元素递归按本页规则映射 |
| `[N]T` | `List<T>` | Dart → Go 时严格检查长度必须为 `N` |

普通 slice 参数默认生成非空且 `required` 的 `List<T>`。若需要区分 Go 的 nil slice 与空 slice，
在参数上使用 `//fgb:nullable`，字段上使用 `fgb:"nullable"`。指针 slice 也会自然生成可空类型。

未标记 nullable 时，Go → Dart 的 nil slice 会归一化为空列表或空 typed list。

## map 与动态值

| Go | Dart | 说明 |
| --- | --- | --- |
| `map[K]V` | `Map<K, V>` | key 和 value 递归映射；map 走 standard codec |
| `any`<br>`interface{}` | `Object?` | 运行时动态值，走 standard codec |

当前 map key 支持以下映射类别：

- `bool`、`string`；
- 所有受支持的整数和浮点类型；
- `time.Duration`；
- `net/netip.Prefix`；
- `net/url.URL`；
- 底层类型同样可作为 map key 的 Go 命名类型。

结构体、slice、map、`BigInt`、`InternetAddress`、`UuidValue`、接口和 opaque handle 不能作为 map key。
生成器会在 codegen 阶段报错，不会等到运行时。

`any` 只支持 standard codec 能表达的动态值图。不要把任意 Go 对象或函数值放入 `any`，否则结果
编码会失败。

## 指针与可空类型

| Go | Dart | 说明 |
| --- | --- | --- |
| `*T` | `T?` | nil 与 null 双向对应；嵌套指针 `**T` 不支持 |
| `*struct` | `XXX?` | 普通结构体仍按字段传输，不会仅因使用指针就变成 opaque |
| `*OpaqueType` | `OpaqueType?` | Dart 保存 Go handle，而不是结构体字段 |

例如：

```go
type User struct {
	Name string
}

func FindUser(id int64) *User
```

```dart
final class User {
  const User({required this.name});
  final String name;
}

User? findUser({required int id})
```

Go 指针表示可空，并不表示共享内存。普通 `*User` 返回值解码后仍是一个 Dart value object；只有
`GoOpaque` 使用可持续解析到同一 Go 对象的 handle。

## 标准库与专用类型

| Go | Dart | wire 与边界行为 |
| --- | --- | --- |
| `time.Time` | `DateTime` | wire 使用 RFC3339Nano 字符串；Dart 使用 `DateTime.parse` |
| `time.Duration` | `Duration` | wire 使用微秒；Go → Dart 会截断不足 1 微秒的纳秒部分 |
| `math/big.Int` | `BigInt` | 使用无损大整数编码；`*big.Int` 映射为 `BigInt?` |
| `net/netip.Addr` | `InternetAddress` | wire 使用 IP 文本；零值地址与空字符串互转；需要 `dart:io` |
| `net/netip.Prefix` | `String` | wire 使用 CIDR 文本，例如 `192.168.1.0/24`；零值或非法 Prefix 与空字符串互转 |
| `net/url.URL` | `Uri` | wire 使用 `URL.String()`；Dart 使用 `Uri.parse` |
| `github.com/gofrs/uuid/v5.UUID` | `UuidValue` | wire 使用 UUID 字符串；需要 Dart `uuid` package |

上表中的值类型使用 Go 指针时都会得到对应的 Dart 可空类型，例如：

```go
func ParseRoute(addr *netip.Addr, prefix *netip.Prefix) (*url.URL, error)
```

```dart
Uri? parseRoute({InternetAddress? addr, String? prefix})
```

### `net/netip.Prefix`

`netip.Prefix` 映射成 `String`，而不是 `InternetAddress`，因为 Dart 的 `InternetAddress` 只表示地址，
不携带 CIDR 前缀长度：

```go
type Route struct {
	Destination netip.Prefix
}
```

```dart
final class Route {
  const Route({required this.destination});
  final String destination;
}
```

Dart → Go 时使用 `netip.ParsePrefix` 校验字符串。空字符串会还原为 `netip.Prefix{}`；Go → Dart 时
无效 Prefix 也编码为空字符串。非空非法 CIDR 会成为 `invalid_argument`。

### UUID 依赖

生成代码首次使用 `uuid.UUID` 时会导入：

```dart
import 'package:uuid/uuid.dart';
```

若项目 `pubspec.yaml` 尚未声明 `uuid`，codegen 会通过 Flutter/FVM 执行 `flutter pub add uuid`。
文档站使用 Bun，不影响生成目标 Flutter/Dart 项目的依赖管理方式。

## 结构体与命名接口

| Go | Dart | 说明 |
| --- | --- | --- |
| `type XXX struct { ... }` | `class XXX` | 所有参与桥接的字段都可映射时，生成 value class |
| `type XXX struct { ... }`<br>`//fgb:opaque` | `class XXX extends GoOpaque` | Go 保存对象，Dart 保存 handle；导出签名必须使用 `*XXX` |
| `type XXX interface { ... }` | `abstract interface class XXX` | 具体生成类型使用 `implements XXX` |
| `struct{}` | `class XXX` | 命名空结构体生成带空 `const` 构造器的 value class |

普通 value struct 按字段编码，Dart 类包含命名构造参数。不可传输字段、仅私有状态或显式
`//fgb:opaque` 会使结构体使用 `GoOpaque`。匿名嵌入值结构体会映射为 Dart `extends`，被提升字段在
wire 上扁平化。

命名非空接口使用 `[实现序号, 载荷]` 的 tagged union，并走 standard codec。完整分类、继承、实现
发现和限制见[结构体与接口](/zh/reference/structs-interfaces)。

## atomic 包装类型

| Go | Dart | 说明 |
| --- | --- | --- |
| `sync/atomic.Bool` | `bool` | 使用 Go `Load()` 读取，解码时使用 `Store()` 写入 |
| `sync/atomic.Int32`<br>`sync/atomic.Int64` | `int` | 映射为 `Load()` 返回的基础类型 |
| `sync/atomic.Uint32`<br>`sync/atomic.Uint64`<br>`sync/atomic.Uintptr` | `int` 或 `BigInt` | 由 `Load()` 的具体返回类型决定 |
| `atomic` 包中的兼容包装类型 | `T` | 需提供 `Load() T` 与 `Store(T)`，且 `T` 是受支持基础类型 |

识别依据是行为和包名，不只硬编码标准库类型：包名必须是 `atomic`，pointer method set 中必须存在
匹配的 `Load`/`Store`。atomic 值按一次快照传输，不会让 Dart 和 Go 共享原子变量。

## error

| Go | Dart | 说明 |
| --- | --- | --- |
| `error` | `FgbPlatformException` | error slot 不进入 Dart 返回类型；非 nil 时抛出异常 |

`error` 可以位于任意返回位置，也可以声明多个。单个非 nil error 使用 `message`；多个 error 使用
`goErrors` 保存逐条消息。详见[返回值与 error](/zh/reference/returns-errors)。

## Stream、callback 与上下文

这些类型的 Dart 形状取决于它们所在的函数，而不只是类型本身：

| Go | Dart | 说明 |
| --- | --- | --- |
| `chan<- T` | `StreamSink<T>` | 只能作为 `//fgb:async` 函数的直接参数 |
| `fgb.StreamSink[T]` | `StreamSink<T>` | 支持 Add、AddError、Close 等 bridge sink 操作 |
| `func(A, B) R` | `FutureOr<R> Function(A, B)` | 只能作为 `//fgb:async` 函数的直接参数 |
| `context.Context` | 不出现在 Dart 签名中 | runtime 创建；Stream 取消订阅时触发 cancel |
| `fgb.DartOpaque` | `Object` | Dart 对象以 registry handle 穿过 Go |
| `*fgb.DartOpaque` | `Object?` | handle 0/null 表示没有 Dart 对象 |

当 async 函数恰好拥有一个 `chan<- T` 或 `fgb.StreamSink[T]`，并且没有普通返回值时，生成 API 会
直接返回 `Stream<T>`。否则 sink 作为 `StreamSink<T>` 参数存在。详细生命周期见
[Stream](/zh/reference/stream)。

callback 允许 Dart 传同步或异步闭包，因此使用 `FutureOr<R>`。当前不支持嵌套函数类型、泛型
callback 和可变参数 callback。详见[Dart 闭包回调](/zh/reference/callbacks)。

## nullable 规则

并非所有可以在 Go 中为 nil 的类型都会默认生成 Dart nullable 参数：

| Go 形状 | 默认 Dart 参数 | 标记 nullable 后 |
| --- | --- | --- |
| `*T` | `T?`，可省略 | 指针已经可空，不需要标记 |
| `[]T` | `required List<T>` | `List<T>?`，可省略并保留 nil |
| `map[K]V` | `required Map<K, V>` | `Map<K, V>?`，可省略并保留 nil |
| `func(...)` | `required FutureOr<...> Function(...)` | 可空 callback，可省略 |
| 命名接口 | `required Interface` | `Interface?`，可省略 |

函数参数使用 `//fgb:nullable = "name"`，结构体字段使用 `fgb:"nullable"`。标量、字符串、数组和值
结构体本身不能直接为 nil；需要可空时应使用 Go 指针。完整规则见[指令与字段 tag](/zh/reference/directives)。

## 当前不支持的类型形状

- 泛型函数和带类型参数的 callback；
- 可变参数函数和可变参数 callback；
- `complex64`、`complex128`、`unsafe.Pointer`；
- 嵌套指针，例如 `**T`；
- 未命名非空接口；
- 作为非直接参数出现的函数类型，以及嵌套函数类型；
- 双向 channel 和只接收 channel；Stream 只支持直接参数中的 `chan<- T`；
- 无法递归映射字段的复杂外部类型；它们可能报错或按结构体规则退化为 `GoOpaque`。

私有函数、方法、类型和常量不进入生成 API；结构体私有字段也不会跨 bridge。
