# Operator overloading

A Go method whose name and signature match one of the Dart operators is rendered as that operator
rather than as an ordinary method. No directive is involved: the name and the signature are the whole
contract, so a type that reads naturally in Go reads naturally in Dart too.

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

Nothing changes on the wire. An operator is another Dart spelling of a method that was already
bridged, so the Go side, the C ABI and the CST/DCO codecs are untouched.

## The operators

Each Go method name is the English reading of the symbol.

| Dart | Go method | Operands | Result |
| --- | --- | --- | --- |
| `+` | `Add` | one | own type |
| `-` | `Subtract` | one | own type |
| `*` | `Multiply` | one | own type |
| `/` | `Divide` | one | own type |
| `~/` | `TruncateDivide` | one | own type |
| `%` | `Modulo` | one | own type |
| `&` | `BitwiseAnd` | one | own type |
| <code>&#124;</code> | `BitwiseOr` | one | own type |
| `^` | `BitwiseXor` | one | own type |
| `<<` | `ShiftLeft` | one | own type |
| `>>` | `ShiftRight` | one | own type |
| `<` | `LessThan` | one | `bool` |
| `>` | `GreaterThan` | one | `bool` |
| `<=` | `LessThanOrEqualTo` | one | `bool` |
| `>=` | `GreaterThanOrEqualTo` | one | `bool` |
| `~` | `BitwiseNot` | none | own type |

## Conditions

Every one of these has to hold. A method that fails any of them keeps its ordinary named-parameter
shape, which is also what a method merely *called* `Add` gets:

- **The name matches the table exactly**, and is exported so Go itself can call it.
- **It is a method** on a bridged type — a top-level function is never an operator.
- **The operand is the receiver's own type**, `T` or `*T`. A relational operator on `Point` takes a
  `Point`, not an `int`. A pointer operand becomes a nullable Dart parameter (`Point?`).
- **The operand count matches**: exactly one for a binary operator, none for `~`. A
  `context.Context` parameter does not count — the bridge supplies it.
- **The result is right for the class**: the receiver's own type by value for arithmetic, bitwise and
  `~`; `bool` for `<`, `>`, `<=` and `>=`, because that is what Dart declares them to return. A
  pointer result is refused: `a + b` has to hand back a value rather than something that can be null.
- **At most one `error` result**, and it is optional. A non-nil one throws `FgbPlatformException` with
  `code == 'go_error'` on the Dart side, exactly as it does for an ordinary bridged method — an
  operator cannot return a second value to check. See
  [Returns and errors](/reference/returns-errors).
- **The method is synchronous.** A Dart operator cannot return a `Future`, so a method marked
  `//fgb:async` stays an ordinary one.

## The operator replaces the method

A qualifying method is exposed as the operator and nothing else: `Add` becomes `+`, not `+` plus an
`add()`. Use `//fgb:rename` on a second Go method if you also want a named entry point, or rename the
Go method itself if you did not want an operator at all.

The ordinary Dart name stays free, so a different method may take it:

```go
func (p Point) Add(other Point) Point { return p }

//fgb:rename = "add"
func (p Point) Combine(other Point) Point { return p }
```

## Where operators apply

Value structs and named types both carry them. A named type becomes a Dart extension type, and an
extension type declares operators like any other member:

```go
type Meters float64

func (m Meters) Add(other Meters) Meters { return m + other }
```

```dart
extension type const Meters(double value) {
  Meters operator +(Meters other) { /* ... */ }
}
```

A `GoOpaque` type cannot have one. Its value form is not allowed to cross the bridge at all — the
generator already reports `GoOpaque type T must be passed as *T` — and a `*T` result is not a valid
operator result. Expose a named method for that case.

## Limitations

- **`==` is never generated from a Go method.** Structural equality comes from the bridged fields
  instead; see [Equality and hashCode](/reference/structs-interfaces#equality-and-hashcode). A `==`
  taken from Go without a matching `hashCode` would break `Set` and `Map`, which is worse than not
  having one.
- **`[]`, `[]=` and unary `-` are not mapped.** None of them fits the one-operand, returns-its-own-type
  contract above: `[]` returns an element, `[]=` takes two operands and returns nothing, and unary `-`
  would compete with `Subtract` for a name. Each needs its own rule before it can be added.
- **An interface declaration wins.** If a bridged interface declares the same method, the
  implementation keeps the named member the interface promises and a warning says so. An operator
  provides no named member, so `implements` would otherwise be left without one. The interface method
  itself is never an operator either: its operand type is the interface, not the implementation.
- **A subclass cannot redeclare an inherited operator.** An anonymous embedded struct becomes a Dart
  superclass, and the subclass inherits its operators. Declaring the operator again on the subclass
  would mean an operand and a result of the subclass's own type, which is not a signature Dart accepts
  as an override of the parent's, so the generator reports the shadow instead of emitting Dart that
  fails to analyze.
