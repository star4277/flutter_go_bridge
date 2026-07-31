# Directives and field tags

`flutter_go_bridge` has two annotation systems:

- **Declaration directives** use `//fgb:...` on functions, methods, types, constants, and interface methods.
- **Struct-field tags** use `fgb:"..."` on exported fields.

They use different syntax and are not interchangeable. This page documents the target, generated
effect, limitations, and failure modes of every directive and tag.

## Declaration syntax

The comment must use exact Go directive spelling, with no space after `//`:

```go
//fgb:async   // valid
// fgb:async  // error
```

Directives may use separate lines or a comma-separated line:

```go
//fgb:async, rename = "fetchUser"
func LoadUser() User { /* ... */ }
```

Quote a multi-name nullable value so its internal commas are not parsed as directive separators:

```go
//fgb:async, nullable = "tags,onEvent"
func Watch(tags []string, onEvent func(string)) {}
```

Names are case-sensitive. Unknown or empty directives, empty rename/parameter values, and conflicting
`sync` plus `async` directives fail generation.

## Quick reference

| Directive | Applies to | Effect |
| --- | --- | --- |
| `//fgb:sync` | Functions, methods, interface methods | Generates a synchronous Dart call; also the default |
| `//fgb:async` | Functions, methods, interface methods | Generates a `Future`-returning Dart call |
| `//fgb:ignore` | Functions, methods, types, constants, interface methods | Excludes the declaration |
| `//fgb:rename = "name"` | Functions, methods, types, constants, interface methods | Changes the generated Dart name |
| `//fgb:opaque` | Struct types | Forces `GoOpaque` handle semantics |
| `//fgb:nullable = "a,b"` | Function and method parameters | Preserves nil for listed nil-capable Go parameters |

## `//fgb:sync`

Unmarked calls are synchronous; the directive makes that choice explicit:

```go
//fgb:sync
func Add(a, b int) int { return a + b }
```

The Dart isolate blocks until Go returns. Sync mode is appropriate for short work that does not need
the Dart event loop. Callback parameters, `chan<- T`, and `fgb.StreamSink[T]` require
`//fgb:async` and are rejected in sync mode.

## `//fgb:async`

Async mode generates one `Future` API without adding an `Async` suffix:

```go
//fgb:async
func LoadUser(id int64) (User, error) { /* ... */ }
```

```dart
final user = await loadUser(id: 42);
```

It is required for Dart closure callbacks and stream parameters, and recommended for long-running
work that should not block the UI isolate. Interface declarations and their bridged implementations
must use compatible async/rename shapes.

## `//fgb:ignore`

- A function or method is omitted from the Dart API and dispatcher.
- A type and all of its methods are omitted. Referencing that type from another bridged signature is
  an error; use `opaque` when a handle is desired.
- A typed exported constant can be omitted.
- An interface method can be omitted from the generated Dart interface.

Unexported Go declarations are already ignored automatically.

## `//fgb:rename = "name"`

Rename changes the generated Dart name of a function, method, type, typed constant, or interface
method. It does not rename the Go declaration.

```go
//fgb:rename = "Position"
type Point struct { X, Y int }

//fgb:rename = "fetchUser"
func LoadUser(id int64) Point { /* ... */ }
```

Use valid, unique Dart identifiers: UpperCamelCase for types and lowerCamelCase for members. Put a
renamed constant in its own const spec; one directive on a multi-name spec applies the same rename to
each constant. Rename is also useful for resolving incompatible method shadowing created by embedded
Go structs.

## `//fgb:opaque`

Opaque forces a struct to stay on the Go heap and cross FFI as a handle:

```go
//fgb:opaque
type Counter struct { total int }

func NewCounter() *Counter { return &Counter{} }
func (c *Counter) Increment() int { c.total++; return c.total }
```

Dart gets a class extending `GoOpaque`. `NativeFinalizer` releases the handle automatically.
Opaque values must appear as `*T` in bridged signatures; passing them by value is rejected. Use
opaque for private mutable state, resources, or fields that should not be serialized. Structs may
also fall back to opaque automatically when a field cannot travel; an explicit directive records
that choice and suppresses the warning.

## `//fgb:nullable = "a,b"`

Nullable makes listed parameters optional and nullable in Dart, preserving Go nil:

```go
//fgb:async, nullable = "tags,scores,onEvent"
func Store(tags []string, scores map[string]int, onEvent func(string)) {}
```

Supported shapes are callbacks, slices, maps, byte/typed lists, and named interfaces. Null and empty
remain distinct. Scalars, strings, arrays, and value structs cannot use this directive; use a Go
pointer instead. Pointer parameters are already nullable and optional. Names must exactly match Go
parameter names, and callbacks still require async mode.

## Field-tag syntax

Options are comma-separated inside one `fgb` tag:

```go
type Item struct {
	Name  string `fgb:"rename:title"`
	Count int    `fgb:"non-final,defaultValue: 0"`
}
```

| Tag | Effect |
| --- | --- |
| `ignore` or `-` | Excludes the field from the Dart class and wire codec |
| `rename:name` | Renames the Dart field, constructor parameter, and wire key |
| `non-final` | Removes Dart's `final` modifier |
| `nullable` | Preserves nil for slice/map/typed-list/interface fields |
| `defaultValue: expr` | Adds a Dart constructor default |

Unknown options and empty rename/default values fail generation.

## `fgb:"ignore"` and `fgb:"-"`

The field is absent from Dart and both codec directions. Unexported fields, blank fields,
`json:"-"`, and `flutter_go_bridge:"-"` are also excluded. If every field of a non-empty struct is
excluded, the type falls back to `GoOpaque`; a genuine `struct{}` remains an empty value class.

## `fgb:"rename:name"`

Rename controls the Dart property, named constructor parameter, and wire key. Name resolution order
is:

1. `fgb:"rename:name"`;
2. the first `flutter_go_bridge:"name"` segment;
3. the first `json:"name"` segment;
4. lowerCamelCase of the Go field name.

Duplicate wire names are rejected. Use a valid, unique Dart field identifier.

## `fgb:"non-final"`

The generated Dart field becomes mutable and any class containing a non-final field loses its const
constructor. This does not create live Go state: a value struct is serialized again only when the
modified Dart object is passed into another call. Use `//fgb:opaque` for persistent Go-side object
identity.

## `fgb:"nullable"`

The field becomes nullable and its constructor argument becomes optional. Nil is preserved in both
directions. Supported field shapes are slices, maps, byte/typed lists, and named interfaces.
Callbacks are direct parameters only and cannot be struct fields. Scalars, arrays, and value structs
are rejected; pointer fields are already nullable, so adding the tag is an error.

## `fgb:"defaultValue: expr"`

The raw Dart expression becomes the optional constructor parameter's default:

```go
type Options struct {
	Retry int      `fgb:"defaultValue: 3"`
	Tags  []string `fgb:"defaultValue: const []"`
}
```

The expression must be a type-compatible Dart compile-time constant. It affects Dart construction,
not the Go zero value. `defaultValue:` consumes the rest of the tag so expressions can contain
commas; it must therefore be the last option:

```go
Values []int `fgb:"non-final,defaultValue: const [1, 2]"`
```

## Combined example

```go
//fgb:rename = "CatalogItem"
type Item struct {
	ID           int64
	Name         string            `fgb:"rename:title"`
	Count        int               `fgb:"non-final,defaultValue: 0"`
	Tags         []string          `fgb:"nullable"`
	InternalNote string            `fgb:"ignore"`
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

