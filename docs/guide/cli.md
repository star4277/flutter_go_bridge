# CLI reference

Install the command with:

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

## `generate`

```sh
flutter_go_bridge_codegen generate [flags]
```

Generates the Go bridge and Dart API. Important flags are `--go-input`, `--go-output`,
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

## `create`

```sh
flutter_go_bridge_codegen create <name> [-t app|plugin]
```

Runs `flutter create`, removes colliding scaffold files, then applies the same template flow as
`integrate`. Use `--org`, `--platforms`, `--go-mod-dir`, and `--library-name` to customize it.

## `integrate`

```sh
flutter_go_bridge_codegen integrate [-t app|plugin]
```

Finds the nearest `pubspec.yaml` and adds Go/Gokit files to an existing project. Existing files
are skipped with warnings; entry files are preserved as comments in the runnable template.

`--no-write-lib`, `--no-integration-test`, `--no-dart-fix`, and `--no-dart-format` disable
optional integration steps.
