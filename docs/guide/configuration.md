# Configuration

`generate` and `run` read configuration from the current project directory. The first matching
file wins:

```text
.flutter_go_bridge.yml
.flutter_go_bridge.yaml
.flutter_go_bridge.json
flutter_go_bridge.yml
flutter_go_bridge.yaml
flutter_go_bridge.json
pubspec.yaml -> flutter_go_bridge:
```

Command-line flags override values loaded from a file. Paths are resolved relative to the
configuration file (or the current directory when flags are used).

## Example

```yaml
go_input: go/api
go_output: go/bridge_generated.go
dart_output: lib/src/bridge_generated.dart
library_name: go_lib_example
dart_entrypoint_class_name: FlutterGoBridge
dart_format: true
stop_on_error: true
dart_format_line_length: 100
```

## Options

| Key | Default | Meaning |
| --- | --- | --- |
| `base_dir` | current directory | Base used to resolve relative paths. |
| `go_input` | required | Directory, `.go` file, or package pattern resolving to one Go package. |
| `go_output` | nearest `go.mod` root + `bridge_generated.go` | Generated Go cgo bridge. |
| `dart_output` | `lib/src/bridge_generated.dart` | Generated Dart runtime/bridge file. A directory gets the default filename. |
| `library_name` | `go_lib_<pubspec name>` | Dynamic library base name. Falls back to the Go module name. |
| `dart_entrypoint_class_name` | `FlutterGoBridge` | Generated Dart entrypoint class. |
| `dart_format` | `true` | Run `dart format` after generation. |
| `dart_format_line_length` | `100` | Formatter line length; minimum `40`. |
| `dart_preamble` / `go_preamble` | empty | Raw text inserted before generated Dart/Go code. |
| `stop_on_error` | `true` | Stop on the first unsupported exported declaration. |

`go_input` must resolve to exactly one package. Keep the generated bridge beside `go.mod`; an
input subpackage cannot write its bridge inside itself unless it is `package main`.

## Pubspec form

```yaml
flutter_go_bridge:
  go_input: go/api
  dart_output: lib/src/bridge_generated.dart
```

Use `--config-file path/to/file.yaml` to select a file explicitly.

