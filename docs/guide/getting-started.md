# Getting started

## Install the CLI

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

That installs `flutter_go_bridge_codegen`.

::: tip Building from source
The project templates are embedded into the binary with `go:embed`, and they include the Gokit
submodule. Run `git submodule update --init --recursive` before building from a checkout, or
`integrate` will fail at runtime with a missing-template error.

From a source checkout, `make local` builds for the current OS and architecture, then installs the
CLI to `GOBIN`, or to `GOPATH/bin` when `GOBIN` is empty.
:::

## Start a project

Pick whichever matches your situation.

### A new project

```sh
flutter_go_bridge_codegen create my_app
flutter_go_bridge_codegen create my_plugin -t plugin
```

`create` runs `flutter create` for you, removes the scaffold files that would collide with the
template, and then applies exactly the same injection flow as `integrate`. Because the entry files
are fresh, they are pure template — nothing of yours is commented out.

### An existing project

Run this anywhere inside a Flutter project; it walks up to find `pubspec.yaml`.

```sh
flutter_go_bridge_codegen integrate               # app template
flutter_go_bridge_codegen integrate -t plugin     # FFI plugin template
```

Existing files are skipped with a warning. The one exception is your entry file — `lib/main.dart`
for an app, `lib/<package>.dart` for a plugin — whose contents are commented out in place so the
result is a runnable self-contained demo.

Both commands are covered in full on the [CLI page](/guide/cli).

## Write a Go function

The template leaves you a Go module under `go/`. Add to `go/api/api.go`:

```go
package api

func Add(a, b int) int { return a + b }

//fgb:async
func Greet(name string) (string, error) {
	if name == "" {
		return "", errors.New("name is required")
	}
	return "hello, " + name, nil
}
```

Nothing here is special to the bridge except the `//fgb:async` line, which is what makes `Greet`
return a `Future` on the Dart side. See [Sync and async](/concepts/sync-async).

## Generate

```sh
flutter_go_bridge_codegen generate
```

The command finds its configuration automatically — see [Configuration](/guide/configuration) — or
takes everything on the command line:

```sh
flutter_go_bridge_codegen generate \
  --go-input go/api \
  --go-output go/bridge_generated.go \
  --dart-output lib/src/bridge_generated.dart \
  --library-name go_lib_example
```

While iterating, keep it running:

```sh
flutter_go_bridge_codegen generate --watch
```

## Call it from Dart

Generated code depends only on libraries shipped with the Dart SDK. Every generated entry point —
functions, methods and constructors — uses named parameters. `bridge_generated.dart` does not
re-export the API files, so import what you need:

```dart
import 'bridge_generated.dart'; // FlutterGoBridge / FgbPlatformException / GoOpaque
import 'api/api.dart';

void main() async {
  await FlutterGoBridge.initialize(libraryPath: 'path/to/mylib.dll');

  final answer = add(a: 20, b: 22);
  final message = await greet(name: 'world');
}
```

For a Web build, `FlutterGoBridge.initialize()` has a default Web initializer. It calls
`WidgetsFlutterBinding.ensureInitialized()` first and then the `FgbWasmLoader` embedded in
`bridge_generated.web.dart` before opening the bridge. You can pass `webInitializer` to replace
that default when a project needs a custom loader. Native and pure-Dart builds do not import
Flutter widgets; a pure-Dart Web caller must provide its own `webInitializer` instead of using the
Flutter asset loader. No separate `fgb_wasm_loader*.dart` files are generated.

Errors from Go arrive as `FgbPlatformException`; see [Returns and errors](/reference/returns-errors).

## Run the app

```sh
flutter_go_bridge_codegen run -d emulator-5554
```

`run` starts `flutter run` and watches both source trees: Dart edits hot reload, Go edits
regenerate and restart the process. The [dev server page](/guide/dev-server) explains why a Go
change cannot be hot reloaded.

## Next

- [Configuration](/guide/configuration) — every option and where it can live.
- [Output structure](/guide/output-structure) — what lands where, and why.
- [Type mapping](/reference/type-mapping) — what your Go signatures become in Dart.
