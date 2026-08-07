# 输出结构

生成器写入一个 Go 入口文件，并按 Go 模块目录镜像 Dart API：

```text
go/
├── go.mod
├── bridge_generated.go
├── bridge_generated_web.go
├── fgb_web_build.json       # Web 元数据，供 Gokit build-web 使用
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

- 一次 `generate` 同时生成两个平台实现：`bridge_generated.go` 包含 Native 的 `package main`、cgo
  导出、dispatcher 和 codec；`bridge_generated_web.go` 包含不含 C ABI 的纯 Go `syscall/js` registry、
  dispatcher 和 codec。
- `bridge_generated.dart` 只包含共享 API、模型和 standard codec，并通过条件导入选择
  `bridge_generated.io.dart` 或 `bridge_generated.web.dart`。
- `internal/fgb/fgb_generated.go`：`StreamSink`、`DartOpaque` 等签名支持类型。
- 每个 Go 源文件生成一个同名 Dart API 文件。

镜像锚点是 Go 模块根，例如 `go/api/api.go` → `lib/src/api/api.dart`。生成文件不要手改；
修改 Go API 或配置后重新运行 `generate`。

生成的 Dart 会使用符合 lint 的 lowerCamelCase 标识符、完整控制流代码块和字符串插值，
因此可在 Dart/Flutter 推荐分析规则下保持干净。app 项目的 `bridge_generated.dart` 不生成命名
`library` 声明；plugin 项目的 `library <plugin_name>;` 仍由插件公开入口文件持有，并由该入口
导出生成实现。
生成顺序在多次运行之间保持确定：源文件按规范化路径排序，每个文件内维持声明顺序，因此输入
不变时输出也不变。

Web 的 Flutter 资源 loader（`FgbWasmManifest` 和 `FgbWasmLoader`）直接位于
`bridge_generated.web.dart` 中，不再需要独立的 loader Dart 文件。Web 生成还会在 Go bridge 旁写入 `fgb_web_build.json`。Gokit 会把其中的协议、生成器、库名和 API
hash 写入 `fgb_wasm_manifest.json`；Web loader 在启动 `wasm_exec.js` 前会校验 manifest。manifest
的 target 是 `web-wasm`。Flutter asset bundle 通过逻辑 key
`packages/<plugin>/assets/wasm/...` 读取 manifest；浏览器加载 `wasm_exec.js` 和 `.wasm` 时则使用
打包后的 `assets/packages/<plugin>/assets/wasm/...` URL。
