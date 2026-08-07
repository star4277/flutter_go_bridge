# flutter_go_bridge

<p align="center">
  <strong>A code-generation bridge between Go and Dart.</strong>
</p>

<p align="center">
  Turn Go service APIs into type-safe Dart and Flutter interfaces with generated bindings, a stable FFI ABI, and no Flutter runtime dependency in the generated Dart code.
</p>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://star4277.github.io/flutter_go_bridge"><img alt="Documentation" src="https://img.shields.io/badge/docs-online-4969ed"></a>
  <a href="./LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-green"></a>
  <a href="https://github.com/star4277/flutter_go_bridge/stargazers"><img alt="GitHub stars" src="https://img.shields.io/github/stars/star4277/flutter_go_bridge?style=flat"></a>
  <a href="https://github.com/star4277/flutter_go_bridge/actions"><img alt="GitHub Actions" src="https://img.shields.io/github/actions/workflow/status/star4277/flutter_go_bridge/docs.yml?branch=main&label=docs"></a>
</p>

## Documentation

The complete English documentation is available at:

**https://star4277.github.io/flutter_go_bridge**

It covers installation, configuration, generated output, serialization, type mapping, directives,
structs, interfaces, streams, callbacks, return values, and error handling.

## What it does

Write ordinary Go code and let `flutter_go_bridge_codegen` produce the bridge:

```text
Go package
    │
    │ go/packages + go/types
    ▼
flutter_go_bridge_codegen
    ├── bridge_generated.go
    └── mirrored Dart API tree
            └── bridge_generated.dart keeps every FFI detail
```

```go
package api

import "errors"

type User struct {
	ID   int64
	Name string
}

func Add(a, b int) int {
	return a + b
}

//fgb:async
func LoadUser(id int64) (User, error) {
	if id <= 0 {
		return User{}, errors.New("id must be positive")
	}
	return User{ID: id, Name: "Gopher"}, nil
}
```

The generated Dart API uses named parameters and normal Dart types:

```dart
final total = add(a: 20, b: 22);
final user = await loadUser(id: 1);
print(user.name);
```

Unmarked functions are synchronous. Add `//fgb:async` only when the Dart API should return a
`Future`.

## Why flutter_go_bridge

- **Ordinary Go source** — APIs are parsed with the official `go/packages`, `go/ast`, and `go/types`
  toolchain. No custom Go syntax or interface definition language is required.
- **Dart-first generated API** — every Go source file gets a matching Dart file, parameters are named,
  and structs become typed Dart classes.
- **FFI stays internal** — dynamic library loading, native memory, codecs, handles, and Dart API DL are
  isolated in `bridge_generated.dart`.
- **No Flutter SDK dependency in generated bindings** — generated APIs use Dart SDK libraries and do
  not import `package:flutter/services.dart` or depend on Flutter Native Assets.
- **Per-call codec selection** — fast CST/DCO paths are used where possible; maps, dynamic values, and
  interfaces fall back to the built-in standard codec when needed.
- **Stable native ABI** — adding a Go function does not add a new exported C symbol. Calls use a fixed
  dispatcher ABI, so Gokit and CMake integration remains stable.
- **Stateful Go objects** — serializable structs become Dart value classes; stateful or unsupported
  structs become `GoOpaque` handles released by `NativeFinalizer`.
- **Streams and callbacks** — expose Go producers as Dart `Stream<T>` and pass synchronous or async
  Dart closures into Go functions.

## Install

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

This installs the `flutter_go_bridge_codegen` command.

When building the code generator from a source checkout, initialize the Gokit submodule first:

```sh
git submodule update --init --recursive
```

## Quick start

### Create a new project

```sh
flutter_go_bridge_codegen create my_app
```

For an FFI plugin:

```sh
flutter_go_bridge_codegen create my_plugin -t plugin
```

`create` runs `flutter create` and applies the Go/Gokit bridge template, leaving a runnable project.

### Integrate an existing project

Run the command anywhere inside the Flutter project:

```sh
flutter_go_bridge_codegen integrate
```

For an existing FFI plugin:

```sh
flutter_go_bridge_codegen integrate -t plugin
```

The command finds the nearest `pubspec.yaml`, adds the Go module and Gokit build files, generates the
initial bridge, and preserves existing files whenever possible.

### Generate bindings

```sh
flutter_go_bridge_codegen generate
```

During development, regenerate automatically:

```sh
flutter_go_bridge_codegen generate --watch
```

Or let the CLI run Flutter and coordinate both source trees:

```sh
flutter_go_bridge_codegen run -d emulator-5554
```

Dart changes use hot reload. Go changes regenerate the bridge and restart the application process so
the rebuilt platform artifact is loaded. When the device is Web, `run` also invokes Gokit
`build-web` before startup and after Go changes.

For a one-shot platform artifact, generate and build together:

```sh
flutter_go_bridge_codegen build web -- --release
flutter_go_bridge_codegen build windows -- --release
```

## Configuration

The CLI discovers these files automatically:

- `.flutter_go_bridge.yml`, `.flutter_go_bridge.yaml`, or `.flutter_go_bridge.json`;
- `flutter_go_bridge.yml`, `flutter_go_bridge.yaml`, or `flutter_go_bridge.json`;
- the `flutter_go_bridge` section in `pubspec.yaml`.

A minimal configuration looks like this:

```yaml
go_input: go/api
go_output: go/bridge_generated.go
dart_output: lib/src/bridge_generated.dart
dart_entrypoint_class_name: FlutterGoBridge
dart_format: true
```

`library_name` is optional and defaults to `go_lib_<pubspec package name>`. Command-line flags
override configuration files.

See the [configuration reference](https://star4277.github.io/flutter_go_bridge/guide/configuration)
for every option.

## Generated output

Given this Go module:

```text
go/
├── go.mod
└── api/
    ├── api.go
    └── account.go
```

Code generation produces one Go bridge and a mirrored Dart tree:

```text
go/
├── bridge_generated.go
├── bridge_generated_web.go
├── internal/fgb/fgb_generated.go
└── api/
    ├── api.go
    └── account.go

lib/src/
├── bridge_generated.dart
├── bridge_generated.io.dart
├── bridge_generated.web.dart
└── api/
    ├── api.dart
    └── account.dart
```

- `bridge_generated.go` contains the Native cgo exports, dispatchers, and Go codecs.
- `bridge_generated_web.go` contains the pure-Go `js/wasm` dispatcher and standard codec.
- `bridge_generated.dart` contains shared types/codecs and conditionally selects the Native or Web
  wire.
- Mirrored Dart files contain the public classes, functions, methods, interfaces, and constants.

## Serialization model

Each call selects a transport from all types reachable through its parameters, receiver, and return
values:

| Direction | Preferred path | Purpose |
| --- | --- | --- |
| Dart → Go | CST | Real C wire structs with inline scalars and short-lived arenas for nested values |
| Go → Dart | DCO | `Dart_CObject` values posted or returned directly to Dart |
| Either direction | Standard codec | Fallback for maps, `any`, named interfaces, and other dynamic shapes |

This selection is generated per call. Application code works only with the public Dart API and does
not choose codecs manually.

## Supported API shapes

| Go | Generated Dart |
| --- | --- |
| `bool`, `string` | `bool`, `String` |
| `int8` through `int64`, `int` | `int` |
| `uint8`, `uint16`, `uint32` | `int` with range checks |
| `uint64`, `uint`, `uintptr` | `BigInt` |
| `float32`, `float64` | `double` |
| CGo scalars such as `C.char`, `C.int`, `C.size_t`, typedefs, and enums | Underlying `int`, `BigInt`, or `double` |
| `[]byte`, `[]int32`, `[]int64`, `[]float64` | Dart typed lists |
| `[]T`, `[N]T`, `map[K]V` | `List<T>`, `List<T>`, `Map<K, V>` |
| `time.Time`, `time.Duration`, `math/big.Int` | `DateTime`, `Duration`, `BigInt` |
| `net/netip.Addr`, `net/netip.Prefix`, `net/url.URL` | `InternetAddress`, `String`, `Uri` |
| `github.com/gofrs/uuid/v5.UUID` | `UuidValue` |
| `type XXX struct { ... }` | `class XXX` or `class XXX extends GoOpaque` |
| `type XXX interface { ... }` | `abstract interface class XXX` |
| `error` | `FgbPlatformException` |
| `chan<- T`, `fgb.StreamSink[T]` | `Stream<T>` or `StreamSink<T>` |
| `func(A) R` parameter | `FutureOr<R> Function(A)` |

See the complete [type mapping](https://star4277.github.io/flutter_go_bridge/reference/type-mapping),
including pointer, nullable, collection, interface, and unsupported-type rules.

## Core bridge features

### Structs and interfaces

Serializable Go structs become Dart value classes. Anonymous embedded structs become Dart
inheritance, and promoted fields are flattened on the wire. Named Go interfaces become Dart
`abstract interface class` declarations. Interfaces from dependencies discover exported concrete
types across the loaded package graph and use a `GoOpaque` fallback for unnameable runtime implementations.

Structs with state that cannot be serialized can be marked explicitly:

```go
//fgb:opaque
type Counter struct {
	total int
}
```

They become `GoOpaque` handle classes and retain Go-side identity across calls.

### Streams

A send-only channel is enough to expose a Go-owned Dart stream:

```go
//fgb:async
func Count(out chan<- int) {
	for value := range 5 {
		out <- value
	}
}
```

```dart
await for (final value in count()) {
  print(value);
}
```

Use `fgb.StreamSink[T]` when Go also needs to add error events or close the stream explicitly.

### Dart closure callbacks

```go
//fgb:async
func Transform(input string, mapper func(string) string) string {
	return mapper(input)
}
```

```dart
final value = await transform(
  input: 'go',
  mapper: (text) => text.toUpperCase(),
);
```

The generated callback type uses `FutureOr`, so both synchronous and asynchronous Dart closures are
accepted.

### Return values and errors

- One non-error Go result stays a normal Dart value.
- Multiple non-error results become a Dart record.
- Named Go results become named record fields.
- `error` may appear anywhere in the Go result list.
- Non-nil errors throw `FgbPlatformException`; several error results are available through
  `FgbPlatformException.goErrors`.

## CLI overview

| Command | Purpose |
| --- | --- |
| `generate` | Generate the Go bridge and mirrored Dart API |
| `generate --watch` | Regenerate when Go source changes |
| `run` | Run Flutter, hot reload Dart, and restart after Go changes |
| `build` | Generate once and build a Flutter platform through the signing boundary |
| `create` | Create a new Flutter app or FFI plugin with Go integration |
| `integrate` | Add the bridge to an existing Flutter project |

The full command and flag reference is in the
[CLI documentation](https://star4277.github.io/flutter_go_bridge/guide/cli).

Release workflow and version management are documented in the
[releasing guide](https://star4277.github.io/flutter_go_bridge/guide/releasing).

## Development

Clone with submodules, then run the Go test suite:

```sh
git clone --recurse-submodules https://github.com/star4277/flutter_go_bridge.git
cd flutter_go_bridge
go test ./...
```

Build the CLI locally:

```sh
go build ./cmd/flutter_go_bridge_codegen
```

To install the CLI into the same directory `go install` would use, run:

```sh
make local
```

`make local` builds for the current OS and architecture, then copies the executable to `GOBIN`, or
to `$(go env GOPATH)/bin` when `GOBIN` is empty.

Build release archives with the Makefile:

```sh
make windows-amd64
make linux-amd64
make macos-arm64
```

The documentation site uses Bun:

```sh
cd docs
bun install
bun run typecheck
bun run build
```

## License

`flutter_go_bridge` is available under the [MIT License](./LICENSE).

Generated codec code also incorporates or follows third-party components under their respective
licenses. See [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

## Star History

<a href="https://www.star-history.com/?repos=star4277%2Fflutter_go_bridge&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=star4277/flutter_go_bridge&type=date&theme=dark&legend=top-left&sealed_token=ntAVkzoAipnCqu0fF5eSnrqgSH6664QfHmXBikwXOX-CgEG9899ZalyZnHAVHUQZBCM_j6q6_c7xuZ5Eno9QPsXyzbm733barobb5HlJE3FFNOKo08pW3g" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=star4277/flutter_go_bridge&type=date&legend=top-left&sealed_token=ntAVkzoAipnCqu0fF5eSnrqgSH6664QfHmXBikwXOX-CgEG9899ZalyZnHAVHUQZBCM_j6q6_c7xuZ5Eno9QPsXyzbm733barobb5HlJE3FFNOKo08pW3g" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=star4277/flutter_go_bridge&type=date&legend=top-left&sealed_token=ntAVkzoAipnCqu0fF5eSnrqgSH6664QfHmXBikwXOX-CgEG9899ZalyZnHAVHUQZBCM_j6q6_c7xuZ5Eno9QPsXyzbm733barobb5HlJE3FFNOKo08pW3g" />
 </picture>
</a>
