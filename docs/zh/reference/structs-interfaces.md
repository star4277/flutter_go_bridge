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

### 相等性与 hashCode

值结构体按字段跨界，所以从同一份 wire 字节解出来的两个实例代表同一个 Go 值。Dart 默认按引用比较
会把它们判为不同，因此**至少有一个桥接字段**的值类会生成 `operator ==` 和 `hashCode`：

```go
type Point struct {
	X int
	Y int
}
```

```dart
final class Point {
  final int x;
  final int y;

  const Point({required this.x, required this.y});

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is Point &&
          other.runtimeType == runtimeType &&
          fgbInternalDeepEquals(x, other.x) &&
          fgbInternalDeepEquals(y, other.y);

  @override
  int get hashCode {
    var result = runtimeType.hashCode;
    result = result * 31 + fgbInternalDeepHash(x);
    result = result * 31 + fgbInternalDeepHash(y);
    return result;
  }
}
```

`Set` 去重、`Map` 的 key、`List.contains`、以及 Flutter 判断是否重建，全都依赖这两个成员，
所以跨界回来的值和 Dart 本地构造的值行为一致。

几个需要知道的细节：

- **集合字段按内容比较。** Dart 的 `List` 和 `Map` 默认按引用比较，与 Go 的值语义相反，
  所以每个字段都经过 `fgbInternalDeepEquals`。map 的比较和哈希都不受插入顺序影响。
- **`hashCode` 以 31 累积。** 每个字段的哈希按 `result * 31 + fieldHash` 折入，
  初值取 `runtimeType.hashCode`。31 是经验上在性能和散列分布之间较均衡的奇质数。
- **继承链会被纳入，并且父子不会互等。** 匿名嵌入的结构体成为 Dart 父类，子类同时比较被提升的
  字段；`runtimeType` 检查保证字段值相同的父类实例不会等于子类实例。
- **没有可桥接字段的结构体保持引用相等。** 没有东西可比时，任意两个实例都会相等，
  这比 `Object` 原本的语义更没用。
- **`GoOpaque` 句柄保持引用相等。** 它的身份就是句柄，而同一个 Go 对象可能以多个句柄跨界，
  生成 `==` 会给出兑现不了的承诺。需要按值比较句柄时，请在 Go 侧提供方法。

`==` 是唯一由字段生成的运算符。其余 Dart 运算符都来自签名匹配的 Go 方法，见
[运算符重载](/zh/reference/operators)。

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

可达的命名非空接口也会作为 tagged serialization union 分析。生成器只从公共 API 已经可达的类型中
寻找实现该接口的导出命名结构体；无关 import 不会增加 union 成员或移动已有 tag，并追加一个接口级
`GoOpaque` 成员来保留生成 Go 无法命名的实现。依赖接口的方法不会生成 Dart 调用：Dart 接口只作为
marker，具体类负责携带可序列化字段或 `GoOpaque` handle；生成器会对这一能力边界输出 warning。

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

对于依赖接口，生成器只遍历公共 API 已经可达的类型，收集 `T` 或 `*T` method set 满足接口的导出、
非泛型命名结构体；无关 import 不会增加 union 成员，也不会移动已有 tag。以下声明不会进入实现集合：
名为 `main` 的包、受 Go `internal` 导入规则
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
[implementorTag, payload]
```

`implementorTag` 标识具体实现，`payload` 是该 value struct 的字段数据或 opaque handle。Dart 传入
不属于已生成实现集合的对象时会抛出 `ArgumentError`；但 Go 先前返回的接口 fallback 对象可以原样
传回。收到未知 tag 会报格式错误。

输入包实现按 Go 声明位置保持稳定并继续使用数字 tag；依赖实现按完整包路径和 Go 类型名排序，
但 tag 是由相同内容生成的稳定字符串。若同一 value 类型的 `T` 与 `*T` 都能作为 Go 动态实现，
Dart → Go 使用值形态作为规范 tag。生成版本不一致时会报告未知 tag，而不是静默解码成错误类型。

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

`//fgb:nullable` 让参数变成 `Shape?`，nil 在两个方向上都保持为真正的 Dart `null`。

当 nil Go 接口出现在**非空**位置（必填字段、参数或返回值）时，不会报错。Go 编码器发送 `null`，Dart
解码器会生成一个缺省实现——`final class _ShapeAbsent implements Shape, GoAbsent`，其方法覆盖抛出
`StateError`，`toString` 返回 `'Shape(absent)'`。

这一机制存在是因为 Go 接口的零值就是 `nil`——它是唯一一种无需指针就天然可为 nil 的 Go 类型，但 Dart 侧
却将该字段声明为非空。缺省实例让 Go 的零值结构体可以正常跨 bridge，与其他 nil-able Go 类型的处理方式
保持一致：`*T` 将 nil 映射为 `null`，slice 和 map 将 nil 归一化为空集合，接口将 nil 归一化为一个缺省
对象。Dart 类型保持非空，这样已有代码（期望 `Shape` 而非 `Shape?`）可以继续编译。

**缺省实例是兜底机制，不是推荐用法。** 它把编码边界处的响亮失败换成了使用点的安静失败——该值在 Dart
分析器看来是一个正常的 `Shape`，但背后没有真实实现。如果字段或参数确实可能为 nil，请标记
`fgb:"nullable"`（字段）或 `//fgb:nullable`（参数），让 Dart 类型变成 `Shape?`，由类型系统显式强制 nil 检查。

不要在 Dart 中手动构造生成的缺省类型再传给 Go。该类是库私有的（`_ShapeAbsent`），仅供解码器在 Go 发送
`null` 时使用。Dart 代码需要表达「没有值」时，应通过 nullable 参数传递 `null`。

通过共享的 `GoAbsent` marker 可以检测缺省值：

```dart
if (shape is GoAbsent) {
  // shape 在 Go 侧为 nil，其方法不可调用。
}
```

把缺省值回传给 Go 时会再次编码为 `null`，往返保持 nil。标记了 `fgb:"nullable"` 的字段仍然得到真正的
Dart `null`，因为字段级解码器在共享接口解码器运行之前就拦截了 `null`。

**依赖接口的限制。** 依赖接口在 Dart 中只是 marker——没有生成方法声明，因此其缺省类没有方法可以覆盖，
`StateError` 保护是空的。此时 `GoAbsent` marker 是区分缺省依赖接口值与真实值的唯一方式。

### 接口方法指令

输入包接口方法支持 `//fgb:sync`、`//fgb:async`、`//fgb:rename` 和 `//fgb:ignore`。写在接口方法上的
指令决定整份契约：它选定的 Dart 名称和调用模式会应用到该方法的每一个生成实现，因此只需在接口上写
一次，不必在每个实现类型上重复。

```go
type Loader interface {
	//fgb:async, rename = "fetch"
	Load(id int) (string, error)
}

type Remote struct{ Host string }

// 这里不需要再写指令，Dart 形状已由接口决定。
func (r Remote) Load(id int) (string, error) { /* ... */ }
```

```dart
abstract interface class Loader {
  Future<String> fetch({required int id});
}

final class Remote implements Loader {
  Future<String> fetch({required int id}) async { /* ... */ }
}
```

以下两种情况会在生成阶段直接报错，而不是产出无法编译的 Dart 类：

- 两个接口对同一个 Go 方法要求了不同的 Dart 形状。请让两处指令一致，或重命名其中一个 Go 方法。
- 某个实现没有桥接接口声明的方法，通常是因为该方法带了 `//fgb:ignore`。请去掉该指令，或让这个类型
  不再实现该接口。

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

## `toString()`

对于值结构体和命名值类型，生成器按 `ToString() string`、`String() string`、
`MarshalJSON() ([]byte, error)` 的顺序选择 Dart 的 `toString()` 实现。选中的 Go 方法通过同步
bridge 调用；`MarshalJSON` 返回的字节只在这条 `toString()` 路径上按 UTF-8 解码，普通 `[]byte`
字段和返回值仍使用原有 CST/DCO 或 standard codec。

当 `String()` 没有被选为 `toString()` 时，它会生成名为 `asString()` 的 Dart 方法。没有可用 Go
方法时，值结构体使用字段副本在 Dart 本地生成字符串。带必填参数的方法不能覆盖 Dart 的零参数
`Object.toString()`，生成器会发出 warning 并回退到下一候选；opaque handle 不生成字段格式化的
`toString()`。空值结构体也会生成稳定的 `Type()` 形式。

生成成 Dart extension type 的命名基础类型是例外：Dart 禁止 extension type 声明从 `Object` 继承的
成员，包括 `toString`。因此它们保留 Dart 默认的 `toString`，也不生成字段本地 fallback。被选中的
Go `String` 仍可通过 `asString()` 调用；被选中的 `ToString` 和 `MarshalJSON` 会使用安全的普通名称
`toStringValue` 和 `marshalJson`。增强型 Dart enum 可以正常覆盖 `toString`。

接口声明与具体生成类使用同一套选择规则；`String() string` 接口会导出 `toString()`，嵌入值
结构体提升的方法也会参与优先级判断。普通方法如果占用了保留的 `toString` 或 `asString` 名称，
生成器会发出 warning 并追加数字后缀。
