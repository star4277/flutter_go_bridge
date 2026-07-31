# 配置

`generate` 和 `run` 会在当前项目目录按顺序查找：

```text
.flutter_go_bridge.yml/.yaml/.json
flutter_go_bridge.yml/.yaml/.json
pubspec.yaml 中的 flutter_go_bridge: 节
```

命令行参数覆盖配置文件。相对路径基于配置文件目录解析。

## 完整示例

```yaml
go_input: go/api
go_output: go/bridge_generated.go
dart_output: lib/src/bridge_generated.dart
library_name: go_lib_example
dart_entrypoint_class_name: FlutterGoBridge
dart_format: true
dart_format_line_length: 100
stop_on_error: true
```

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `base_dir` | 当前目录 | 相对路径基准目录 |
| `go_input` | 必填 | Go 包目录、单个 `.go` 文件或 package pattern |
| `go_output` | 最近 `go.mod` 同级的 `bridge_generated.go` | Go cgo bridge |
| `dart_output` | `lib/src/bridge_generated.dart` | Dart runtime/bridge |
| `library_name` | `go_lib_<pubspec name>` | 动态库基础名，无 pubspec 时回退 Go module |
| `dart_entrypoint_class_name` | `FlutterGoBridge` | Dart 入口类名 |
| `dart_format` | `true` | 生成后执行 Dart 格式化 |
| `dart_format_line_length` | `100` | 最小值 40 |
| `dart_preamble` / `go_preamble` | 空 | 插入生成文件头部的原始文本 |
| `stop_on_error` | `true` | 遇到首个不支持的导出声明时停止 |

`go_input` 必须只解析出一个包。可用 `--config-file` 显式指定配置文件。

