# Building with Gokit

Gokit compiles the generated Go cgo bridge during the normal Flutter build. Users run
`flutter run` or `flutter build`; they do not need to precompile a library manually.

## Platform outputs

| Platform | Output |
| --- | --- |
| Android / HarmonyOS | `lib<name>.so` |
| Windows | `<name>.dll` |
| Linux | `lib<name>.so` |
| iOS / macOS | `lib<name>.a`, linked into the Framework |

The generated bridge must be in the Go module root (`package main`), and `gokit.yaml` normally
uses:

```yaml
library_name: go_lib_example
main_package: .
```

The templates include platform glue under `android/`, `ios/`, `macos/`, `windows/`, `linux/`, and
`ohos/`. The detailed Gokit usage guide is embedded in the generated project at
`gokit/docs/usage_zh.md`.

## Requirements

Install Go and the platform C toolchain: MinGW-w64 on Windows, GCC/Clang on Linux, Xcode on
Apple platforms, Android SDK/NDK on Android, and an OHOS native SDK with `OHOS_SDK_HOME` for
HarmonyOS.

For source checkouts, initialize the embedded submodule before building the CLI:

```sh
git submodule update --init --recursive
```

