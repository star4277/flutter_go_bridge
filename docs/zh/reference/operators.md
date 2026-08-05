# 运算符重载

方法名和签名同时匹配某个 Dart 运算符的 Go 方法，会渲染成那个运算符，而不是普通方法。不需要任何
指令：名字和签名就是全部约定，所以在 Go 侧读起来自然的类型，在 Dart 侧也读起来自然。

```go
type Point struct {
	X int
	Y int
}

func (p Point) Add(other Point) Point { return Point{X: p.X + other.X, Y: p.Y + other.Y} }
func (p Point) LessThan(other Point) bool { return p.X*p.X+p.Y*p.Y < other.X*other.X+other.Y*other.Y }
```

```dart
final class Point {
  final int x;
  final int y;

  const Point({required this.x, required this.y});

  Point operator +(Point other) {
    return FlutterGoBridge.instance.fgbInternalCall0(this, other);
  }

  bool operator <(Point other) {
    return FlutterGoBridge.instance.fgbInternalCall1(this, other);
  }
}
```

```dart
final sum = Point(x: 1, y: 2) + Point(x: 3, y: 4);
final nearer = a < b;
```

wire 上什么都没变。运算符只是一个本来就已经桥接的方法的另一种 Dart 写法，Go 侧、C ABI 和
CST/DCO 编解码都不动。

## 运算符对照

每个 Go 方法名都是符号的英文读法。

| Dart | Go 方法 | 操作数 | 返回值 |
| --- | --- | --- | --- |
| `+` | `Add` | 一个 | 自身类型 |
| `-` | `Subtract` | 一个 | 自身类型 |
| `*` | `Multiply` | 一个 | 自身类型 |
| `/` | `Divide` | 一个 | 自身类型 |
| `~/` | `TruncateDivide` | 一个 | 自身类型 |
| `%` | `Modulo` | 一个 | 自身类型 |
| `&` | `BitwiseAnd` | 一个 | 自身类型 |
| <code>&#124;</code> | `BitwiseOr` | 一个 | 自身类型 |
| `^` | `BitwiseXor` | 一个 | 自身类型 |
| `<<` | `ShiftLeft` | 一个 | 自身类型 |
| `>>` | `ShiftRight` | 一个 | 自身类型 |
| `<` | `LessThan` | 一个 | `bool` |
| `>` | `GreaterThan` | 一个 | `bool` |
| `<=` | `LessThanOrEqualTo` | 一个 | `bool` |
| `>=` | `GreaterThanOrEqualTo` | 一个 | `bool` |
| `~` | `BitwiseNot` | 没有 | 自身类型 |

## 全部条件

下面每一条都必须成立。任何一条不满足，方法就保持普通的命名参数形态——只是恰好**叫** `Add`
的方法也是这个结果：

- **方法名与表格完全一致**，并且首字母大写，Go 自己也能调用。
- **必须是方法**，绑定在被桥接的类型上。顶层函数永远不会变成运算符。
- **操作数就是接收者自己的类型**，`T` 或 `*T`。`Point` 上的比较运算符接收 `Point`，不是 `int`。
  指针操作数在 Dart 侧成为可空参数（`Point?`）。
- **操作数个数正确**：二元运算符恰好一个，`~` 没有。`context.Context` 参数不计入——由桥接层提供。
- **返回值符合该类运算符**：算术、位运算和 `~` 返回接收者自身类型的值；`<`、`>`、`<=`、`>=`
  返回 `bool`，因为 Dart 就是这么声明它们的。**指针返回值被拒绝**：`a + b` 必须交回一个值，
  而不是可能为 null 的东西。
- **最多一个 `error` 返回值**，可选。非 nil 时在 Dart 侧抛 `FgbPlatformException`，
  `code` 为 `go_error`，和普通桥接方法完全一样——运算符没法再多返回一个值让调用方检查。
  见[返回值与错误](/zh/reference/returns-errors)。
- **必须是同步方法。** Dart 运算符不能返回 `Future`，所以标了 `//fgb:async` 的方法保持普通形态。

## 运算符会取代原来的方法

满足条件的方法只以运算符暴露，不会额外再给一份：`Add` 变成 `+`，而不是 `+` 加一个 `add()`。
如果同时还想要一个具名入口，用 `//fgb:rename` 另开一个 Go 方法；如果本来就不想要运算符，
改掉 Go 方法名即可。

原来的 Dart 名字是空着的，别的方法可以占用：

```go
func (p Point) Add(other Point) Point { return p }

//fgb:rename = "add"
func (p Point) Combine(other Point) Point { return p }
```

## 哪些类型可以有运算符

值结构体和命名类型都可以。命名类型生成 Dart extension type，而 extension type 声明运算符和声明
其他成员一样：

```go
type Meters float64

func (m Meters) Add(other Meters) Meters { return m + other }
```

```dart
extension type const Meters(double value) {
  Meters operator +(Meters other) { /* ... */ }
}
```

`GoOpaque` 类型不能有运算符。它的值形态根本不允许跨界——生成器本来就会报
`GoOpaque type T must be passed as *T`——而 `*T` 又不是合法的运算符返回值。这种情况请暴露一个
普通具名方法。

## 限制

- **`==` 永远不会从 Go 方法生成。** 结构相等来自桥接字段，见
  [相等性与 hashCode](/zh/reference/structs-interfaces#相等性与-hashcode)。只从 Go 取 `==`
  而 `hashCode` 仍是引用哈希，会直接破坏 `Set` 和 `Map`，比不生成更糟。
- **`[]`、`[]=` 和一元 `-` 不做映射。** 它们都不符合上面「一个操作数、返回自身类型」的约定：
  `[]` 返回元素类型，`[]=` 有两个操作数且没有返回值，一元 `-` 还会和 `Subtract` 抢名字。
  每一个都需要各自的规则才能加进来。
- **接口声明优先。** 如果某个被桥接的接口声明了同名方法，实现类保留接口承诺的具名成员，
  并给出 warning 说明原因。运算符不提供具名成员，否则 `implements` 就会少一个成员。
  接口方法本身也不会变成运算符：它的操作数类型是接口，不是实现类型。
- **子类不能重新声明继承来的运算符。** 匿名嵌入的结构体成为 Dart 父类，子类会继承它的运算符。
  子类再声明一遍，意味着操作数和返回值都是子类自己的类型，而这不是 Dart 认可的父类运算符覆盖签名，
  所以生成器直接报告这个遮蔽，而不是产出过不了 analyze 的 Dart 代码。
