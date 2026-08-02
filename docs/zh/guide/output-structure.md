# 输出结构

生成器写入一个 Go 入口文件，并按 Go 模块目录镜像 Dart API：

```text
go/
├── go.mod
├── bridge_generated.go
├── internal/fgb/fgb_generated.go
└── api/
    ├── api.go
    └── account.go

lib/src/
├── bridge_generated.dart
└── api/
    ├── api.dart
    └── account.dart
```

- `bridge_generated.go`：`package main`、cgo 导出、dispatcher 和 codec。
- `internal/fgb/fgb_generated.go`：`StreamSink`、`DartOpaque` 等签名支持类型。
- `bridge_generated.dart`：动态库加载、FFI、内存、Dart API DL 和 codec runtime。
- 每个 Go 源文件生成一个同名 Dart API 文件。

镜像锚点是 Go 模块根，例如 `go/api/api.go` → `lib/src/api/api.dart`。生成文件不要手改；
修改 Go API 或配置后重新运行 `generate`。

生成的 Dart 会使用符合 lint 的 lowerCamelCase 标识符、完整控制流代码块和字符串插值，
因此可在 Dart/Flutter 推荐分析规则下保持干净。app 项目的 `bridge_generated.dart` 不生成命名
`library` 声明；plugin 项目的 `library <plugin_name>;` 仍由插件公开入口文件持有，并由该入口
导出生成实现。
生成顺序在多次运行之间保持确定：源文件按规范化路径排序，每个文件内维持声明顺序，因此输入
不变时输出也不变。
