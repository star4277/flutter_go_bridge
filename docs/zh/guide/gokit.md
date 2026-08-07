# 用 Gokit 构建

Gokit 同时负责 Native cgo 和 Web Wasm 构建。两类目标共用配置解析、源码/工具链 fingerprint、
产物缓存、文件锁、原子安装和结构化构建事件。IDE 工具可在 Go 修改后调用 `build-web`；代码生成器
本身不负责 Go 编译。

| 平台 | 产物 |
| --- | --- |
| Android / HarmonyOS | `lib<name>.so` |
| Windows | `<name>.dll` |
| Linux | `lib<name>.so` |
| iOS / macOS | `lib<name>.a` 并链接进 Framework |
| Web | `go_lib_<name>.wasm`、`wasm_exec.js` 和 `fgb_wasm_manifest.json` |

生成的 bridge 位于 Go 模块根且是 `package main`，典型配置为：

```yaml
library_name: go_lib_example
main_package: .
```

需要 Go 和对应 C 工具链：Windows 使用 MinGW-w64，Linux 使用 GCC/Clang，Apple 平台使用
Xcode，Android 使用 SDK/NDK，HarmonyOS 设置 `OHOS_SDK_HOME`。

模板中的完整平台接入手册位于生成项目的 `gokit/docs/usage_zh.md`。从本仓库源码构建 CLI
前先执行 `git submodule update --init --recursive`。

`build-web` 固定设置 `CGO_ENABLED=0 GOOS=js GOARCH=wasm`。生成的 Web bridge 使用
`syscall/js` 和兼容 StandardMessageCodec 的字节消息，不含 C ABI、`import "C"` 或 Dart FFI。
Dart 调用 API 前，Web loader 必须执行 `wasm_exec.js`、实例化 manifest 指向的 Wasm 产物，并
等待库注册到全局 bridge registry。

包含 `import "C"` 的源文件中声明的方法仍保留在 Dart API 中，但在 Web 调用时会按记录的 cgo
原因抛出 `UnsupportedError`。同包其他纯 Go 文件只要能在禁用 cgo 后完整编译，其方法仍可使用。
初始 Web 传输层还会对 callback、stream、`DartOpaque`、opaque handle/interface 和使用
`dart:io` `InternetAddress` 的参数生成明确兜底；Native 支持不变。
