# 结构体与接口

生成器不会把所有 Go 结构体都按同一种方式处理。一个结构体会根据字段是否可传输，生成 Dart
值对象，或退化为只保留 Go 对象身份的 `GoOpaque` 句柄。命名非空接口则生成 Dart 接口，并由已生成
的具体类型实现。

| Go 声明 | Dart 结果 | 主要语义 |
| --- | --- | --- |
| 字段都可传输的结构体 | `final class` 或 `class` | 按字段复制、序列化和重建 |
| `//fgb:opaque` 结构体 | `final class ... extends GoOpaque` | Go 端持有状态，Dart 只持有 handle |
| 含不可传输字段的结构体 | 自动退化为 `GoOpaque`，并产生 warning | 避免错误复制无法编码的状态 |
| 命名非空接口 | `abstract interface class` | 只能传递生成器已发现的 Go 实现类型 |
| `any` / `interface{}` | `Object?` | 动态值，不生成 Dart 接口声明 |

## 值结构体

当所有参与桥接的字段都能映射到 Dart 时，结构体会生成 value class：

```go
type User struct {
	ID       int64
	Name     string
	Nickname *string
	Tags     []string `fgb:"nullable"`
}

func SaveUser(user User) User { return user }
```

大致生成：

```dart
final class User {
  const User({
    required this.id,
    required this.name,
    this.nickname,
    this.tags,
  });

  final int id;
  final String name;
  final String? nickname;
  final List<String>? tags;
}

User saveUser({required User user}) { /* bridge call */ }
```

这里的“值对象”非常重要：传入 Go 时会把 Dart 字段编码成一个新的 Go 值；从 Go 返回时又会根据
返回字段创建一个新的 Dart 对象。两端没有共享对象地址，也没有自动的双向状态同步。

### 相等性、hashCode 与 `toString()`

所有生成的值类型都会重写 `operator ==` 和 `hashCode`。比较范围包含直接字段和匿名嵌入后提升的
字段；List、typed data 和 Map 都按内容进行深比较与深哈希，而不是使用 Dart 默认的对象身份。
因此，同一个 Go 值被分别解码得到的两个 Dart 对象，可以正常用于 Set 和 Map key。

结构体从 Go 发往 Dart 时，Go bridge 会对实际 Go 值调用 `encoding/json.Marshal`。序列化成功后，
生成的 JSON 字符串会随结构体一起传到 Dart，并由 `toString()` 直接返回。Dart 不会根据映射字段
重新拼装 JSON，因此 Go 的 JSON tag、`MarshalJSON`、由自定义 marshaler 暴露的私有状态、字节
编码等行为都以 Go `encoding/json` 为准。若序列化失败，结构体本身仍会正常传输，`toString()`
则退回 Dart 默认的对象文本。

只在 Dart 本地构造、尚未经过 Go 返回的对象没有 Go 生成的 JSON 快照，因此它的 `toString()`
同样使用默认文本；当该值传入 Go 并由 Go 返回后，返回的新对象会携带 JSON。

### 哪些字段参与桥接

默认只桥接导出字段。以下字段会被跳过：

- 未导出字段和名为 `_` 的字段；
- `fgb:"ignore"` 或 `fgb:"-"`；
- `flutter_go_bridge:"-"`；
- `json:"-"`。

字段名和 wire key 的优先级是 `fgb:"rename:name"`、`flutter_go_bridge` tag、`json` tag，最后才是
Go 字段名转 lowerCamelCase。两个字段最终得到相同 wire key 时，生成器会报错而不是覆盖数据。

字段 tag 的完整语法、可空类型和默认值规则见[指令与字段 tag](/zh/reference/directives)。

### 构造器、final 与可空字段

生成规则如下：

- 普通字段生成 `final` 字段和 `required` 命名构造参数；
- Go 指针字段生成可空 Dart 字段，构造参数可以省略；
- `fgb:"nullable"` 支持的字段也可空且可以省略；
- `fgb:"defaultValue: ..."` 为参数增加 Dart 默认值，因此不再 `required`；
- 默认构造器是 `const`；只要存在 `fgb:"non-final"` 字段，构造器就不再是 `const`；
- 不参与继承时类默认是 `final class`，父类和子类则必须是普通 `class`。

`defaultValue` 是原始 Dart 编译期常量，只影响 Dart 构造对象的行为，不会修改 Go 零值。

### 指针字段与指针 receiver 不是一回事

字段类型为 `*T` 表示这个字段本身可以是 nil，因此映射为 `T?`。方法的 receiver 为 `*T` 则表示
Go 方法接收指针：

```go
type CounterView struct {
	Count int
}

func (c *CounterView) Add(delta int) int {
	c.Count += delta
	return c.Count
}
```

值结构体仍会生成 Dart 实例方法，但调用时 bridge 只是根据当前 Dart 字段临时重建一个 Go receiver。
上例对 `c.Count` 的修改不会写回原来的 Dart 对象：

```dart
final view = CounterView(count: 1);
final nextCount = view.add(delta: 2);
print(view.count); // 仍然是 1
```

需要更新值对象时，让 Go 返回更新后的结构体；需要跨多次调用保持同一个 Go 对象及其可变状态时，
应改用 `GoOpaque`。

### 循环引用

通过指针形成的递归类型可以生成，因为指针会变成可空引用：

```go
type Node struct {
	Value int
	Next  *Node
}
```

运行时值不能真正形成环；编码器会在 64 层后返回 codec 错误。嵌套指针（例如 `**Node`）不是支持
的普通字段形状，会使结构体退化为 opaque。

## `GoOpaque` 结构体

opaque 类型不复制字段。Go bridge 把实际 Go 指针存入 handle registry，Dart 对象只携带 bridge 和
整数 handle：

```go
//fgb:opaque
type Counter struct {
	total int
}

func NewCounter() *Counter { return &Counter{} }

func (c *Counter) Add(delta int) int {
	c.total += delta
	return c.total
}
```

大致生成：

```dart
final class Counter extends GoOpaque {
  Counter.fgbInternal({
    required super.fgbBridge,
    required super.fgbHandle,
  });

  int add({required int delta}) { /* call using fgbHandle */ }
}
```

每次调用都通过同一个 handle 找到同一个 Go 对象，因此 `total` 可以持续变化。Dart 引用被垃圾回收
后，`NativeFinalizer` 会通知 Go 释放对应 handle；生成 API 不要求手动调用 `dispose()`。不要依赖
finalizer 的即时执行来管理文件、socket 等必须确定性关闭的业务资源，仍应在 Go API 中提供显式
`Close` 方法。

生成的 opaque 类从 `GoOpaque` 继承相等与哈希语义：只有生成类型、bridge 实例和 Go handle 都
相同才视为相等。Go 返回 opaque 值时，也会把 Go 端生成的 JSON 字符串与 handle 一起发送。这使
“字段无法桥接而自动退化为 `GoOpaque`，但实现了 `MarshalJSON`”的类型仍能得到有意义的
`toString()`。该字符串是 handle 发往 Dart 时的快照；后续方法若修改 Go 状态，不会自动刷新，
除非 Go 再次返回这个对象。

### 何时自动退化为 opaque

以下情况会阻止按值编码，并使结构体自动退化为 `GoOpaque`：

- 函数字段或普通 channel 字段；
- 不支持的字段类型或嵌套指针；
- 按值包含另一个 opaque 结构体；
- 声明了字段，但所有字段都未导出或被 tag 排除。

依赖包的命名非空接口通过已发现实现和接口级 opaque fallback 按 tagged union 传输，不会再单独导致
外层结构体 opaque 退化。

自动退化会给出 warning，并指出阻塞字段。若这本来就是你的设计，添加 `//fgb:opaque` 可以明确
意图并消除 warning。

opaque 类型必须以 `*T` 出现在导出的参数、返回值或 receiver 中，不能按 `T` 传递：按值复制会
丢失它的对象身份，生成器因此会直接拒绝。

### 空结构体与“没有可桥接字段”

真正的空结构体仍是值类型：

```go
type Marker struct{}
```

```dart
final class Marker {
  const Marker();
}
```

但 `struct{ private int }` 或字段全部标记为 ignore 的非空结构体会退化为 opaque，因为它们通常包含
Dart 无法重建的 Go 状态。这两种情况不能混为一谈。

## 外部依赖类型

导出 API 直接或间接引用的第三方命名类型也会被分析。可传输的外部结构体会在 `_generated.dart`
中生成值类；只有私有状态或含不支持字段的外部结构体会生成 opaque handle。

不同包的类型若产生相同 Dart 名称，生成器会添加 Go 包名前缀消除冲突。例如两个 `User` 可能生成
为 `ModelsUser` 和 `AuthUser`。因此应从生成文件导入类型，不要假定外部类型一定保留短名称。

可达的命名非空接口也会作为 tagged serialization union 分析。生成器会扫描输入 API 已加载的完整依赖图，
寻找实现该接口的导出命名结构体，并追加一个接口级 `GoOpaque` 成员来保留生成 Go 无法命名的实现。
依赖接口的方法不会生成 Dart 调用：Dart 接口只作为 marker，具体类负责携带可序列化字段或
`GoOpaque` handle。

## 匿名嵌入与 Dart 继承

匿名嵌入一个普通值结构体会映射为 Dart `extends`：

```go
type Animal struct {
	Name string
	Legs int
}

func (a Animal) Label() string { return a.Name }

type Dog struct {
	Animal
	Breed string
}
```

大致生成：

```dart
class Animal {
  const Animal({required this.name, required this.legs});

  final String name;
  final int legs;

  String label() { /* bridge call */ }
}

class Dog extends Animal {
  const Dog({
    required super.name,
    required super.legs,
    required this.breed,
  });

  final String breed;
}
```

被提升的字段在 wire 上会扁平化，编码结果包含 `name`、`legs`、`breed`，不会再嵌套一层 `animal`。
嵌入类型的方法由 Dart 继承；子类型声明兼容的同名方法时会生成 `@override`。

### 嵌入限制

- Dart 只有单继承，所以一个结构体最多只能嵌入一个可作为父类的值结构体；
- 不能嵌入 `*T` 作为父类，因为 Dart 不能继承可空类型；
- 不能继承 opaque 结构体；
- 专用映射类型或非结构体命名类型不会自动变成父类；
- 提升后字段的 wire key 冲突会导致生成失败；
- Go 允许子类型用任意签名遮蔽提升方法，Dart override 必须兼容。签名不兼容时请用
  `//fgb:rename` 改名。

接口编码时，生成器会先检查继承层级中更具体的子类，再检查父类，避免 `Dog` 被错误标记成
`Animal`。

## 命名接口

命名非空接口会生成 `abstract interface class`。输入 Go 包中的接口还会生成可桥接的方法声明：

```go
type Shape interface {
	Area() int
	Label() string
}

type Circle struct {
	Radius int
}

func (c Circle) Area() int     { return c.Radius * c.Radius }
func (c Circle) Label() string { return "circle" }

func Describe(shape Shape) string { return shape.Label() }
```

```dart
abstract interface class Shape {
  int area();
  String label();
}

final class Circle implements Shape {
  const Circle({required this.radius});

  final int radius;

  @override
  int area() { /* concrete Go call */ }

  @override
  String label() { /* concrete Go call */ }
}

String describe({required Shape shape}) { /* bridge call */ }
```

接口中的方法只有 Dart 声明，不存在一个“调用接口自身”的 FFI 方法。实际调用始终落到生成的具体类
方法，由具体值携带字段或 opaque handle。

### 实现类型如何发现

对于输入包接口，生成器检查输入包中参与桥接的结构体，并按声明顺序保持原有 wire 兼容性。

对于依赖接口，生成器遍历 `go/packages` 已加载的完整依赖图，收集 `T` 或 `*T` method set 满足接口的
导出、非泛型命名结构体。以下声明不会进入实现集合：名为 `main` 的包、受 Go `internal` 导入规则
限制而无法由生成 bridge 引用的包、未导出类型、类型别名、非结构体命名类型，以及未实例化的泛型声明。
这些类型不能作为具名 union 成员，但仍会由最后的接口级 opaque fallback 传输：Go handle registry
保存接口值本身，运行时注册的新实现也使用同一 fallback。

- value 实现按字段传输；
- 只有 `*T` 实现接口时，Go 解码会重建指针，Dart 仍看到对应 value class；
- `T` 和 `*T` 都实现接口时，Go 返回两种动态类型都能映射到同一个 Dart 类；
- opaque 实现按 `*T` handle 传输；
- 第三方结构体匿名嵌入指针时会自动退化为 `GoOpaque`，因为 Dart 不能继承可空父类，且无法给依赖类型添加指令；
- 输入包接口至少必须有一个已桥接实现，否则生成失败并提示 `no bridged type implements interface ...`；
- 依赖接口始终带有 opaque fallback，即使没有任何可命名的具体实现也能保留并回传 Go 对象。

输入包接口不能包含未导出方法。依赖接口在 Dart 中只作为 marker，不生成其方法；第三方声明没有
FGB 调用指令，也没有对应的生成调用入口，因此 marker 语义只负责安全序列化具体实现。

### 接口值的 wire 格式

接口值使用 standard codec 的 tagged union 传输，逻辑形状是：

```text
[implementorIndex, payload]
```

`implementorIndex` 标识具体实现，`payload` 是该 value struct 的字段数据或 opaque handle。Dart 传入
不属于已生成实现集合的对象时会抛出 `ArgumentError`；但 Go 先前返回的接口 fallback 对象可以原样
传回。收到未知 tag 会报格式错误。

输入包实现按 Go 声明位置保持稳定；依赖实现按完整包路径和 Go 类型名排序。若同一 value 类型的
`T` 与 `*T` 都能作为 Go 动态实现，Dart → Go 使用值形态作为规范 tag。tag 数字仍属于生成协议的
内部细节。Go bridge 与 Dart 目录必须由同一次生成结果配套发布，不要手写或持久化这些 tag。

接口 tagged union 走 standard codec，不走 CST 快速路径。

### 接口的 null

接口参数默认非空且 `required`。若 Go 需要接收 nil 接口，可在函数或方法上标记参数：

```go
//fgb:nullable = "shape"
func DescribeOptional(shape Shape) string {
	if shape == nil {
		return "none"
	}
	return shape.Label()
}
```

```dart
String describeOptional({Shape? shape})
```

Go 返回 nil 命名接口时，Dart 无法知道它本应对应哪个具体实现，因此解码会抛出 `FormatException`。
需要表达“可能没有值”的返回结果时，优先返回显式结果结构体、布尔标志，或其他可无歧义映射的类型。

### 接口方法指令

输入包接口方法支持 `//fgb:sync`、`//fgb:async`、`//fgb:rename` 和 `//fgb:ignore`。实现方法的生成
形状必须与接口声明兼容，例如接口方法标为 async 时，对应实现方法也应生成同样的 `Future` 签名。
依赖接口的方法只作 marker，不读取这些指令。完整语法见[指令与字段 tag](/zh/reference/directives)。

## 如何选择

| 需求 | 推荐建模 |
| --- | --- |
| DTO、配置、一次调用的输入输出 | 值结构体 |
| 需要 Dart 修改字段后再次传回 Go | 值结构体，可按需使用 `non-final` |
| Go 对象需要跨调用保持身份和内部状态 | `//fgb:opaque` + `*T` |
| 文件、连接、缓存、带锁对象 | opaque，并提供显式 `Close` |
| 一组有限、已知的 Go 实现需要多态传输 | 输入包中的命名接口，或具有可发现实现的可达依赖接口 |
| 无固定结构的动态数据 | `any` / `Object?` |

## 常见生成错误

| 现象 | 原因与处理 |
| --- | --- |
| `must be passed as *T` | opaque 类型被按值使用，改为 `*T` |
| 自动生成了 `GoOpaque` | 检查 warning 中的不可传输字段；修正字段或显式标记 opaque |
| `only extend one type` | 匿名嵌入了多个值结构体，改用组合或只保留一个父类 |
| 嵌入 pointer/opaque 报错 | Dart 无法把它们作为父类，改成普通字段或重新建模 |
| duplicate wire field | 字段重命名或提升后发生 key 冲突，选择唯一名称 |
| override signature 不兼容 | Go 遮蔽方法不满足 Dart override，使用 `//fgb:rename` |
| 接口没有 bridged implementor | 只会发生在输入包接口；声明并暴露至少一个实现。依赖接口会使用 opaque fallback |
| Dart 传入自定义 `implements Shape` 对象失败 | 只有生成器登记的 Go 实现能跨 bridge，不能传任意 Dart 实现 |
| Go 返回 nil 接口后 `FormatException` | 返回值缺少具体实现 tag，改用显式可选结果模型 |
