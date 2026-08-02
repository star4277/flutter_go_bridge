# Type mapping

This page lists the currently supported Go-to-Dart mappings. The Dart column shows the actual types
used in generated APIs. Pointers, nullable tags, streams, and callbacks can additionally change
whether a parameter is nullable or required and whether a function returns `Future` or `Stream`.

## Basic types

| Go | Dart | Notes |
| --- | --- | --- |
| `bool` | `bool` | Direct mapping |
| `string` | `String` | Direct mapping |
| `int8`<br>`int16`<br>`int32`<br>`int64`<br>`int` | `int` | Dart-to-Go decoding checks the signed target range |
| `uint8`<br>`uint16`<br>`uint32` | `int` | Negative and out-of-range values are rejected |
| `uint64`<br>`uint`<br>`uintptr` | `BigInt` | Preserves unsigned values and platform-width semantics |
| `float32`<br>`float64` | `double` | `float32` input is converted to 32-bit precision in Go |

`complex64`, `complex128`, and `unsafe.Pointer` are not supported as ordinary bridged types.

### Named basic types

A named Go type becomes a Dart extension type rather than losing its API name:

```go
type UserID int64
type Status string
```

```dart
extension type const UserID(int value) {}
extension type const Status(String value) {}
```

Exported methods and same-type constants are emitted on the extension type. Its wire representation
still uses the underlying type.

## Slices, arrays, and typed lists

| Go | Dart | Notes |
| --- | --- | --- |
| `[]byte`<br>`[]uint8` | `Uint8List` | `byte` is an alias of `uint8` |
| `[]int32` | `Int32List` | Uses Dart typed data |
| `[]int64` | `Int64List` | Uses Dart typed data |
| `[]float64` | `Float64List` | Uses Dart typed data |
| `[]T` | `List<T>` | Except for the four specialized slices above |
| `[N]T` | `List<T>` | Dart-to-Go decoding requires exactly `N` elements |

Slice parameters are non-nullable and required by default. Use `//fgb:nullable` on parameters or
`fgb:"nullable"` on fields to preserve the difference between a nil and empty slice. Without it, a
nil Go slice is normalized to an empty Dart list on Go-to-Dart encoding.

## Maps and dynamic values

| Go | Dart | Notes |
| --- | --- | --- |
| `map[K]V` | `Map<K, V>` | Keys and values map recursively; maps use the standard codec |
| `any`<br>`interface{}` | `Object?` | Runtime dynamic values using the standard codec |

Supported map-key categories are booleans, strings, supported integers and floats, `time.Duration`,
`net/netip.Prefix`, `net/url.URL`, and named Go types whose underlying mapping is also a valid key.
Structs, collections, `BigInt`, `InternetAddress`, `UuidValue`, interfaces, and opaque handles are not
valid map keys.

`any` can only carry value graphs expressible by the standard codec. Arbitrary Go objects and
function values cannot be hidden inside `any`.

## Pointers and nullable types

| Go | Dart | Notes |
| --- | --- | --- |
| `*T` | `T?` | nil and null map both ways; nested pointers such as `**T` are unsupported |
| `*struct` | `XXX?` | A normal struct still travels by fields; a pointer alone does not make it opaque |
| `*OpaqueType` | `OpaqueType?` | Dart stores a Go handle rather than struct fields |

A pointer does not imply shared memory. A normal `*User` result is decoded into a Dart value object.
Only a `GoOpaque` handle continues to resolve to the same Go object across calls.

## Standard-library and dedicated mappings

| Go | Dart | Wire and boundary behavior |
| --- | --- | --- |
| `time.Time` | `DateTime` | Signed 64-bit Unix microseconds on the wire; Go → Dart uses `DateTime.fromMicrosecondsSinceEpoch`, Dart → Go uses `microsecondsSinceEpoch`, and sub-microsecond Go nanoseconds are truncated |
| `time.Duration` | `Duration` | Microseconds on the wire; sub-microsecond Go nanoseconds are truncated |
| `math/big.Int` | `BigInt` | Lossless big-integer encoding; `*big.Int` becomes `BigInt?` |
| `net/netip.Addr` | `InternetAddress` | IP text on the wire; zero address maps to an empty string; imports `dart:io` |
| `net/netip.Prefix` | `String` | CIDR text such as `192.168.1.0/24`; zero/invalid Prefix maps to an empty string |
| `net/url.URL` | `Uri` | Uses `URL.String()` and `Uri.parse` |
| `github.com/gofrs/uuid/v5.UUID` | `UuidValue` | UUID string on the wire; requires the Dart `uuid` package |

`time.Time` used RFC3339Nano text before the Unix-microseconds mapping. Regenerate the Go and Dart
outputs together when upgrading; mixing old and new generated files fails wire-type validation.

Pointers to these value types produce the nullable Dart equivalent.

### `net/netip.Prefix`

`netip.Prefix` maps to `String`, not `InternetAddress`, because `InternetAddress` does not carry a CIDR
prefix length:

```go
type Route struct {
	Destination netip.Prefix
}
```

```dart
final class Route {
  const Route({required this.destination});
  final String destination;
}
```

Dart-to-Go decoding validates non-empty text with `netip.ParsePrefix`. An empty string decodes to
`netip.Prefix{}`, and an invalid Go Prefix encodes as an empty string. Invalid non-empty CIDR text is
reported as `invalid_argument`.

### UUID dependency

Generated UUID APIs import `package:uuid/uuid.dart`. When `uuid` is missing from the target project's
`pubspec.yaml`, codegen uses Flutter or FVM to run `flutter pub add uuid`.

## Structs and named interfaces

| Go | Dart | Notes |
| --- | --- | --- |
| `type XXX struct { ... }` | `class XXX` | Becomes a value class when every bridged field maps successfully |
| `type XXX struct { ... }`<br>`//fgb:opaque` | `class XXX extends GoOpaque` | Go owns the object; exported signatures use `*XXX` |
| `type XXX interface { ... }` | `abstract interface class XXX` | Generated concrete classes use `implements XXX` |
| `struct{}` | `class XXX` | A named empty struct gets an empty `const` constructor |

Value structs travel by fields and use named Dart constructor parameters. Unsupported/private-only
state or explicit `//fgb:opaque` selects handle semantics. Anonymous embedded value structs become
Dart inheritance with flattened promoted fields.

Named non-empty interfaces use a `[implementation index, payload]` tagged union through the standard
codec. Input-package interfaces expose generated methods. Reachable dependency interfaces are
marker-only, discover exported named struct implementations across the loaded dependency graph, and
use an interface-level `GoOpaque` fallback for concrete runtime types generated Go cannot name.
See [Structs and interfaces](/reference/structs-interfaces) for classification, inheritance,
implementor discovery, and restrictions.

## Atomic wrapper types

| Go | Dart | Notes |
| --- | --- | --- |
| `sync/atomic.Bool` | `bool` | Reads with Go `Load()` and initializes with `Store()` |
| `sync/atomic.Int32`<br>`sync/atomic.Int64` | `int` | Uses the basic type returned by `Load()` |
| `sync/atomic.Uint32`<br>`sync/atomic.Uint64`<br>`sync/atomic.Uintptr` | `int` or `BigInt` | Determined by the concrete `Load()` result type |
| Compatible wrapper in an `atomic` package | `T` | Requires `Load() T` and `Store(T)` for a supported basic `T` |

Atomic values cross as snapshots. Dart and Go do not share the same atomic variable. Generated
codecs use pointers internally to avoid copying `sync/atomic`'s `noCopy` state. A direct atomic value
or a struct containing one cannot be passed or returned by value; use a pointer.

Atomic state nested in another value-struct field makes the outer struct fall back to `GoOpaque` and
emits a warning, so no lock state is copied. Slices and maps whose elements contain atomic state also
use synthetic `GoOpaque` token classes (for example `[]atomic.Int64` becomes
`AtomicInt64Slice extends GoOpaque`). These tokens have no fields or methods: Dart may only retain and
pass them back to Go. Arrays containing atomic state remain unsupported because even returning the
array would copy its elements. Use a slice or an explicitly opaque named wrapper instead.

Every GoOpaque transfer receives an independent handle. If encoding or message delivery fails, only
the handles created for that transfer are rolled back; live handles from other calls are unaffected.

## Error

| Go | Dart | Notes |
| --- | --- | --- |
| `error` | `FgbPlatformException` | The error slot is removed from the return type; non-nil throws |

Errors may appear anywhere and several may be declared. See [Returns and errors](/reference/returns-errors).

## Streams, callbacks, and context

These mappings depend on the containing function:

| Go | Dart | Notes |
| --- | --- | --- |
| `chan<- T` | `StreamSink<T>` | Direct parameter of a `//fgb:async` function only |
| `fgb.StreamSink[T]` | `StreamSink<T>` | Supports bridge Add, AddError, and Close operations |
| `func(A, B) R` | `FutureOr<R> Function(A, B)` | Direct parameter of a `//fgb:async` function only |
| `context.Context` | Omitted from the Dart signature | Created by the runtime; stream cancellation triggers cancel |
| `fgb.DartOpaque` | `Object` | A Dart object crosses Go through a registry handle |
| `*fgb.DartOpaque` | `Object?` | Null represents no registered Dart object |

An async call with exactly one owned channel/sink and no ordinary value returns `Stream<T>` directly;
otherwise the sink remains a `StreamSink<T>` parameter. See [Stream](/reference/stream).

Callbacks accept synchronous or asynchronous Dart closures through `FutureOr<R>`. Nested, generic,
and variadic function types are unsupported. See [Dart closure callbacks](/reference/callbacks).

## Nullable rules

| Go shape | Default Dart parameter | With nullable marking |
| --- | --- | --- |
| `*T` | Optional `T?` | Already nullable; no marking needed |
| `[]T` | `required List<T>` | Optional `List<T>?`, preserving nil |
| `map[K]V` | `required Map<K, V>` | Optional `Map<K, V>?`, preserving nil |
| `func(...)` | Required `FutureOr<...> Function(...)` | Optional nullable callback |
| Named interface | Required interface type | Optional nullable interface |

Use `//fgb:nullable = "name"` for parameters and `fgb:"nullable"` for fields. Scalars, strings,
arrays, and value structs are not nil-capable without a Go pointer. See
[Directives and field tags](/reference/directives).

## Unsupported shapes

- generic functions and generic callbacks;
- variadic functions and callbacks;
- `complex64`, `complex128`, and `unsafe.Pointer`;
- nested pointers such as `**T`;
- unnamed non-empty interfaces;
- function types outside direct parameters and nested function types;
- bidirectional and receive-only channels; streams support direct `chan<- T` parameters only;
- external types whose fields cannot be mapped recursively; they may fail or follow opaque fallback
  rules when they are structs.

Private functions, methods, types, and constants are omitted. Private struct fields never cross the
bridge.

All generated encoders reject values nested beyond 64 levels. This bounds malicious inputs and
turns cyclic runtime object graphs into a codec error instead of a stack overflow.
