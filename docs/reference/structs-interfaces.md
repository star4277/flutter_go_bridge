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

### Equality and hashCode

A value struct travels by fields, so two instances decoded from the same wire bytes represent the
same Go value. Dart compares objects by identity by default, which would call them different, so a
value class with at least one bridged field gets a generated `operator ==` and `hashCode`:

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

`Set`, `Map` keys, `List.contains`, and Flutter's rebuild comparisons all work off these, so a value
that crossed the bridge behaves like one built in Dart.

### `toString()`

Value structs and named value types get a useful `toString()` without changing their wire codec.
The generator checks receiver methods in this order:

1. `ToString() string`
2. `String() string`
3. `MarshalJSON() ([]byte, error)`

An eligible method is called through the synchronous bridge. `MarshalJSON` bytes are decoded as
UTF-8 only for this `toString()` call; ordinary `[]byte` fields and results keep their existing
CST/DCO or standard encoding. `String()` is exposed as `asString()` when another method wins.

When no eligible method exists, the generated value class formats its bridged fields locally. A
method with required parameters cannot override Dart's zero-argument `Object.toString()` and is
skipped with a warning; selection then falls back to the next method. Opaque handles are not given
a field-based `toString()`. This fallback is also emitted for an empty value struct as `Type()`.

Named basic types rendered as Dart extension types are the exception: Dart forbids extension types
from declaring members inherited from `Object`, including `toString`. They therefore keep Dart's
default `toString` and do not get a local field fallback. A selected Go `String` remains callable as
`asString()`, while selected `ToString` and `MarshalJSON` methods are exposed under safe ordinary
names (`toStringValue` and `marshalJson`). Enhanced Dart enums may override `toString` normally.

The same selection is applied to interface declarations and their concrete generated classes. An
interface `String() string` therefore exposes `toString()`, and promoted methods from an embedded
value struct participate in the priority decision. Ordinary methods colliding with reserved
`toString` or `asString` names are suffixed with a warning.

Details worth knowing:

- **Collection fields compare by content.** Dart's `List` and `Map` are identity-compared, which
  contradicts Go value semantics, so the generated code routes every field through
  `fgbInternalDeepEquals`. Map comparison and hashing ignore entry order.
- **`hashCode` accumulates with 31.** Each field hash is folded in as `result * 31 + fieldHash`,
  seeded from `runtimeType.hashCode`.
- **Inheritance is included and kept distinct.** An anonymous embedded struct becomes a Dart
  superclass; the subclass compares its promoted fields too, and the `runtimeType` check stops a
  parent holding the same values from comparing equal to its subclass.
- **A struct with no bridged fields keeps identity equality.** With nothing to compare, every
  instance would equal every other, which is less useful than what `Object` already provides.
- **`GoOpaque` handles keep identity equality.** Their identity is the handle, and the same Go
  object can cross the bridge under more than one, so a generated `==` would promise more than it
  can deliver. Expose a Go method when a handle needs a value comparison.

`==` is the only operator that comes from the fields. Every other Dart operator comes from a matching
Go method instead — see [Operator overloading](/reference/operators).

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

Unsupported function/channel fields, unsupported types, nested pointers, by-value opaque fields,
and non-empty structs with no bridged fields cause automatic opaque fallback. Dependency interfaces
remain translatable through discovered implementations plus an interface-level opaque fallback. The
generator emits a warning naming a blocking field. Add `//fgb:opaque` when that is intentional.

Opaque types must appear as `*T` in exported parameters, results, and receivers. Passing them by
value is rejected because it would lose handle identity.

A genuine `struct{}` remains an empty value class with `const Marker();`. A non-empty struct whose
fields are all private or ignored becomes opaque because Dart cannot reconstruct its Go state.

## External dependency types

Reachable named types from dependency packages are analyzed too. Translatable external structs
become value classes in `_generated.dart`; private-only or unsupported ones become opaque handles.
Name collisions receive a Go package prefix, so two external `User` types may become `ModelsUser`
and `AuthUser`.

Reachable named non-empty interfaces are analyzed as tagged serialization unions. Their exported
named struct implementations are discovered only when the concrete type is already reachable from
the public API; unrelated imports do not change the union. Dependency implementations use stable
package-path/type-name tags, and a final interface-level `GoOpaque` member preserves implementations
that generated Go code cannot name. Dependency interface methods are not generated as Dart calls: the
Dart interface is marker-only, and its concrete classes carry either serialized fields or a `GoOpaque`
handle. A warning reports this method limitation.

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

A named, non-empty interface becomes an `abstract interface class`. Interfaces from the input
package include their bridged method declarations:

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

For an input-package interface, the generator collects bridged structs from that package whose `T`
or `*T` method set satisfies the interface, preserving declaration order for compatibility.

For a dependency interface, the generator keeps explicitly reachable public-API types and also scans
the already-loaded packages in the same third-party Go module as the interface declaration. This finds
implementations in sibling packages without scanning unrelated modules or the standard library.
Implementations are exported, non-generic named structs whose `T` or `*T` method set satisfies the
interface. Packages named `main`, packages hidden by Go's `internal` import rule, unexported types,
aliases, non-struct named types, and uninstantiated generic declarations are excluded because the
generated bridge cannot name them safely as concrete union members. They remain transportable through
the final interface-level opaque fallback, which boxes the interface value itself in the Go handle
registry. Runtime-registered implementations use the same fallback.

Value implementations travel by fields. Pointer-receiver-only implementations decode as pointers.
When both `T` and `*T` implement the interface, Go results accept either dynamic representation and
map both to the same Dart class. Opaque implementations travel as pointer handles. A dependency
struct that anonymously embeds a pointer also falls back to `GoOpaque`, because Dart cannot represent
a nullable superclass and the dependency cannot be annotated.

At least one bridged implementation is required for an interface in the input package. Dependency
interfaces always include the opaque fallback, even when no concrete type can be named. Unexported
methods are rejected for interfaces in the input package. Dependency interfaces are marker-only in
Dart: their methods are intentionally not generated because dependency declarations have no FGB call
directives or generated call entrypoints.

For an automatically discovered concrete implementation, the generator also inspects promoted methods
named `ToString`, `String`, and `MarshalJSON`. The selected method is bridged on the concrete Dart
class, so `toString()` dispatches to the runtime implementation (`ToString` > `String` > `MarshalJSON`).
`String()` remains available as `asString()` when it does not win. Other dependency methods are not
generated unless they are part of an input-package interface contract.

### Tagged-union encoding

Interface values use the standard codec with this logical shape:

```text
[implementorTag, payload]
```

The payload contains value-struct fields or an opaque handle. Passing an arbitrary Dart object that
merely `implements Shape` throws `ArgumentError`; it must be one of the registered generated Go
implementations or an interface fallback object previously returned by Go. Unknown tags fail decoding.

Input-package implementor order follows Go declaration position and uses numeric tags for compatibility.
Dependency implementations are ordered by full package path and Go type name, but their tags are stable
string identifiers derived from the same content; when both value and pointer dynamic forms exist, the
value form is canonical for Dart-to-Go encoding. A mismatched generation reports an unknown tag instead
of silently decoding the wrong type. Interface unions use the standard codec rather than the CST fast path.

### Nullability and directives

Interface parameters are non-nullable and required by default. `//fgb:nullable = "shape"` produces
an optional `Shape?` parameter and preserves a nil Go interface as a real Dart `null`.

A nil Go interface in a **non-nullable** position (a required field, parameter, or result) does not
fail. The Go encoder sends `null`, and the Dart decoder materializes a generated absent stand-in —
a `final class _ShapeAbsent implements Shape, GoAbsent` whose method overrides throw `StateError` and
whose `toString` returns `'Shape(absent)'`.

This bridge exists because a Go interface's zero value is `nil` — it is the only Go type that is
naturally nil without a pointer, yet the Dart side declares the field as non-nullable. The absent
stand-in lets a zero-value Go struct cross the bridge without a runtime error, consistent with how
other nil-able Go types are handled: `*T` maps nil to `null`, slices and maps normalize nil to
empty collections, and an interface normalizes nil to an absent object. The Dart type stays
non-nullable so existing code that expects `Shape` (not `Shape?`) continues to compile.

**The absent stand-in is a fallback, not a recommended pattern.** It trades a loud failure at the
encoding boundary for a quieter failure at the point of use, because the value looks like a normal
`Shape` to the Dart analyzer but has no real implementation behind it. If a field or parameter can
genuinely be nil, mark it `fgb:"nullable"` (for fields) or `//fgb:nullable` (for parameters) so the
Dart type becomes `Shape?` and the type system enforces the nil check explicitly.

Do not manually construct the generated absent type in Dart to pass to Go. The class is
library-private (`_ShapeAbsent`) and exists only for the decoder to use when Go sends `null`. Dart
code that needs to represent "no value" should pass `null` through a nullable parameter instead.

Detect an absent value with the shared `GoAbsent` marker:

```dart
if (shape is GoAbsent) {
  // shape was nil on the Go side; its methods are not callable.
}
```

Sending an absent value back to Go encodes it as `null` again, so the round trip preserves the nil.
A field marked `fgb:"nullable"` still receives a real Dart `null` instead of the absent stand-in,
because the field-level decoder intercepts `null` before the shared interface decoder runs.

**Dependency interface limitation.** A dependency interface is marker-only in Dart — it has no
generated method declarations. Its absent class therefore has no methods to override, so the
`StateError` guard is empty. The `GoAbsent` marker is the only way to distinguish an absent
dependency-interface value from a real one in that case.

Input-package interface methods support `//fgb:sync`, `//fgb:async`, `//fgb:rename`, and
`//fgb:ignore`. A directive on an interface method shapes the whole contract: the Dart name and call
mode it selects are applied to every generated implementation of that method, so you write the
directive once on the interface rather than repeating it on each implementor.

```go
type Loader interface {
	//fgb:async, rename = "fetch"
	Load(id int) (string, error)
}

type Remote struct{ Host string }

// No directive needed here; the interface already decided the Dart shape.
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

Two situations are reported as generation errors instead of producing a Dart class that cannot
compile:

- Two interfaces ask for different Dart shapes of the same Go method. Make the directives agree, or
  rename one of the Go methods.
- An implementation does not bridge a method the interface declares, usually because that method
  carries `//fgb:ignore`. Drop the directive, or keep the type out of the interface.

Dependency interface methods are marker-only and do not consume directives. See
[Directives and field tags](/reference/directives).

## Choosing a model

| Requirement | Recommended model |
| --- | --- |
| DTO, configuration, one-call input/output | Value struct |
| Dart edits fields and later sends the value back | Value struct, optionally `non-final` |
| Go identity and mutable state persist across calls | `//fgb:opaque` with `*T` |
| File, connection, cache, lock-owning object | Opaque plus explicit `Close` |
| Closed set of polymorphic Go implementations | Named interface in the input package, or a reachable dependency interface with discoverable implementations |
| Unstructured dynamic data | `any` / `Object?` |

## Common failures

| Symptom | Cause and fix |
| --- | --- |
| `must be passed as *T` | An opaque type was used by value; change it to `*T` |
| Unexpected `GoOpaque` output | Read the warning for an unsupported field; fix it or mark an input type opaque explicitly. Dependency pointer embedding falls back automatically |
| `only extend one type` | Multiple embedded value structs require composition or one chosen superclass |
| Embedded pointer/opaque error | It cannot be a Dart superclass; remodel it as a field |
| Duplicate wire field | Renaming or promotion produced the same key; choose unique names |
| Incompatible override | Go shadowing is invalid in Dart; rename one method |
| No bridged implementor | Add and expose at least one implementation in the input package |
| Custom Dart implementation rejected | Only generated Go implementations may cross the bridge |
