# 配置

`generate`、`run` 和 `build` 会在当前项目目录按顺序查找：

```text
.flutter_go_bridge.yml/.yaml/.json
flutter_go_bridge.yml/.yaml/.json
pubspec.yaml 中的 flutter_go_bridge: 节
```

命令行参数覆盖配置文件。相对路径基于配置文件目录解析。

## 完整示例

```yaml
target: native
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
| `target` | `native` | Go 分析和生成传输层的目标：`native` 或 `web` |
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

使用 `target: web` 时，包分析固定采用 `CGO_ENABLED=0 GOOS=js GOARCH=wasm`。Go 工具链会
排除任何包含 `import "C"` 的源文件，因此该文件中声明的所有导出 API 在 Web 不可用。这不表示
整个包一定失效：同包的其他纯 Go 文件只要仍能完整编译，其 API 仍可用于 Web。只有剩余 Web 文件
无法组成有效包，或引用了被排除文件中才存在的符号时，整个包才不可用。面向 Web 的包不应编写
cgo；生成器会记录具体不可用声明或包级失败原因。
