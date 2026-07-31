# 用 Gokit 构建

Gokit 把 Go/CGO 编译接入 Flutter 的正常构建流程。使用者只需执行 `flutter run` 或
`flutter build`，无需手动预编译动态库。

| 平台 | 产物 |
| --- | --- |
| Android / HarmonyOS | `lib<name>.so` |
| Windows | `<name>.dll` |
| Linux | `lib<name>.so` |
| iOS / macOS | `lib<name>.a` 并链接进 Framework |

生成的 bridge 位于 Go 模块根且是 `package main`，典型配置为：

```yaml
library_name: go_lib_example
main_package: .
```

需要 Go 和对应 C 工具链：Windows 使用 MinGW-w64，Linux 使用 GCC/Clang，Apple 平台使用
Xcode，Android 使用 SDK/NDK，HarmonyOS 设置 `OHOS_SDK_HOME`。

模板中的完整平台接入手册位于生成项目的 `gokit/docs/usage_zh.md`。从本仓库源码构建 CLI
前先执行 `git submodule update --init --recursive`。

