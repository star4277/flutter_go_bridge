# Structs and interfaces

The generator does not bridge every Go struct in the same way. A struct becomes either a Dart value
object or a `GoOpaque` handle, depending on whether its fields can cross the bridge. A named,
non-empty interface becomes a Dart interface implemented by known generated Go types.

| Go declaration | Dart output | Semantics |
| --- | --- | --- |
| Struct with translatable fields | `final class` or `class` | Copied, serialized, and rebuilt by fields |
| Struct marked `//fgb:opaque` | `final class ... extends GoOpaque` | Go owns state; Dart keeps a handle |
| Struct with unsupported state | Automatic `GoOpaque` fallback with a warning | Prevents invalid value copying |
| Named non-empty interface | `abstract interface class` | Only discovered Go implementations may cross |
| `any` / `interface{}` | `Object?` | Dynamic value; no generated interface declaration |

## Value structs

A struct whose bridged fields are all translatable becomes a value class:

```go
type User struct {
	ID       int64
	Name     string
	Nickname *string
	Tags     []string `fgb:"nullable"`
}

func SaveUser(user User) User { return user }
```

It produces approximately:

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
```

Value semantics mean that a Dart object is encoded into a new Go value on input and a Go result is
rebuilt as a new Dart object. The two sides do not share an address or automatically synchronize
mutations.

### Fields and constructors

Only exported fields are bridged. Unexported fields, `_`, `fgb:"ignore"`, `fgb:"-"`,
`flutter_go_bridge:"-"`, and `json:"-"` are skipped. Field/wire-name priority is `fgb:"rename:name"`,
the first `flutter_go_bridge` name, the first `json` name, then lowerCamelCase of the Go field name.
Duplicate wire names are generation errors.

Ordinary fields are `final` and `required`. Pointer fields are nullable and optional.
`fgb:"nullable"` also makes supported nil-capable fields nullable and optional. A `defaultValue`
removes `required`; a `non-final` field removes both `final` and the class's `const` constructor.
Classes are `final class` unless they participate in inheritance.

See [Directives and field tags](/reference/directives) for the complete tag syntax.

### Pointer fields and pointer receivers

A `*T` field is nullable data. A `*T` method receiver is different: for a value class the bridge
reconstructs a temporary Go receiver from the Dart fields. Go mutations do not write back into the
original Dart object:

```go
type CounterView struct { Count int }

func (c *CounterView) Add(delta int) int {
	c.Count += delta
	return c.Count
}
```

```dart
final view = CounterView(count: 1);
final nextCount = view.add(delta: 2);
print(view.count); // Still 1.
```

Return an updated struct for value-style updates. Use `GoOpaque` when object identity and mutable Go
state must persist across calls.

Recursive pointer types such as `type Node struct { Next *Node }` can be generated, but runtime
values must not contain a cycle. Encoders stop after 64 nested levels and report a codec error.
Nested pointers such as `**Node` are not an ordinary translatable field shape and cause opaque
fallback.

## `GoOpaque` structs

Opaque types keep the real pointer in the Go handle registry. Dart holds only a bridge reference and
an integer handle:

```go
//fgb:opaque
type Counter struct { total int }

func NewCounter() *Counter { return &Counter{} }

func (c *Counter) Add(delta int) int {
	c.total += delta
	return c.total
}
```

```dart
final class Counter extends GoOpaque {
  Counter.fgbInternal({
    required super.fgbBridge,
    required super.fgbHandle,
  });

  int add({required int delta}) { /* bridged call */ }
}
```

Repeated calls resolve the same handle and therefore the same Go state. `NativeFinalizer` releases
the handle after the Dart object is collected. Do not depend on prompt finalization for files,
sockets, or other resources that require deterministic shutdown; expose an explicit Go `Close`
method as well.

Unsupported function/channel fields, unsupported types, nested pointers, external non-empty
interfaces, by-value opaque fields, and non-empty structs with no bridged fields cause automatic
opaque fallback. The generator emits a warning naming the blocking field. Add `//fgb:opaque` when
that is intentional.

Opaque types must appear as `*T` in exported parameters, results, and receivers. Passing them by
value is rejected because it would lose handle identity.

A genuine `struct{}` remains an empty value class with `const Marker();`. A non-empty struct whose
fields are all private or ignored becomes opaque because Dart cannot reconstruct its Go state.

## External structs

Reachable named types from dependency packages are analyzed too. Translatable external structs
become value classes in `_generated.dart`; private-only or unsupported ones become opaque handles.
Name collisions receive a Go package prefix, so two external `User` types may become `ModelsUser`
and `AuthUser`.

## Anonymous embedding and inheritance

One anonymously embedded plain value struct becomes the Dart superclass:

```go
type Animal struct {
	Name string
	Legs int
}

type Dog struct {
	Animal
	Breed string
}
```

```dart
class Animal {
  const Animal({required this.name, required this.legs});
  final String name;
  final int legs;
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

Promoted fields are flattened on the wire rather than nested under `animal`. Embedded methods are
inherited; compatible shadowing produces `@override`.

Important restrictions:

- Dart has single inheritance, so only one embedded value struct may become a superclass;
- an embedded pointer cannot be a nullable superclass;
- an opaque struct cannot be a superclass;
- promoted wire-name collisions fail generation;
- Go permits incompatible method shadowing, while Dart overrides must be compatible; use
  `//fgb:rename` when needed.

When encoding interface values, the generated Dart code tests more-derived subclasses before their
parents so a `Dog` is not tagged as an `Animal`.

## Named interfaces

A named, non-empty interface from the input package becomes an `abstract interface class`:

```go
type Shape interface {
	Area() int
	Label() string
}

type Circle struct { Radius int }

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
```

Interface methods are declarations only. Calls dispatch through the concrete generated Dart class,
which owns either serialized fields or an opaque handle.

### Implementor discovery

The generator collects bridged structs in the input package whose `T` or `*T` method set satisfies
the interface. Value classes expose bridged pointer-receiver methods too; opaque implementors travel
as pointer handles. Dependency packages are not searched as an open-ended implementation universe.

At least one bridged implementation is required. Unexported interface methods are rejected, and
external named non-empty interfaces cannot be bridged automatically. Use `any` for dynamic values or
declare a controlled bridge interface and its implementations in the input package.

### Tagged-union encoding

Interface values use the standard codec with this logical shape:

```text
[implementorIndex, payload]
```

The payload contains value-struct fields or an opaque handle. Passing an arbitrary Dart object that
merely `implements Shape` throws `ArgumentError`; it must be one of the registered generated Go
implementations. Unknown tags fail decoding.

Implementor order follows Go declaration position, but numeric tags are generated protocol details.
Always deploy the Go bridge and Dart tree produced by the same generation run. Interface unions use
the standard codec rather than the CST fast path.

### Nullability and directives

Interface parameters are non-nullable and required by default. `//fgb:nullable = "shape"` produces
an optional `Shape?` parameter and preserves a nil Go interface. A nil named-interface result cannot
identify a concrete implementation and is rejected on the Dart side with `FormatException`; prefer
an explicit result struct or presence flag for optional results.

Interface methods support `//fgb:sync`, `//fgb:async`, `//fgb:rename`, and `//fgb:ignore`. The
generated implementation shape must remain compatible with the interface declaration. See
[Directives and field tags](/reference/directives).

## Choosing a model

| Requirement | Recommended model |
| --- | --- |
| DTO, configuration, one-call input/output | Value struct |
| Dart edits fields and later sends the value back | Value struct, optionally `non-final` |
| Go identity and mutable state persist across calls | `//fgb:opaque` with `*T` |
| File, connection, cache, lock-owning object | Opaque plus explicit `Close` |
| Closed set of polymorphic Go implementations | Named interface in the input package |
| Unstructured dynamic data | `any` / `Object?` |

## Common failures

| Symptom | Cause and fix |
| --- | --- |
| `must be passed as *T` | An opaque type was used by value; change it to `*T` |
| Unexpected `GoOpaque` output | Read the warning for an unsupported field; fix it or mark the type opaque explicitly |
| `only extend one type` | Multiple embedded value structs require composition or one chosen superclass |
| Embedded pointer/opaque error | It cannot be a Dart superclass; remodel it as a field |
| Duplicate wire field | Renaming or promotion produced the same key; choose unique names |
| Incompatible override | Go shadowing is invalid in Dart; rename one method |
| No bridged implementor | Add and expose at least one implementation in the input package |
| Custom Dart implementation rejected | Only generated Go implementations may cross the bridge |
| Nil interface result gives `FormatException` | Return an explicit optional-result model instead |
