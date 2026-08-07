# CLI reference

Install the command with:

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

From a source checkout, `make local` builds for the current OS and architecture, then copies the
executable to `GOBIN`, or to `GOPATH/bin` when `GOBIN` is empty.

## Version information

Use either version flag:

```sh
flutter_go_bridge_codegen -v
flutter_go_bridge_codegen --version
```

The output contains both the flutter_go_bridge version and the Go toolchain that built the executable:

```text
flutter_go_bridge_codegen version v1.2.3
Build with go1.25.0
```

The build line is important when diagnosing source compatibility. Compare it with the Go version used by
the project whose package is being parsed; a codegen executable built by an older Go toolchain may not
understand syntax or type-system behavior introduced by a newer project toolchain.

## `generate`

```sh
flutter_go_bridge_codegen generate [flags]
```

Generates both the Native and Web Go bridges plus one shared Dart API. `--target native|web` is a
deprecated compatibility option and does not change the dual output. Important flags are `--go-input`, `--go-output`,
`--dart-output`, `--library-name`, `--config-file`, `--watch`, `--no-dart-format`,
`--print-ast`, and `--stop-on-error`.

`--watch` polls local Go files every 400ms by default, excludes generated outputs, and continues
after a generation warning. It requires a local directory or file input, not a package pattern.

## `run`

```sh
flutter_go_bridge_codegen run -d <device> -- [flutter run args]
```

Starts `flutter run --machine`, hot reloads Dart changes, and regenerates/restarts for Go changes.
`-d all` is not supported because one daemon session targets one device.
For Web devices, it also runs Gokit `build-web` before startup and after each Go regeneration.

## `build`

```sh
flutter_go_bridge_codegen build <platform> -- [flutter build args]
```

Runs one shared Native/Web generation, then builds the requested Flutter platform. The platform is
the same positional target accepted by `flutter build`, for example:

```sh
flutter_go_bridge_codegen build web -- --release
flutter_go_bridge_codegen build windows -- --release
```

`build web` invokes Gokit `build-web` first so the current `.wasm`, `wasm_exec.js`, and manifest are
installed before `flutter build web`. Other targets use the Native platform builder. Both builders
pass their results through a platform-specific signing interface; the initial Native and Web
signers are no-ops reserved for future signing integration. Use `--project-dir` to select a Flutter
project explicitly. Plugin projects default to their runnable `example/` app.

## `create`

```sh
flutter_go_bridge_codegen create <name> [-t app|plugin]
```

Runs `flutter create`, removes colliding scaffold files, then applies the same template flow as
`integrate`. Use `--org`, `--platforms`, `--go-mod-dir`, and `--library-name` to customize it.
Cleanup is idempotent when a Flutter version omits an optional scaffold directory. Nested content
outside the platform `Classes/` scaffolds is preserved; iOS/macOS `Classes/` is replaced as a unit.

## `integrate`

```sh
flutter_go_bridge_codegen integrate [-t app|plugin]
```

Finds the nearest `pubspec.yaml` and adds Go/Gokit files to an existing project. Existing files
are skipped with warnings; entry files are preserved as comments in the runnable template.

`--no-write-lib`, `--no-integration-test`, `--no-dart-fix`, and `--no-dart-format` disable
optional integration steps.
