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

