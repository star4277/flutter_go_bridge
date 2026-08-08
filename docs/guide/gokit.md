# Building with Gokit

Gokit owns both Native cgo builds and Web Wasm builds. It uses the same configuration parsing,
source/toolchain fingerprint, artifact cache, file lock, atomic install, and structured build events
for both targets. Plain `generate` remains a code-generation command. The long-running
`flutter_go_bridge_codegen run -d chrome` wrapper invokes `build-web` automatically; a direct
`flutter run/build web` command requires the documented Gokit build step first.

The standalone codegen wrapper for that preparation step is:

```sh
flutter_go_bridge_codegen build-web
```

It regenerates the shared bridge, runs Gokit `build-web`, and stops before Flutter. This is the
recommended command before a direct `flutter run -d chrome` or `flutter build web`. The lower-level
Gokit invocation remains useful when code has already been generated or when integrating Gokit into
another build system.

For a one-shot application build, `flutter_go_bridge_codegen build web -- --release` runs shared
generation, Gokit `build-web`, and `flutter build web --release` in order. Native build targets use
the matching Native builder. Both paths expose a platform-specific signing boundary; its initial
implementations are no-ops so signing can be added later without changing the CLI.

## Platform outputs

| Platform | Output |
| --- | --- |
| Android / HarmonyOS | `lib<name>.so` |
| Windows | `<name>.dll` |
| Linux | `lib<name>.so` |
| iOS / macOS | `lib<name>.a`, linked into the Framework |
| Web | `go_lib_<name>.wasm`, `wasm_exec.js`, and `fgb_wasm_manifest.json` |

The generated bridge must be in the Go module root (`package main`), and `gokit.yaml` normally
uses:

```yaml
library_name: go_lib_example
main_package: .
```

The templates include platform glue under `android/`, `ios/`, `macos/`, `windows/`, `linux/`, and
`ohos/`. The detailed Gokit usage guide is embedded in the generated project at
`gokit/docs/usage_zh.md`.

`build-web` always sets `CGO_ENABLED=0 GOOS=js GOARCH=wasm`. The generated Web bridge uses
`syscall/js` and StandardMessageCodec-compatible bytes; it contains no C ABI, `import "C"`, or Dart
FFI. Before Dart invokes an API, the Web loader must run `wasm_exec.js`, instantiate the manifest's
Wasm artifact, and wait until the library appears in the global bridge registry.

Methods declared in a source file importing `"C"` remain visible in the generated Dart API but throw
`UnsupportedError` on Web with the recorded cgo reason. Pure-Go methods in other files of the same
package remain usable when the package still compiles with cgo disabled. Callbacks, streams,
`DartOpaque`, opaque handles/interfaces, and `dart:io` `InternetAddress` parameters are also explicit
Web fallbacks in the initial transport; Native support is unchanged.

Flutter does not execute an arbitrary package command during `flutter run` or `flutter build web`.
Therefore direct Flutter commands package the Wasm files already installed under the package's
`assets/wasm/` directory. Generate the bridge and run Gokit's `build-web` before invoking Flutter;
the exact app and plugin commands are shown in the [development server guide](dev-server.md).

## Requirements

Install Go and the platform C toolchain: MinGW-w64 on Windows, GCC/Clang on Linux, Xcode on
Apple platforms, Android SDK/NDK on Android, and an OHOS native SDK with `OHOS_SDK_HOME` for
HarmonyOS.

For source checkouts, initialize the embedded submodule before building the CLI:

```sh
git submodule update --init --recursive
```
