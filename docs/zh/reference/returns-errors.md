# 返回值与 error

生成器先从 Go 结果列表中移除所有 `error`，再根据剩余结果数量生成 Dart 返回类型。`error` 不会作为
Dart 返回值或 record 字段出现；只要任意 error 非 nil，调用就以 `FgbPlatformException` 失败。

| Go 的非 error 结果数量 | Dart 返回类型 |
| --- | --- |
| 0 | `void` |
| 1 | 该值对应的 Dart 类型 |
| 2 个或更多，结果未命名 | 位置 record `(T1, T2, ...)` |
| 2 个或更多，结果已命名 | 命名 record `({T1 name1, T2 name2, ...})` |

`//fgb:async` 不改变结果形状，只在外层包装 `Future`。例如 `int` 变成 `Future<int>`，位置 record
变成 `Future<(T1, T2)>`。

## 没有返回值

```go
func Reset() {}
```

```dart
void reset()
```

只返回 `error` 的函数成功时也是 `void`：

```go
func Flush() error { return nil }
```

```dart
void flush()
```

如果 `Flush` 返回非 nil error，Dart 不会得到某个“错误返回值”，而是直接抛出异常。

## 一个普通返回值

一个非 error 结果始终保留普通类型，不会为了 Go 的结果列表生成单字段 record：

```go
func Add(a, b int) int { return a + b }

func Find(id int64) (User, error) { /* ... */ }
```

```dart
int add({required int a, required int b})
User find({required int id})
```

即使 Go 给单个结果命名，Dart 仍返回 `User`，而不是 `({User user})`。

## 多返回值与位置 record

两个或更多未命名非 error 结果生成位置 record：

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
```

调用方可以用 record pattern 或 `$1`、`$2` 访问：

```dart
final (quotient, remainder) = divide(value: 11, divisor: 4);
// 或：final result = divide(...); print(result.$1);
```

多个值在 wire 上作为一个列表传输，Dart 会校验列表长度并按原 Go 结果顺序还原 record。

## 命名结果与命名 record

当所有非 error 结果都有名称时，生成命名 record：

```go
func Split(value string) (head string, tail string, err error) {
	// ...
}
```

```dart
({String head, String tail}) split({required String value})
```

```dart
final result = split(value: 'a/b');
print(result.head);
print(result.tail);
```

只有非 error 结果名参与判断，`err` 是否命名不影响 record。名称会转为 lowerCamelCase；如果转换后
冲突，生成器会自动追加唯一后缀。record 字段仍按 Go 中非 error 结果的声明顺序编码和解码。

只要任意非 error 结果是未命名或 `_`，多结果就使用位置 record。

## error 可以出现在任意位置

生成器按类型识别每一个 `error`，不要求它位于最后：

```go
func Load(id int64) (error, User) {
	// ...
}

func Analyze(input string) (error, string, int, error) {
	// ...
}
```

对应 Dart 结果分别是 `User` 和 `(String, int)`。所有 error slot 都从 Dart 返回形状中移除，其余值
保持原相对顺序。

## 单个 error

函数只声明一个 error 结果时：

- error 为 nil：正常解码并返回普通结果；
- error 非 nil：抛出 `FgbPlatformException`；
- `code` 为 `go_error`；
- `message` 为 `err.Error()`；
- `goErrors` 为 null，直接读取 `message`。

```go
func Open(path string) (*File, error) {
	if path == "" {
		return nil, errors.New("path is empty")
	}
	// ...
}
```

```dart
try {
  final file = open(path: path);
} on FgbPlatformException catch (error) {
  if (error.code == 'go_error') {
    print(error.message); // path is empty
  }
}
```

## 多个 error

Go 允许一个函数声明多个 error 结果：

```go
func Validate(input string) (
	normalized string,
	syntaxErr error,
	policyErr error,
) {
	// ...
}
```

生成器不会只看第一个 error：

1. 按声明顺序检查所有 error slot；
2. 忽略 nil error；
3. 没有非 nil error 时调用成功；
4. 有任意非 nil error 时，收集所有消息并抛出一个 `FgbPlatformException`；
5. `message` 使用 `"; "` 拼接消息；
6. `goErrors` 保存逐条消息，顺序与非 nil error 的声明顺序一致。

即使多个 error 中只有一个非 nil，只要函数声明了多个 error，`goErrors` 仍会包含这一条消息。

```dart
try {
  final normalized = validate(input: text);
} on FgbPlatformException catch (error) {
  if (error.code != 'go_error') rethrow;

  print(error.message); // 汇总文本
  for (final message in error.goErrors?.messages ?? const <String>[]) {
    print(message); // 每一条 Go error
  }
}
```

## `FgbPlatformException`

所有由 bridge error envelope 返回的异常使用以下公共类型：

```dart
final class FgbPlatformException implements Exception {
  final String code;
  final String? message;
  final Object? details;
  final FgbGoErrors? goErrors;
}
```

| 字段 | 含义 |
| --- | --- |
| `code` | 稳定的错误类别，例如 `go_error`、`panic`、`invalid_argument` |
| `message` | 适合日志和展示的简要消息 |
| `details` | codec/transport 的附加诊断数据，形状可能随调用路径不同 |
| `goErrors` | 仅多个 error 结果的调用用于保存逐条 Go error 消息 |

`FgbGoErrors` 提供：

```dart
final List<String> messages;
int get length;
String operator [](int index);
```

`messages` 是不可修改列表，也可以用 `goErrors[index]` 读取。`FgbGoErrors.toString()` 会用分号连接
消息。

### 不要解析 `details` 获取多个 Go error

standard codec 会把多个 error 放入类似 `{method: ..., errors: [...]}` 的 map。CST/DCO 路径没有 map
类型，会直接传递消息列表。因此业务代码应读取已经归一化的 `error.goErrors`，不要依赖
`details['errors']` 的具体形状。

单个 Go error 的 `details` 通常只包含 method 诊断信息，`goErrors` 为 null。

## error 与普通结果同时存在

只要任意 error 非 nil，所有普通结果都会被丢弃，不存在“带错误的部分成功 record”：

```go
func Import() (created int, skipped int, err error)
```

成功时 Dart 得到 `({int created, int skipped})`；失败时只得到异常。若业务确实需要同时返回部分数据
和问题列表，应把它们建模为普通结果结构体，而不是 `error`：

```go
type ImportResult struct {
	Created  int
	Skipped  int
	Warnings []string
}

func Import() (ImportResult, error)
```

这样 fatal error 仍走异常，非致命 warning 则作为正常业务数据返回。

## sync 与 async 的异常时机

同步和异步 API 的异常类型相同，区别只在抛出时机：

```go
func ReadNow() (string, error) { /* ... */ }

//fgb:async
func ReadLater() (string, error) { /* ... */ }
```

```dart
try {
  final value = readNow(); // 调用过程中立即 throw
} on FgbPlatformException catch (error) {
  // ...
}

try {
  final value = await readLater(); // Future 以 error 完成，在 await 处 throw
} on FgbPlatformException catch (error) {
  // ...
}
```

不要遗漏 `await`，否则当前 `try/catch` 不会捕获 Future 后续完成时的异常。

## Go 业务 error 与 bridge 错误

`FgbPlatformException` 不只表示 Go 函数显式返回的业务 error。常见 code 包括：

| code | 来源 |
| --- | --- |
| `go_error` | Go 函数或方法返回了非 nil `error` |
| `panic` | Go 调用或 callback 路径发生 panic，dispatcher 已 recover |
| `invalid_argument` | Dart 参数、receiver 或 handle 无法按目标 Go 类型解码 |
| `encode_error` | Go 正常结果无法编码回 Dart |
| `invalid_request` | 底层 method call envelope 或 FFI 输入无效 |
| `method_not_found` | Go/Dart 生成物不匹配，调用了当前 bridge 不认识的方法或 call id |
| `bridge_error` | bridge 在构造错误响应等内部阶段再次失败 |
| `stream_error` | Go 主动向 Stream 发送错误，见 [Stream](/zh/reference/stream) |

处理业务校验错误时先判断 `code == 'go_error'`；其他 code 通常应记录完整异常并按集成故障排查。
`panic` 的 standard-codec `details` 可能带 Go stack，仅适合诊断，不应直接展示给最终用户。

## `FormatException` 与协议不一致

某些失败发生在 Dart 本地解码阶段，不是 Go error envelope，因此会抛 `FormatException`，例如：

- 多结果列表长度与生成签名不一致；
- envelope flag 或字段类型非法；
- 接口实现 tag 未知；
- Go 返回无法还原具体实现的 nil 命名接口；
- Go 动态库与 Dart 生成目录不是同一次 codegen 的产物。

这类问题通常不是业务错误。首先重新运行 codegen，并确保应用加载的动态库和 Dart 文件来自同一次
构建。

## Stream 与 callback 的特殊情况

拥有唯一 stream sink/channel 且没有普通返回值的 async Go 函数，会生成 `Stream<T>`，而不是
`Future<void>`。Stream 内的 `AddError`、关闭与生命周期规则见 [Stream](/zh/reference/stream)。

Dart 闭包 callback 自身的返回值和 error 会按 callback 协议送回 Go；如果 callback 签名没有末尾
error，Dart/transport 失败可能在合成 Go callback 中 panic，再由外层 async dispatcher 转为
`FgbPlatformException`。详见 [Dart 闭包回调](/zh/reference/callbacks)。

## 常见问题

| 现象 | 处理 |
| --- | --- |
| 期望 record，却只得到普通类型 | 只有一个非 error 结果；单结果不会包装 record |
| 命名结果生成了位置 record | 检查是否有未命名或 `_` 的非 error 结果 |
| 多个 error 只想看逐条消息 | 使用 `error.goErrors?.messages`，不要拆分汇总 `message` |
| `goErrors` 为 null | 函数只声明了一个 error；读取 `message` |
| 返回值存在但 Dart 没收到 | 任意非 nil error 会让整个调用失败并丢弃普通结果 |
| async 异常没有被 catch | 在 `try` 中 `await` Future |
| 收到 `method_not_found` 或 `FormatException` | Go 动态库与 Dart 生成物可能不同步，重新生成并一起部署 |
| 想返回 warning 和数据 | warning 放入普通结果结构体，只把 fatal failure 作为 `error` |
