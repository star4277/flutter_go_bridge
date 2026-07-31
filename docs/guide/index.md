# What is flutter_go_bridge?

`flutter_go_bridge` is a Go → Dart/Flutter code generator built for [Gokit](/guide/gokit). You
write ordinary Go, run one command, and get a Dart API that calls into it over FFI.

It borrows the CLI shape and the generated-project layout from
`flutter_rust_bridge_codegen`, but it deliberately does not depend on Flutter Native Assets, Everything it emits runs on
a plain Dart VM as happily as it does inside a Flutter app.

## What you write, what you get

```go
// go/api/api.go
package api

func Add(a, b int) int { return a + b }

//fgb:async
func LoadAccount(id int) (Account, error) { /* ... */ }
```

```dart
import 'bridge_generated.dart';
import 'api/api.dart';

FlutterGoBridge.initialize(libraryPath: 'path/to/mylib.dll');

final answer = add(a: 20, b: 22);        // unmarked in Go → synchronous in Dart
final account = await loadAccount(id: 1); // //fgb:async → asynchronous in Dart
```

No hand-written binding layer sits between those two files. The generator reads your Go types with
`go/packages` and writes both halves.

## Commands

| Command | Purpose |
| --- | --- |
| [`generate`](/guide/cli#generate) | Read the Go input package, write the Go bridge and the Dart tree |
| [`generate --watch`](/guide/cli#generate-watch) | Regenerate whenever a watched Go file changes |
| [`run`](/guide/dev-server) | Drive `flutter run`, hot reloading Dart edits and restarting on Go edits |
| [`create`](/guide/cli#create) | Scaffold a fresh Flutter + Go project from zero |
| [`integrate`](/guide/cli#integrate) | Add the Go bridge to an existing Flutter project |

## How it decides things

Three decisions shape almost everything the generator does, and each has its own page:

- **Which codec a call uses.** Dart → Go prefers CST (real C structs), Go → Dart prefers DCO
  (`Dart_CObject`), and anything neither can express yet falls back to a pure-Dart standard codec.
  See [Serialization strategy](/concepts/serialization).
- **Whether a Dart method is sync or async.** Only `//fgb:async` produces a `Future`. There is
  never both a sync and an async version of one Go function. See [Sync and async](/concepts/sync-async).
- **Whether a struct travels by value or by handle.** Structs whose fields can all be serialized
  become Dart value classes; anything else becomes a `GoOpaque` handle. See
  [Structs and interfaces](/reference/structs-interfaces).

## Not supported yet

Generic functions, variadic parameters, and complex external named types are not handled. Private
Go identifiers — lowercase types, fields, methods, functions and constants — never take part in
generation at all.

## Where to go next

- [Getting started](/guide/getting-started) — install the CLI and make your first call.
- [Design principles](/concepts/) — the rules the generator holds itself to.
- [Type mapping](/reference/type-mapping) — the full Go ↔ Dart table.
