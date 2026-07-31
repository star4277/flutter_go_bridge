# Stream

`flutter_go_bridge` 支持 Go 主动向 Dart 连续推送数据。只要异步 Go 函数的直接参数中出现
`chan<- T` 或 `fgb.StreamSink[T]`，生成器就会建立 Go → Dart Stream 通道。

这两种写法共享同一套底层 stream port，但生命周期和能力不同：

| 形式 | 适合场景 | 错误事件 | 函数返回后继续推送 | 手动关闭 |
| --- | --- | --- | --- | --- |
| `chan<- T` | 函数执行期间顺序产生数据 | 不支持 | 不支持 | 通常不需要，桥自动关闭 |
| `fgb.StreamSink[T]` | 需要完整生命周期控制或后台生产者 | `AddError` | 支持 | 使用 `Close` |

## 共同规则

无论使用哪种形式，都必须满足：

- 外层函数或方法标记 `//fgb:async`；
- Stream 只能从 Dart 调用 Go 时作为参数传入，不能作为 Go 返回值；
- `//fgb:nullable` 不能用于 stream 参数；
- 元素类型 `T` 必须能被 bridge 编解码；
- 不支持 Stream 中再包含 `StreamSink`；
- `context.Context` 参数不会出现在 Dart API 中，可用于响应订阅取消。

## 最简形式：`chan<- T`

### Go API

```go
//fgb:async
func Ticks(count int, out chan<- int) error {
	for i := 0; i < count; i++ {
		out <- i
	}
	return nil
}
```

当函数只有一个 stream 参数，并且没有非 `error` 返回值时，生成 Dart API 为：

```dart
Stream<int> ticks({required int count})
```

调用：

```dart
await for (final value in ticks(count: 5)) {
  print(value);
}
```

### channel 由谁创建和关闭

开发者不会从 Dart 传入 Go channel。生成 bridge 会：

1. 创建一个容量为 16 的内部 channel；
2. 把它作为 `chan<- T` 传给业务函数；
3. 启动 drain goroutine，把值编码并投递到 Dart；
4. 在业务函数返回时自动关闭 channel；
5. channel 被抽干后结束 Dart Stream。

业务函数通常只负责发送，不需要调用 `close(out)`。如果业务代码已经关闭 channel，桥的延迟关闭会
容忍重复关闭，不会让生成 dispatcher 崩溃。

### 为什么只能是 `chan<- T`

只写方向明确表达“业务代码生产，bridge 消费”。以下形式都会在生成期报错：

```go
func Bad1(out chan int)      {} // 双向 channel
func Bad2(out <-chan int)    {} // 只读 channel
```

正确写法：

```go
func Good(out chan<- int) {}
```

### 不要在函数返回后继续发送

channel 会在函数返回时自动关闭，因此不能把它交给一个生命周期超过函数的后台 goroutine：

```go
// 错误：函数返回后 bridge 会关闭 out，goroutine 再发送会 panic。
//fgb:async
func Start(out chan<- int) {
	go func() {
		out <- 1
	}()
}
```

如果生产者需要在外层函数返回后继续运行，改用 `fgb.StreamSink[T]`。

### 取消后的发送行为

Dart 取消订阅后，drain goroutine仍会继续读取 channel，但不再向 Dart 投递。这样即使业务代码没有
及时检查取消信号，`out <- value` 也不会因为 Dart 已离开而永久阻塞。

channel 的缓冲区容量为 16，生产速度短时间高于 drain 速度时，发送仍可能暂时阻塞；保证的是取消后
仍有人持续抽干，而不是所有发送都绝对无等待。

channel 没有返回错误的能力。如果元素编码失败，该项会被丢弃。需要感知关闭或编码错误时，使用
`StreamSink.Add`。

## 完整形式：`fgb.StreamSink[T]`

支持包由生成器写入当前 Go 模块的 `internal/fgb`：

```go
import "example.com/my_app/internal/fgb"
```

典型 API：

```go
//fgb:async
func Ticks(count int, sink fgb.StreamSink[int]) error {
	defer sink.Close()

	for i := 0; i < count; i++ {
		if err := sink.Add(i); err != nil {
			if errors.Is(err, fgb.ErrStreamClosed) {
				return nil
			}
			return err
		}
	}
	return nil
}
```

### `StreamSink` 方法

| 方法 | 行为 |
| --- | --- |
| `Add(value T) error` | 编码并投递一个数据事件；不会等待 Dart listener 消费 |
| `AddError(err error) error` | 向 Dart Stream 添加错误事件，Stream 保持打开 |
| `Close()` | 发送完成事件；幂等，可安全重复调用 |
| `IsClosed() bool` | 判断 sink 是否已关闭或 Dart 已停止监听 |

### `Add`

`Add` 会先编码值，再通过共享 stream port 投递。正常投递是非阻塞的。

可能返回：

- 元素编码错误；
- `fgb.ErrStreamClosed`：已经 `Close`，Dart 取消订阅、关闭 controller，或发生 hot restart。

收到 `ErrStreamClosed` 后应停止生产，它不是需要重试的瞬时错误。

### `AddError`

```go
if err := sink.AddError(errors.New("temporary failure")); err != nil {
	return err
}
```

Dart listener 收到一个 `FgbPlatformException`，其 code 为 `stream_error`。该事件不会自动关闭
Stream，之后仍可继续 `Add`；完成时仍需调用 `Close`。

传入 nil error 时，发送的消息为 `unknown error`。

### `Close`

`Close` 是幂等操作。常见写法是：

```go
defer sink.Close()
```

但如果 sink 被交给后台 goroutine，不能在外层函数返回时提前关闭，应由最后一个生产者负责：

```go
//fgb:async
func Start(sink fgb.StreamSink[Event]) error {
	go func() {
		defer sink.Close()
		for event := range eventSource {
			if err := sink.Add(event); err != nil {
				return
			}
		}
	}()
	return nil
}
```

### 拷贝与并发

`StreamSink[T]` 是可拷贝值，所有副本共享同一个底层 stream 状态，并可从多个 goroutine 调用。
任意副本关闭后，其他副本的 `Add` 都会返回 `ErrStreamClosed`。

Go 侧最后一份 sink 引用被 GC 回收时，runtime 会通知 Dart 完成 Stream，避免 listener 永久等待。
不过正常业务仍应显式 `Close`，不要依赖 GC 决定完成时机。

## Go-owned 与 Dart-owned

生成器根据函数签名决定 Dart API 的形态。

### Go-owned：直接返回 `Stream<T>`

同时满足以下条件：

1. 恰好一个直接 stream 参数；
2. 没有非 `error` 返回值。

`error` 返回值不影响该判定。

```go
//fgb:async
func Watch(name string, out chan<- Event) error
```

生成：

```dart
Stream<Event> watch({required String name})
```

stream 参数被隐藏，controller 由生成代码创建。

#### 冷流语义

返回的是单订阅冷 Stream：

- 调用 `watch(...)` 本身不会立即启动 Go；
- 第一次 listener 订阅时才执行 Go 函数；
- 创建 Stream 后从不监听，不会启动生产者，也不会无限缓存；
- 取消订阅会释放 sink 并关闭内部 controller。

Go 调用启动失败时，生成 runtime 会尝试把失败作为 Stream error 投递并关闭 controller。

### Dart-owned：调用者传入 `StreamSink<T>`

出现任一情况时，stream 参数保留在 Dart 签名中：

- 函数有非 `error` 返回值；
- 有多个直接 stream 参数；
- sink 位于结构体字段中。

```go
//fgb:async
func Subscribe(
	name string,
	sink fgb.StreamSink[string],
) (subscriptionID int64, err error)
```

生成：

```dart
Future<int> subscribe({
  required String name,
  required StreamSink<String> sink,
})
```

Dart 调用者创建 controller：

```dart
final controller = StreamController<String>();
final subscription = controller.stream.listen(
  print,
  onError: (Object error, StackTrace stack) {
    // 处理 AddError 或其他 stream 错误
  },
);

try {
  final id = await subscribe(
    name: 'news',
    sink: controller.sink,
  );
  print('subscription: $id');
} finally {
  await controller.close();
  await subscription.cancel();
}
```

Dart-owned 描述的是“controller 由调用者创建、sink 显式出现在 API 中”。Go 仍可以通过
`sink.Close()` 发出完成事件。调用者与 Go 生产者应约定由谁结束 Stream，避免生命周期含糊。

同一个 Dart sink 被传给多个参数时，runtime 会复用同一个 handle。

### 结构体字段

只有 `fgb.StreamSink[T]` 能作为字段；channel 字段不受支持：

```go
type Watcher struct {
	Name   string
	Events fgb.StreamSink[string]
}

//fgb:async
func Watch(watcher Watcher) error
```

Dart 侧字段类型是 `StreamSink<String>`，由调用者创建并放入对象。

## 使用 `context.Context` 响应取消

### 基本写法

```go
//fgb:async
func Watch(ctx context.Context, out chan<- string) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case value := <-updates:
			out <- value
		}
	}
}
```

`context.Context` 不出现在 Dart 签名。bridge 会创建：

```go
context.WithCancel(context.Background())
```

当 Dart 取消订阅或关闭对应 sink 时调用 cancel。函数返回时也会执行 cancel 清理资源。

一个桥接函数最多支持一个 `context.Context` 参数。它可以出现在 Go 参数列表任意位置，生成器会在
调用 Go 时按原位置插回。

多个 sink 存在时，调用级 context 与第一个 stream 参数的 handle 关联；关闭第一个 sink 会触发
context 取消。复杂的多流 API 如果需要独立取消，建议分别设计调用或在业务层传递自己的控制消息。

### 必须合作式取消

cancel 只关闭 `ctx.Done()`，不会强行终止 goroutine。业务代码应主动 select 或定期检查
`ctx.Err()`。如果忽略 context：

- channel drain 仍能避免发送永久阻塞；
- `StreamSink.Add` 会返回 `ErrStreamClosed`；
- 但业务 goroutine 自身仍可能继续占用资源。

## 错误与完成语义

| 来源 | Dart 表现 | 是否自动关闭 |
| --- | --- | --- |
| `sink.AddError(err)` | Stream 收到 code 为 `stream_error` 的 `FgbPlatformException` | 否 |
| Go-owned 调用返回 error/调用失败 | Stream 收到调用异常 | 是 |
| `sink.Close()` | Stream 完成 | 是 |
| channel 外层函数返回 | channel 被关闭并最终完成 Stream | 是 |
| Dart 取消订阅/关闭 sink | Go 后续 `Add` 返回 `ErrStreamClosed` | 是 |
| Go 最后一份 sink 被 GC | runtime 尝试完成 Dart Stream | 是 |

Dart-owned 调用的普通函数错误仍通过返回的 `Future` 抛出。该错误与 sink 的生命周期是两条独立
通道，调用者应在 `try/finally` 中关闭自己创建的 controller。

## 热重启安全

Flutter hot restart 会销毁 Dart isolate，但动态库和 Go goroutine 可能继续存在。新 isolate 的 stream
handle 又会从 1 开始，如果只按 handle 路由，旧生产者可能把事件送进新 Stream。

生成 runtime 为每个 isolate 维护 generation：

- 新 isolate attach 时 generation 增加；
- 上一 generation 的 context 全部取消；
- 旧 `StreamSink.Add` 返回 `ErrStreamClosed`；
- 旧 channel 的事件被丢弃；
- 旧 producer 不能命中新 isolate 中重复使用的 handle。

因此 Stream 事件不会跨 hot restart 泄漏，但业务 goroutine仍应响应 context 或关闭错误，才能及时退出。

## 类型与签名限制

- 必须是 `//fgb:async`；
- channel 只接受 `chan<- T`；
- `StreamSink` 只能作为参数或结构体字段，不能作为返回值；
- stream 参数不能标记 `nullable`；
- channel 不能作为结构体字段；
- 不支持 `StreamSink[StreamSink[T]]` 或任何嵌套包含 sink 的元素；
- 元素类型需要具备可用 codec；
- Go-owned Stream 是单订阅 Stream；
- 后台生产者应使用 `StreamSink`，不能保存自动关闭的 channel。

## 选择建议

使用 `chan<- T`，如果：

- 所有值都在函数返回前产生；
- 只需要数据和完成事件；
- 不需要从发送操作得到错误；
- 希望用最普通的 Go channel 写法。

使用 `fgb.StreamSink[T]`，如果：

- 需要 `AddError`；
- 需要函数返回后继续推送；
- 需要从多个 goroutine 推送；
- 需要检测 Dart listener 已离开；
- 需要把 sink 放进结构体；
- 需要显式控制完成时机。

## 常见问题

| 问题 | 原因与处理 |
| --- | --- |
| 提示需要 `//fgb:async` | Stream 依赖 Dart 事件循环，在外层函数上添加该指令 |
| `chan T` 或 `<-chan T` 生成失败 | 只支持 `chan<- T` |
| channel 后台 goroutine 出现 send on closed channel | channel 在外层函数返回时自动关闭，改用 `StreamSink` |
| `Add` 返回 `ErrStreamClosed` | listener 已取消、controller 已关闭、sink 已 Close 或发生 hot restart，应停止生产 |
| Dart 没收到数据 | 确认 Go-owned Stream 已被监听；冷流在订阅前不会启动 |
| Stream 一直不结束 | 确认调用 `Close`、外层 channel 函数已返回，或没有仍被持有的 sink |
| 取消后 goroutine 仍运行 | Go 代码没有检查 `ctx.Done()` 或 `Add` 错误 |

