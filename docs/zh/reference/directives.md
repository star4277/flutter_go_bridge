# 指令与字段 tag

`flutter_go_bridge` 提供两套控制生成结果的标记：

- **声明指令**：写在函数、方法、类型、常量或接口方法上，格式为 `//fgb:...`。
- **字段 tag**：写在结构体字段上，格式为 `fgb:"..."`。

两者的语法不同，不能混用。本页逐项说明每个指令和 tag 的适用位置、生成结果、限制与错误条件。

## 声明指令语法

指令必须严格采用 Go directive 写法，`//` 后面不能有空格：

```go
// 正确
//fgb:async

// 错误：生成器会明确报错
// fgb:async
```

多个指令既可以分行写，也可以用逗号组合：

```go
//fgb:async
//fgb:rename = "fetchUser"
func LoadUser() User { /* ... */ }

// 等价写法
//fgb:async, rename = "fetchUser"
func LoadUser() User { /* ... */ }
```

带多个参数名的 `nullable` 应把整个值放在引号中，避免参数名之间的逗号被当成指令分隔符：

```go
//fgb:async, nullable = "tags,onEvent"
func Watch(tags []string, onEvent func(string)) {}
```

指令名称区分大小写。空指令、未知指令、空重命名、空参数名，以及同时使用冲突的
`sync`/`async` 都会在生成阶段报错。

## 指令速查

| 指令 | 主要适用对象 | 生成效果 |
| --- | --- | --- |
| `//fgb:sync` | 函数、方法、接口方法 | 生成同步 Dart 调用；未标记时也是此模式 |
| `//fgb:async` | 函数、方法、接口方法 | 生成返回 `Future` 的 Dart 调用 |
| `//fgb:ignore` | 函数、方法、类型、常量、接口方法 | 从生成 API 中排除声明 |
| `//fgb:rename = "name"` | 函数、方法、类型、常量、接口方法 | 修改生成的 Dart 名称 |
| `//fgb:opaque` | 结构体类型 | 强制使用 `GoOpaque` 句柄语义 |
| `//fgb:enum` | 命名整数和字符串类型 | 从导出的 typed constant 生成闭集 Dart enum |
| `//fgb:nullable = "a,b"` | 函数、方法的参数 | 允许指定的 Go nil-able 参数在 Dart 中为 null 或省略 |

## `//fgb:sync`

### 用途

显式要求函数或方法生成同步 Dart API。未写任何调用模式指令时，默认就是 `sync`。

```go
func Add(a, b int) int { return a + b }

//fgb:sync
func Subtract(a, b int) int { return a - b }
```

生成结果：

```dart
final sum = add(a: 20, b: 22);
final difference = subtract(a: 22, b: 20);
```

### 行为

同步调用会阻塞当前 Dart isolate，直到 Go 返回结果。它适合计算量小、能立即完成、不需要反向调用
Dart 事件循环的操作。

### 限制

以下签名不能使用同步模式：

- 带 Dart 闭包回调参数的函数；
- 带 `chan<- T` 或 `fgb.StreamSink[T]` 的函数。

这些功能需要 Dart 事件循环处理回调或 Stream 消息，必须使用 `//fgb:async`，否则生成器会报错。

`//fgb:sync` 和 `//fgb:async` 同时出现时会报“调用模式冲突”。

## `//fgb:async`

### 用途

把生成的 Dart API 变成返回 `Future` 的异步调用：

```go
//fgb:async
func LoadUser(id int64) (User, error) {
	return queryUser(id)
}
```

```dart
final user = await loadUser(id: 42);
```

函数名不会自动增加 `Async` 后缀，也不会同时生成同步版本。

### 何时必须使用

- Go 函数接收 Dart 闭包，即 `func(...)` 参数；
- Go 函数通过 `chan<- T` 产生 Dart Stream；
- Go 函数使用 `fgb.StreamSink[T]`。

普通耗时任务也建议使用异步模式，避免阻塞 Dart UI isolate。

### 接口方法

接口方法可以标记为异步：

```go
type Loader interface {
	//fgb:async
	Load(id int64) (User, error)
}
```

实现该接口的已桥接方法会自动采用接口声明选定的调用模式，因此 `//fgb:async` 只需写在接口方法上，
不必在每个实现上重复。

## `//fgb:ignore`

### 函数或方法

被标记的函数或方法不会出现在 Dart API 中，也不会进入 dispatcher：

```go
//fgb:ignore
func InternalCleanup() {}
```

### 类型

被忽略的类型不会生成 Dart 类型，它的所有方法也会一起跳过：

```go
//fgb:ignore
type InternalState struct {
	Value int
}
```

如果其他已桥接函数仍在参数或返回值中引用该类型，生成器会报错。换句话说，`ignore` 是完全排除，
不是把类型自动转换成 `GoOpaque`；需要句柄语义时应使用 `//fgb:opaque`。

### 常量

可以跳过导出的命名类型常量：

```go
type Status int

const (
	StatusReady Status = iota
	//fgb:ignore
	StatusInternal
)
```

### 接口方法

标记后，该方法不会出现在生成的 Dart 接口声明中：

```go
type Store interface {
	Get(id int64) Item

	//fgb:ignore
	DebugDump() string
}
```

Go 的未导出标识符本来就不会参与生成；通常不需要再加 `ignore`。

## `//fgb:rename = "name"`

### 函数与方法

只修改生成的 Dart API 名称，不修改原始 Go 名称：

```go
//fgb:rename = "fetchUser"
func LoadUser(id int64) User { /* ... */ }
```

```dart
final user = fetchUser(id: 42);
```

### 类型

可以重命名生成的结构体、命名基础类型、接口或 opaque 句柄类：

```go
//fgb:rename = "Position"
type Point struct {
	X int
	Y int
}
```

所有引用该类型的生成签名都会使用 `Position`。

重命名为 Dart 无需 import 即可使用的名字（例如 `List`、`Duration`）时，生成器会加上 `Go` 前缀并
输出警告，否则生成的 library 无法编译。参见
[Dart 名称冲突](/zh/reference/type-mapping#dart-名称冲突)。

### 常量

可以修改生成的 Dart 常量名：

```go
type Status int

//fgb:rename = "statusReady"
const StatusReady Status = 1
```

建议把带 `rename` 的常量单独写成一个 const spec。若同一 spec 声明多个常量，指令会应用于其中
每个常量，容易产生重名。

### 接口方法

接口方法的 Dart 声明可以重命名：

```go
type Loader interface {
	//fgb:async, rename = "fetch"
	Load(id int64) (string, error)
}
```

实现方法会自动采用接口选定的 Dart 名称，因此重命名只需写在接口方法上。两个接口对同一个 Go 方法
要求不同名称时，生成器会直接报错。详见
[命名接口](/zh/reference/structs-interfaces#命名接口)。

### 命名要求

推荐使用合法的 Dart 标识符，并遵循：

- 类型：`UpperCamelCase`；
- 函数、方法、常量：`lowerCamelCase`。

生成器不会替你修复指令中无效的 Dart 语法。重命名造成 Dart 名称冲突时，应调整名称后重新生成。
它也可用于解决 Go 匿名嵌入方法在 Dart 继承中发生的 override 冲突。

## `//fgb:enum`

`//fgb:enum` 把命名字符串或有符号/无符号整数类型显式映射为 Dart enum。指令必须写在类型声明上，
不能写在函数或常量上：

```go
//fgb:enum
type Status int

const (
	StatusUnknown Status = iota
	StatusReady
)
```

类型必须至少有一个导出的同类型常量。底层值重复或底层类型不受支持时，生成会失败。该指令不能与
`//fgb:opaque` 组合使用。

成员名默认去掉 Go 类型名前缀。可以在常量上写 `//fgb:rename = "caseName"` 显式指定成员名。
与 Dart enum 内建成员冲突或彼此重名时，生成器会添加数字后缀并输出 warning。

enum 保存并传输每个常量精确的底层值。standard 和 CST/DCO 解码器收到声明集合之外的值时会抛出
`FormatException`。该标记不改变 Go 表示及其 codec；不写标记时始终保留原有的命名 extension-type
表示。

## `//fgb:opaque`

### 用途

强制结构体不按字段序列化，而是作为 Go 对象句柄跨越 FFI：

```go
//fgb:opaque
type Counter struct {
	total int
}

func NewCounter() *Counter {
	return &Counter{}
}

func (c *Counter) Increment() int {
	c.total++
	return c.total
}
```

Dart 侧生成一个继承 `GoOpaque` 的类型。对象状态留在 Go 堆中，Dart 只保存 handle；最后一个 Dart
引用被回收后，`NativeFinalizer` 会自动通知 Go 释放 handle，不需要手动 `dispose()`。

### 适用场景

- 结构体依赖私有字段保存状态；
- 方法需要持续修改同一个 Go 对象；
- 字段包含无法按值序列化的资源；
- 不希望把内部结构暴露为 Dart value class。

### 限制

opaque 类型必须以指针形式 `*T` 出现在桥接签名中：

```go
func NewCounter() *Counter          // 正确
func UseCounter(counter *Counter)   // 正确
func CopyCounter(counter Counter)   // 错误
```

`opaque` 与 `ignore` 语义相反，不应组合使用。前者生成句柄 API，后者完全排除类型。

即使不显式标记，含不可序列化字段的结构体也可能自动降级为 `GoOpaque` 并产生警告；显式标记可以
明确表达设计意图并消除该警告。

## `//fgb:nullable = "a,b"`

### 用途

Go 的 slice、map、函数值和接口不需要指针也能是 `nil`。默认生成 API 会把这些参数视为必填
非空值；`nullable` 用来保留 Go 的 nil 语义：

```go
//fgb:async, nullable = "tags,scores,onEvent"
func Store(
	id int64,
	tags []string,
	scores map[string]int,
	onEvent func(string),
) {}
```

生成的 Dart 参数类似：

```dart
Future<void> store({
  required int id,
  List<String>? tags,
  Map<String, int>? scores,
  FutureOr<void> Function(String)? onEvent,
})
```

调用者可以省略这些参数或传 `null`，Go 侧收到真正的 `nil`。

### 支持的参数类型

| Go 参数 | Dart 参数 |
| --- | --- |
| `func(...)` | 可空的 `FutureOr<R> Function(...) ?` |
| `[]T` | `List<T>?` |
| `map[K]V` | `Map<K, V>?` |
| `[]byte` | `Uint8List?` |
| `[]int32` | `Int32List?` |
| `[]int64` | `Int64List?` |
| `[]float64` | `Float64List?` |
| 命名接口 | `InterfaceType?` |

### nil 与空集合

`nil` 和空集合会保持为不同值：

```dart
await store(tags: null);       // Go: tags == nil
await store(tags: const []);   // Go: tags != nil && len(tags) == 0
```

### 不支持的用法

- 标量、字符串、数组和按值结构体不能标记；需要可空时改成 Go 指针；
- 固定数组 `[N]T` 永远不能为 nil；
- 指针本身已经会生成可空且非 required 的 Dart 参数，不需要 `nullable`；
- 参数名必须与 Go 源码中的名称完全一致；
- 不存在的参数名会报错；
- 回调参数仍然要求外层函数使用 `//fgb:async`；
- `nullable` 只控制参数，不允许 Go 返回一个不可还原的 nil 接口值。

```go
func Find(id int64, fallback *User) // fallback 自动生成 User?，无需 nullable

// 错误：int 不能为 nil
//fgb:nullable = "count"
func SetCount(count int) {}
```

## 字段 tag 语法

字段选项放在同一个 `fgb` tag 中，用逗号组合：

```go
type Item struct {
	Name  string `fgb:"rename:title"`
	Count int    `fgb:"non-final,defaultValue: 0"`
}
```

支持的选项区分大小写：

| tag | 生成效果 |
| --- | --- |
| `ignore` 或 `-` | 字段不进入 Dart class 和 wire codec |
| `rename:name` | 修改 Dart 字段名、构造参数名和 wire key |
| `non-final` | 去掉 Dart 字段的 `final` |
| `nullable` | 保留 slice/map/typed-list/接口字段的 nil |
| `defaultValue: expr` | 为 Dart 构造参数设置默认值 |

未知选项、空 `rename:`、空 `defaultValue:` 会在生成阶段报错。

## `fgb:"ignore"` 与 `fgb:"-"`

### 用途

字段完全不参与 Dart 类型和双向 wire 编解码：

```go
type User struct {
	ID       int64
	Password string `fgb:"ignore"`
	Cache    string `fgb:"-"`
}
```

Dart 侧只会生成 `id`。

以下字段也会自动跳过：

- 小写开头的未导出字段；
- 名为 `_` 的字段；
- `json:"-"`；
- `flutter_go_bridge:"-"`。

如果一个原本非空的结构体所有字段都被排除，生成器会把它降级为 `GoOpaque`，避免生成一个丢失
全部状态的空 value class。真正声明为 `struct{}` 的空结构体仍会生成无字段 value class。

被忽略字段中的值不会跨 FFI 传输。不要用它隐藏仍需在 Dart 与 Go 之间保持的业务状态。

## `fgb:"rename:name"`

### 用途

同时修改三个名称：

1. Dart 字段名；
2. Dart 构造函数命名参数；
3. wire codec 使用的字段 key。

```go
type Item struct {
	Name string `fgb:"rename:title"`
}
```

```dart
final class Item {
  final String title;

  const Item({required this.title});
}
```

字段名解析优先级为：

1. `fgb:"rename:name"`；
2. `flutter_go_bridge:"name"` 的第一个片段；
3. `json:"name"` 的第一个片段；
4. Go 字段名自动转为 lower camel case。

例如：

```go
type Profile struct {
	DisplayName string `json:"display_name,omitempty"`
}
```

没有 `fgb:rename` 时，wire key 使用 `display_name`，Dart 字段名会转换为 `displayName`。

`fgb:rename` 的值应是合法且唯一的 Dart 字段名。两个字段得到相同 wire key 时生成器会直接报错。

## `fgb:"non-final"`

### 用途

默认生成的 value class 字段是不可变的：

```dart
final int count;
```

标记后改为可写字段：

```go
type CounterView struct {
	Count int `fgb:"non-final"`
}
```

```dart
int count;
```

只要当前类或继承链中存在一个 non-final 字段，生成构造函数就不会使用 `const`。

### 语义说明

这只改变 Dart 对象的可变性，不会把修改实时同步到某个 Go 对象。value struct 每次调用都按字段
重新序列化；只有把修改后的 Dart 对象再次传给 Go，Go 才能看到新值。需要持久的 Go 端对象状态时，
应使用 `//fgb:opaque`，而不是 `non-final`。

## `fgb:"nullable"`

### 用途

让不用指针也能为 nil 的字段生成可空 Dart 类型，并在两个方向保留 nil：

```go
type Record struct {
	Tags   []string       `fgb:"nullable"`
	Scores map[string]int `fgb:"nullable"`
	Shape  Shape          `fgb:"nullable"`
	Plain  []string
}
```

生成结果：

```dart
final List<String>? tags;
final Map<String, int>? scores;
final Shape? shape;
final List<String> plain;
```

带 `nullable` 的构造参数不加 `required`。Go → Dart 时，nil 保持为 null；未标记的普通集合字段
会走非空编码路径，通常归一为对应的非空集合表示。

### 支持与限制

字段 tag 实际适用于：

- slice；
- map；
- byte/typed list；
- 命名接口。

函数类型虽然在 Go 中可以为 nil，但回调目前只能作为函数的直接参数，不能作为结构体字段。
标量、字符串、数组和值结构体不能标记。指针字段已经可空，再加 `fgb:"nullable"` 会作为冗余配置报错。

## `fgb:"defaultValue: expr"`

### 用途

为生成的 Dart 构造参数提供默认值，使字段不再是 `required`：

```go
type Options struct {
	Retry int      `fgb:"defaultValue: 3"`
	Tags  []string `fgb:"defaultValue: const []"`
}
```

```dart
const Options({
  this.retry = 3,
  this.tags = const [],
});
```

`expr` 会原样写入 Dart 代码，生成器不会替你转换 Go 表达式。它必须是类型兼容的 Dart
编译期常量，例如：

- `0`、`false`、`''`；
- `null`；
- `const []`、`const {}`；
- 合法的 Dart enum/const 值。

### 必须放在最后

`defaultValue:` 会读取 tag 中剩余的所有文本，以便表达式自身包含逗号：

```go
Values []int `fgb:"non-final,defaultValue: const [1, 2]"`
```

因此它必须是最后一个选项。下面的写法不会把 `non-final` 识别为另一个 tag，而会把它当成 Dart
表达式的一部分：

```go
// 错误
Values []int `fgb:"defaultValue: const [1, 2],non-final"`
```

默认值只影响 Dart 构造对象时的省略行为，不会修改 Go 类型本身的零值，也不会让 Go 端自动填充默认值。

## 组合示例

```go
//fgb:rename = "CatalogItem"
type Item struct {
	ID int64

	// Dart: final String title
	// wire key: title
	Name string `fgb:"rename:title"`

	// Dart: int count，构造时默认 0，整个构造函数不再是 const
	Count int `fgb:"non-final,defaultValue: 0"`

	// Dart: List<String>? tags，可省略，并保留 Go nil
	Tags []string `fgb:"nullable"`

	// 完全不生成
	InternalNote string `fgb:"ignore"`
}

//fgb:async, rename = "loadCatalogItem", nullable = "filters,onProgress"
func LoadItem(
	id int64,
	filters map[string]string,
	onProgress func(int),
) (Item, error) {
	// ...
}
```

## 常见错误

| 问题 | 原因与修复 |
| --- | --- |
| 写成 `// fgb:async` | `//` 后不能有空格，改为 `//fgb:async` |
| 同时写 `sync` 和 `async` | 两种调用模式冲突，只保留一个 |
| `nullable` 提示 unknown parameter | 使用了 Dart 名或拼错名称；这里必须写 Go 参数名 |
| 给 `int`、`string`、数组加 nullable | 它们不能直接为 nil；改成 Go 指针 |
| 给指针字段加 nullable | 指针已经自动映射为可空类型，删除该 tag |
| 回调或 Stream 提示需要 async | 给外层函数添加 `//fgb:async` |
| `defaultValue` 后的 tag 不生效 | `defaultValue` 必须放在最后 |
| Dart 编译报告默认值错误 | 默认值是原始 Dart 表达式，需使用类型兼容的编译期常量 |
| 重命名后出现冲突 | 为声明或字段选择唯一、合法的 Dart 名称 |
