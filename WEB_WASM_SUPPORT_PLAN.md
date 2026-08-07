# Flutter Web Go Wasm 支持方案

状态：已实现，代码位于 `feature/web-wasm-support`。

本方案在不创建第二个 Flutter plugin 的前提下，让现有 `go_builder` 同时承载 Native
CGO/FFI 和 Web Go Wasm。Gokit 是唯一的 Go 构建层；生成器只生成桥接代码，`run` 命令在
Web 运行时调用 Gokit 编译 Wasm。

## 1. 最终契约

- 一次 `flutter_go_bridge_codegen generate` 同时生成 Native 和 Web Go bridge。
- Dart 公共 API、模型和 codec 只生成一份；只有传输层分为
  `bridge_generated.io.dart` 和 `bridge_generated.web.dart`。
- 仍然只有一个 `go_builder` plugin。它在 Native 平台声明 FFI，在 Web 平台声明资源。
- Gokit 统一 Native/Web 的配置解析、工具链 fingerprint、缓存、hash、manifest、文件锁和日志。
- 不使用 Flutter build hook。普通 `flutter run -d chrome` / `flutter build web` 只打包已经存在
  的 Wasm 资源；文档提供手动的 Gokit 命令。
- `flutter_go_bridge_codegen run -d chrome` 会生成代码、调用 Gokit `build-web`、启动 Flutter，
  并在 Go 变化后重复生成、编译和重启。Windows、Android 等 Native target 不调用 Web builder。
- `flutter_go_bridge_codegen build <platform>` 会生成一次代码，再通过 Native/Web 平台 builder
  构建 Flutter 产物；Web builder 先调用 Gokit `build-web`。两套实现都保留平台签名接口。

## 2. `import "C"` 的准确规则

Web 构建使用 `CGO_ENABLED=0 GOOS=js GOARCH=wasm`。Go 是按源文件处理 CGO，而不是看到一个
`import "C"` 就无条件禁用整个包：

| 源码情况 | Web 结果 |
| --- | --- |
| 一个 `.go` 文件包含 `import "C"` | 该文件整体从 Web 构建排除 |
| 同包还有不含 `import "C"` 的纯 Go 文件 | 这些文件仍可参与构建 |
| 纯 Go 文件引用被排除文件中的符号 | 包编译失败 |
| 包的有效实现全部位于 CGO 文件 | 包没有 Web 实现，编译失败 |
| 依赖包使用 CGO 且提供 `js/wasm` 替代文件 | 依赖可能正常工作 |
| 依赖包使用 CGO 且没有 Web 替代实现 | 当前包也会编译失败 |

因此，一个可导出到 Dart 的方法只要声明在 `import "C"` 文件中，即使参数和返回值全是
`int`、`string` 等纯 Go 类型，也不能在 Web 使用。生成器在 Web 分析中会给出逐方法 warning：
Native bridge 仍完整生成，Web Go dispatcher 跳过该方法，Dart 公共 API 保留同名成员并在调用时
抛出 `UnsupportedError`。

用户应把 CGO 限制在 Native 专用文件或 Native 专用依赖中，并为 Web 提供带
`//go:build js && wasm` 的纯 Go 替代实现。仅扫描 `import "C"` 不足以证明依赖可用，生成器还会
用 Web 构建环境加载包；真正的 Gokit `build-web` 仍是最终编译检查。

## 3. 生成文件

```text
go/
├── go.mod
├── bridge_generated.go          # Native，//go:build !(js && wasm)，cgo/FFI ABI
├── bridge_generated_web.go      # Web，//go:build js && wasm，纯 Go + syscall/js
├── fgb_web_build.json           # Web bridge metadata
└── api/
    └── *.go

lib/src/
├── bridge_generated.dart        # 共享 API、模型、standard codec、条件导出
├── bridge_generated.io.dart     # dart:ffi Native wire
├── bridge_generated.web.dart    # dart:js_interop Web wire + Flutter Wasm loader
└── api/
    └── *.dart                   # Native/Web 共用的 Dart API

go_builder/assets/wasm/
├── go_lib_<project>.wasm
├── wasm_exec.js
└── fgb_wasm_manifest.json
```

不再生成 `fgb_wasm_loader.dart`、`fgb_wasm_loader_stub.dart` 或
`fgb_wasm_loader_web.dart`。旧项目中已经存在的文件不会被 `generate` 主动删除，但新模板和新
生成结果不再引用它们；可以在项目迁移时手动删除旧文件。

## 4. Dart 平台选择与初始化

`bridge_generated.dart` 通过 `dart.library.js_interop` 条件导入 `.io.dart` 或 `.web.dart`。
业务代码只导入共享入口和 `api/**`，不写平台判断，也不重新生成两套 API。

生成 Flutter 项目时，`.web.dart` 内嵌 `FgbWasmManifest` 和 `FgbWasmLoader`：

1. `FlutterGoBridge.initialize()` 调用 `WidgetsFlutterBinding.ensureInitialized()`。
2. loader 从 `AssetBundle` 读取 `packages/<plugin>/assets/wasm/fgb_wasm_manifest.json`。
3. 校验 manifest 的 `target`、schema 和 Wasm artifact，加载匹配 Go SDK 的 `wasm_exec.js`。
4. 通过 `dart:js_interop` 实例化 Wasm、调用 `Go.run`，等待 Go bridge 注册 JS 入口。
5. 初始化完成后才打开 bridge；重复初始化复用同一个 Future。

`webInitializer` 仍然是可选覆盖回调。纯 Dart 包不导入 Flutter widgets；它没有 Flutter 资源
loader 的默认实现，必须传入自定义 `webInitializer`，否则初始化时抛出明确的
`UnsupportedError`。Native 初始化保持同一异步入口，但不引入 Flutter。

## 5. Native 与 Web 的传输差异

- Native 保留现有 cgo exports、`dart:ffi`、CST、DCO 和 standard codec。
- Web Go bridge 不包含 `import "C"`、C preamble、`//export`、C ABI struct、Dart API DL、
  `Dart_PostCObject` 或 CST/DCO 的 C 映射。
- Web 使用标准 codec，把 Dart 请求编码为字节，经 JS `Uint8Array` 传给 Go Wasm，再把响应字节
  解码回 Dart。共享 Dart codec 和公共 API 仍只生成一份。
- Web 初始实现明确拒绝 callback、stream、`DartOpaque`、opaque handle/interface、
  `dart:io` `InternetAddress` 以及 CGO-only 方法。Dart API 保留成员以维持跨平台签名一致，
  调用时返回同步异常、失败 Future 或带错误的 Stream。

浏览器的网络、TCP/UDP、TUN、原始 socket、本地文件系统等能力仍受浏览器沙箱限制；Go 能够
编译成 Wasm 不代表这些系统能力在浏览器中存在。

## 6. Gokit 构建流程

Gokit `build-web` 固定使用：

```text
CGO_ENABLED=0 GOOS=js GOARCH=wasm
```

并使用普通 `go build`，不使用 c-shared/C ABI。它生成 Wasm、`wasm_exec.js` 和 manifest，
通过共享 fingerprint/cache 复用产物。Native 仍由同一套 Gokit 配置和缓存抽象处理自己的
`c-shared`/平台构建。

### 6.1 `codegen build`

```powershell
flutter_go_bridge_codegen build web -- --release
flutter_go_bridge_codegen build windows -- --release
```

`build` 是一次性同步流程：先生成共享 Native/Web bridge，再选择平台 builder。Web 依次执行
Gokit `build-web` 和 `flutter build web`；Native 直接执行对应的 `flutter build <platform>`。
两者都会进入平台签名接口，当前 Native/Web signer 是空实现，后续签名对接不改变命令契约。

### 6.2 `codegen run`（开发监听）

```powershell
flutter_go_bridge_codegen run -d chrome -- --web-renderer canvaskit
```

Web 设备识别支持 `chrome`、`edge`、`web-server`、`web-javascript` 和 `web-wasm`，以及转发的
`-d/--device-id`。Native device id 不会触发 Wasm 构建。

### 6.3 直接使用 Flutter（无 IDE、无 hook）

App 项目：

```powershell
go run ./cmd/flutter_go_bridge_codegen generate --config-file flutter_go_bridge.yaml
dart run go_builder/gokit/build_tool/bin/build_tool.dart build-web `
  --manifest-dir "$PWD/go" `
  --output-dir "$PWD/go_builder/assets/wasm" `
  --root-project-dir "$PWD"
flutter run -d chrome
# 或：flutter build web
```

Plugin 项目把路径换成 `gokit/build_tool/bin/build_tool.dart`、`<project>/go` 和
`assets/wasm/`。Flutter 不会替用户执行上述 Go 命令，所以直接运行 Flutter 前必须先完成
`build-web`；Flutter 只负责把已有资源打包进 Web 应用。

## 7. 迁移与限制

1. 重新运行一次 `generate`，让项目获得共享 Dart API 和 `.io/.web` wire。
2. 确认 pubspec 的 plugin 名称与生成的 Wasm asset package 名称一致。
3. 删除旧 loader 文件（它们不再被新生成代码引用）。
4. 检查 Web warning；把 CGO 方法拆到 Native 文件，或提供 js/wasm 替代实现。
5. 运行 Gokit `build-web`，再执行 `flutter run -d chrome` 或 `flutter build web`。

当前实现不承诺把 Native-only callback、stream、opaque/interface 自动变成 Web 实现；这些能力
需要后续单独设计 Web 协议，不能通过把 FFI/CST/DCO 直接搬到 Wasm 解决。

## 8. 验证结果

- `go vet ./...`
- `go test ./... -count=1`
- 聚合 Go 覆盖率（排除 `cmd/**`、`template/**`）：95.2%
- 变更 Go 文件 `gopls check`
- 生成项目 `go build -buildmode=c-shared`
- 生成项目 `CGO_ENABLED=0 GOOS=js GOARCH=wasm go build`
- 生成 Flutter 项目 `dart analyze lib`、`flutter analyze`
- 文档 `bun run typecheck`、`bun run build`
- Chrome `flutter run -d chrome`：Wasm 资源加载和页面启动已验证。
- Windows/Android Native 运行链路已在本分支此前验证；Native run 不调用 Web builder。

## 9. 关键决策

- 不新增 plugin；Web 资源和 loader 归属现有 `go_builder`。
- 不使用 build hook；手动 Flutter 流程和 `codegen run` 都调用同一个 Gokit `build-web`。
- 增加统一的 `build <platform>` 命令作为一次性生成/构建入口；具体 Go artifact 仍由 Gokit
  负责，平台 builder 和 signer 接口负责后续构建与签名扩展。
- 不生成重复的 Native/Web Dart API 树；只拆平台 wire。
- 不把 `import "C"` 解释为“整个包必然不可用”，而是按文件、可达符号和依赖的实际 Web
  编译结果判断，并为每个不可用 Dart callable 提供运行时兜底。
