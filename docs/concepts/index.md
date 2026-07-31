# Design principles

The generator follows a few rules that keep the generated API predictable:

- Parse Go with the official `go/packages`, `go/ast`, and `go/types` packages.
- Choose a codec per call: CST for Dart-to-Go, DCO for Go-to-Dart, and a pure-Dart standard codec
  when either wire format cannot represent the types safely.
- Keep all FFI, allocation, dynamic-library and Dart API DL details in one generated runtime file.
- Export a stable dispatcher ABI instead of one C symbol per business function.
- Mirror Go source files and directories into Dart so generated APIs remain discoverable.

See [Serialization strategy](/concepts/serialization), [Stable ABI](/concepts/stable-abi), and
[Type mapping](/reference/type-mapping) for implementation details.

