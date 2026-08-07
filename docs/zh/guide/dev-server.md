# 开发服务器（run）

`run` 把代码生成、Flutter machine daemon 和文件监听组合到一起：

| 变更 | 动作 |
| --- | --- |
| Go 输入 | 重新生成、重新构建当前平台产物、重启整个应用进程 |
| Dart 输入 | hot reload |

选择 Web 设备（`chrome`、`edge` 或 `web-server`）时，首次生成还会调用 Gokit 的
`build-web`。Go 发生变化后，`run` 会先重新生成并编译 Wasm，再重启 Flutter 进程。Native
设备不会调用 Web 构建器。

hot reload/hot restart 只重建 Dart isolate，无法卸载已打开的动态库；Android 还需要重新把
`.so` 打进 APK 并安装，所以 Go 改动必须重启进程。

## 快捷键

| 键 | 行为 |
| --- | --- |
| `r` | hot reload |
| `R` | hot restart |
| `g` | 生成并重启 |
| `q` | 停止并退出 |
| `d` | detach |
| `h` | 显示帮助 |

## 直接运行 Flutter

`flutter run -d chrome` 和 `flutter build web` 不会调用
`flutter_go_bridge_codegen`，也不会执行任意 Go 编译器。不使用 Flutter build hook 时，需要
先执行同一个 Gokit 构建步骤，确保包的 `assets/wasm/` 中包含最新的 `.wasm`、
`wasm_exec.js` 和 `fgb_wasm_manifest.json`：

```powershell
go run ./cmd/flutter_go_bridge_codegen generate --config-file flutter_go_bridge.yaml
dart run go_builder/gokit/build_tool/bin/build_tool.dart build-web `
  --manifest-dir "$PWD/go" `
  --output-dir "$PWD/go_builder/assets/wasm" `
  --root-project-dir "$PWD"
flutter run -d chrome
# 或：flutter build web
```

Plugin 项目改用 `gokit/build_tool/bin/build_tool.dart` 和 `assets/wasm/`。该命令跨平台一致。
生成的 Web bridge 通过包资源读取 manifest，Flutter 只负责打包已经存在的文件。

默认轮询间隔为 400ms。生成文件会动态加入排除集，并用内容哈希避免“内容相同但 mtime 变化”
造成完整重构建。首次启动成功后，后续生成失败不会停止当前 app。
