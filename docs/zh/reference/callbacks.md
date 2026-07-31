# Dart 闭包回调

Go API 可以直接接收函数类型参数，Dart 侧传入普通闭包或 async 闭包。生成器负责注册闭包句柄、
编码参数、切回 Dart 事件循环执行、等待结果并把结果解码回 Go。

```go
//fgb:async
func Transform(
	input string,
	mapper func(string) string,
) string {
	return mapper(input) + "!"
}
```

```dart
final syncResult = await transform(
  input: 'go',
  mapper: (text) => text.toUpperCase(),
);

final asyncResult = await transform(
  input: 'go',
  mapper: (text) async => await loadReplacement(text),
);
```

## 为什么必须使用 `//fgb:async`

Go 调用 Dart 闭包时会：

1. 把回调参数编码；
2. 向 Dart callback port 投递请求；
3. 暂停当前 Go goroutine；
4. 等待 Dart 执行闭包并回传结果；
5. 解码结果并恢复 goroutine。

同步 FFI 调用期间，Dart isolate 正阻塞在 Go 调用中，无法处理 callback port，请求和调用方会互相
等待形成死锁。因此带函数参数的外层函数必须标记：

```go
//fgb:async
func UseCallback(callback func()) {}
```

缺少指令时生成器会直接报错，而不是生成一个运行时必死锁的 API。

## Dart 类型为什么是 `FutureOr`

Go 的普通函数类型没有“同步闭包”和“异步闭包”之分。生成 Dart 参数统一使用：

```dart
FutureOr<R> Function(...)
```

runtime 内部通过 `Future.sync` 调用并 `await` 结果，因此同时接受：

- 立即返回 `R` 的同步闭包；
- 返回 `Future<R>` 的 async 闭包；
- 同步 throw；
- Future 异步完成时抛错。

Go 调用方始终看到普通阻塞函数：调用 callback 的 goroutine 会等到 Dart 的同步值或 Future 完成。

## 支持的回调签名

回调支持零个或多个可桥接参数，并支持以下返回形态：

| Go 回调类型 | Dart 闭包类型 | 成功时 Go 得到 |
| --- | --- | --- |
| `func()` | `FutureOr<void> Function()` | 无返回值 |
| `func(A, B)` | `FutureOr<void> Function(A, B)` | 无返回值 |
| `func(A) R` | `FutureOr<R> Function(A)` | Dart 返回的 R |
| `func() error` | `FutureOr<void> Function()` | 成功为 nil；Dart 异常变成 error |
| `func(A) (R, error)` | `FutureOr<R> Function(A)` | 成功为 `R, nil`；Dart 异常变成零值和 error |

注意 Dart 闭包签名不会出现 Go 的 `error`。Dart 使用 throw/Future error 表达失败，bridge 决定是
转换成 Go error 还是让外层调用失败。

### 返回值限制

- 最多一个非 `error` 返回值；
- `error` 如果存在，必须是最后一个结果；
- 不支持多值 record 风格的 callback 返回；
- 回调参数和返回值本身都必须是可桥接类型。

```go
func Good1() {}
func Good2(int) string
func Good3(string) (User, error)

func Bad1() (int, string)       // 多个非 error 返回值
func Bad2() (error, string)     // error 不在最后
```

## 参数传递方式

外层生成 Dart API 的参数使用命名参数，但闭包自身的参数保持 Dart 函数类型的**位置参数**：

```go
//fgb:async
func Visit(callback func(index int, name string) bool) {}
```

生成形态：

```dart
Future<void> visit({
  required FutureOr<bool> Function(int, String) callback,
})
```

Go 中 callback 参数名只用于源码可读性，不会变成 Dart 命名参数。

回调每次执行时，参数通过生成的 standard codec 通道从 Go → Dart；返回值再从 Dart → Go。外层函数
本身仍会根据签名选择 CST/DCO 或 fallback codec，回调通道与外层调用 codec 是两个独立层次。

## 异常与 Go `error` 的映射

### 回调末尾带 `error`

推荐需要恢复的回调使用该形式：

```go
//fgb:async
func Process(mapper func(string) (string, error)) (string, error) {
	return mapper("input")
}
```

Dart：

```dart
await process(
  mapper: (value) {
    if (value.isEmpty) {
      throw StateError('empty value');
    }
    return value.toUpperCase();
  },
);
```

Dart 闭包 throw、Future error、参数/返回值编解码错误或 callback transport 错误都会让生成的 Go
函数返回：

- 非 error 结果的零值；
- 一个非 nil `error`。

Go 业务代码可以正常判断、包装或返回该错误。

### 回调没有 `error` 结果

```go
//fgb:async
func Process(mapper func(string) string) string {
	return mapper("input")
}
```

如果 Dart 闭包失败，Go 签名没有位置返回 error，生成 callback 会 panic。外层 async dispatcher
会 recover 该 panic，并让最初的 Dart API 调用以 `FgbPlatformException` 失败。

这不会让整个 Go 进程崩溃，但业务代码无法在 callback 调用点用普通 `if err != nil` 处理。因此，
预期闭包可能失败时，优先设计为 `func(...) (R, error)` 或 `func(...) error`。

### 错误消息

Dart 侧异常会以字符串形式跨 callback envelope 传输，Go 侧错误包含
`dart callback failed: ...`。原始 Dart 异常对象和完整类型不会直接进入 Go。

## 可空回调

函数值在 Go 中可以为 nil。使用 `nullable` 指令后，Dart 参数变为可空且非 required：

```go
//fgb:async, nullable = "onProgress"
func Download(
	url string,
	onProgress func(received int64, total int64),
) error {
	if onProgress != nil {
		onProgress(10, 100)
	}
	return nil
}
```

生成形态：

```dart
Future<void> download({
  required String url,
  FutureOr<void> Function(int, int)? onProgress,
})
```

Dart 省略参数或传 null 时，Go 收到 nil func。Go 代码必须像普通可空函数值一样在调用前判空；
直接调用 nil callback 会产生普通 Go panic，并由外层 dispatcher 转成调用失败。

`nullable` 中使用的是 Go 参数名，不是生成后的 Dart 参数名。

## 生命周期与长期保存

Dart 闭包注册后得到一个 handle，Go 收到的合成函数会持有该 handle 的引用。只要 Go 仍保存函数值，
Dart registry 就会保持原闭包存活：

```go
var saved func(string)

//fgb:async
func Register(callback func(string)) {
	saved = callback
}

//fgb:async
func Emit(value string) {
	if saved != nil {
		saved(value)
	}
}

func Clear() {
	saved = nil
}
```

当 Go 最后一份 callback 引用不可达并被 GC 后，cleanup 会通知 Dart 删除 registry 项。无需手动
dispose，但如果业务把 callback 存在全局变量中，它会一直保留 Dart 闭包及其捕获对象；不再使用时
应主动清空 Go 引用。

生命周期释放依赖 GC，不适合用来实现精确的“取消订阅”时机。需要显式取消协议时，应在业务 API 中
增加取消函数、context 或 token。

## 并发与阻塞

同一个 callback 可以被多个 Go goroutine 调用。每次调用都有独立 request ID 和 waiter，Dart 侧
分别执行并回传。

需要注意：

- 每次 callback 调用都会阻塞发起它的 Go goroutine，直到 Dart 返回；
- Dart 闭包在 isolate 事件循环中执行；
- async 闭包等待 Future 时，其他 Dart 任务仍可运行；
- runtime 没有为单次 callback 提供超时或取消；
- 如果 Dart 永远不完成返回的 Future，对应 Go goroutine 会一直等待。

需要超时时，应在 Dart 闭包内部对 Future 使用 timeout，或在 Go 业务协议中设计超时/取消机制。

## 初始化要求

回调依赖 `FlutterGoBridge.initialize(...)` 初始化 callback port。如果 bridge 没有通过生成的
initialize 入口打开，Go 调用 callback 时会得到“callback port is not initialized”错误；无 error
返回位时该错误会按前述规则变成 panic 并最终让外层 Dart 调用失败。

## Hot restart 与 isolate 生命周期

Dart 闭包属于创建它的 isolate。hot restart 会销毁旧 isolate 及其闭包 registry。不要依赖 Go 中
长期保存的旧 callback 跨 hot restart 继续有效；应用恢复后应重新调用注册 API，把新 isolate 的闭包
交给 Go。

对于需要长期订阅且经常 hot restart 的开发流程，建议提供显式 `Register/Clear` 或
`Subscribe/Unsubscribe` API，并在应用初始化时重新注册。

## 支持的类型

回调参数和单个返回值可以使用生成器支持的普通 bridge 类型，例如：

- bool、整数、浮点、string；
- byte/typed list、slice、array、map；
- 可翻译结构体和命名类型；
- 指针、`GoOpaque`、`DartOpaque`；
- 时间、URI、IP、BigInt、UUID 等专用映射；
- 已桥接的命名接口。

每个类型仍遵守自己的 codec 和可空规则。

## 当前不支持

回调只支持作为桥接函数或方法的**直接参数**。以下情况不支持：

```go
// 返回 callback
func Factory() func()

// callback 放在集合中
func RegisterAll(callbacks []func())

// callback 放在 map 中
func RegisterMap(callbacks map[string]func())

// callback 作为结构体字段
type Handler struct {
	Callback func()
}

// 嵌套 callback
func Nested(callback func(inner func()))

// 可变参数 callback
func Variadic(callback func(values ...int))
```

另外：

- 外层函数必须是 `//fgb:async`；
- callback 自身不能是泛型或 variadic；
- callback 最多一个非 error 返回值；
- callback 的 error 必须位于最后；
- 不支持 callback 中再传 callback；
- 定义的命名函数类型目前不等同于直接匿名 `func(...)` 参数，建议在桥接签名中直接写函数类型。

## 设计建议

- 回调可能失败时，让 Go callback 签名带末尾 `error`；
- 可选通知使用 `nullable`，Go 调用前判 nil；
- 高频单向事件优先考虑 Stream，避免每个事件都让 Go goroutine等待 Dart 回复；
- callback 适合“Go 请求 Dart 计算并需要返回值”的双向交互；
- 不要在持锁状态下调用可能长时间等待的 Dart callback；
- 长期保存 callback 时提供显式清理 API；
- 不要把旧 isolate 的 callback 当成 hot restart 后仍有效。

## 常见问题

| 问题 | 原因与处理 |
| --- | --- |
| 提示 callback 需要 `//fgb:async` | 同步 FFI 会阻塞 Dart 事件循环，给外层函数添加 async 指令 |
| Dart 同步闭包是否支持 | 支持，runtime 使用 `Future.sync` 统一处理 |
| Dart async 闭包是否支持 | 支持，Go goroutine 会等待 Future 完成 |
| Dart throw 后 Go 为什么 panic | callback 签名没有末尾 error；添加 error 结果即可在 Go 中处理 |
| Go 收到 nil callback | Dart 省略/传 null，且参数被标记 nullable；调用前先判 nil |
| Go goroutine 一直卡住 | Dart Future 未完成、isolate 已退出或回调未回复；callback 本身没有内置超时 |
| callback 一直不释放 | Go 仍持有函数值，清空全局/结构体中的引用并允许 GC |
| hot restart 后旧 callback 异常 | 旧闭包属于已销毁 isolate，重新注册 |
