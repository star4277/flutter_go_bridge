# Structs and interfaces

Structs whose exported fields are all translatable become Dart value classes. Pointer fields become
nullable Dart fields. Anonymous embedded structs become Dart inheritance, with promoted fields
flattened on the wire.

```go
type Named struct { Name string }
type Account struct {
    Named
    ID int64
}
```

An unsupported field, a private-only state, or `//fgb:opaque` makes a struct a `GoOpaque` handle.
Opaque values must appear as `*T` in exported signatures and are released by `NativeFinalizer`.

Named Go interfaces become `abstract interface class` declarations. Implementing generated classes
use `implements`; interface values travel through the standard codec as `[implementation, payload]`
tagged unions. At least one bridgeable implementation is required.

