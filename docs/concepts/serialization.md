# Serialization strategy

Every call is classified recursively from its parameters, receiver and results.

| Direction | Preferred codec | Wire representation |
| --- | --- | --- |
| Dart → Go | CST | Generated C-compatible structs and an arena for nested values |
| Go → Dart | DCO | `Dart_CObject` trees decoded into Dart values |
| Either direction fallback | Standard codec | Pure Dart codec for `map`, `any`, interfaces and other unsupported shapes |

Struct fields are encoded in declaration order. Strings, lists and nested structs use temporary
allocation managed by the generated runtime. Codec details never leak into API files.

The standard codec is not Flutter's `StandardMessageCodec`; it is implemented in generated Dart
and therefore also works in a plain Dart VM.

