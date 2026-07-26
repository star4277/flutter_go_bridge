# flutter-go-bridge-gokit

`flutter-go-bridge-gokit` 是面向 Gokit 的 Go → Dart/Flutter 代码生成器。它借鉴
`flutter_rust_bridge_codegen` 的 CLI 与生成结构，但不依赖 Flutter Native Assets，也不依赖
`package:flutter/services.dart`。

当前只实现 `generate`。`create`、`integrate` 和 `generate --watch` 已保留命令边界，但会明确提示尚未实现。

## 设计约定

- 使用 Go 官方 `go/packages`、`go/ast`、`go/types` 解析源码和类型信息，不使用自定义 Go 语法解析器。
- 生成器内部使用可递归扩展的类型 IR 和 codec capability 判定，作用类似 FRB 的 `MirType`。
- 默认序列化方向与 FRB 一致：Dart → Go 使用 CST（C 结构体），Go → Dart 使用 DCO（`Dart_CObject`）。
- `map`、`any` 等当前无法安全表示为 CST/DCO 的调用，会整体回退到内置的纯 Dart Standard codec；不导入 Flutter SDK。
- Dart API DL 用于 `fgb(async)` 的 DCO 对象投递；同步 DCO 结果通过返回的 `Dart_CObject*` 解码。
- 所有 `dart:ffi`、动态库加载、内存管理和 Dart API DL 代码都集中在 `bridge_generated.dart`。
- 每个 Go 源文件生成一个同名 Dart API 文件；目录结构按 Go 输入目录原样镜像。
- Go 端只生成一个 `bridge_generated.go`，默认放在最近的 `go.mod` 同级目录，不生成 `.c` 或 `.h` 文件。
- opaque Go 对象由 `NativeFinalizer` 自动释放；生成 API 不提供、也不要求开发者调用 `dispose()`。

## 序列化策略

每个调用会根据参数、receiver 和返回值递归选择 codec：

| 方向 | 首选 | 说明 |
| --- | --- | --- |
| Dart → Go | CST | 为参数和结构体生成真实 C wire struct；标量内联，字符串、集合和嵌套结构体由短生命周期 arena 管理 |
| Go → Dart | DCO | 使用 `Dart_CObject`；结构体按字段声明顺序编码为 Dart `List`，再在 Dart 侧还原为 API class |
| 双向 fallback | Standard codec | 用于 `map`、`any` 等 CST/DCO 尚不支持的类型，仍然是纯 Dart 实现 |

Go 值结构体的字段会生成在其对应源文件的 Dart class 中。普通字段为 `required` 构造参数；Go 指针字段映射为
Dart 可空类型，构造参数不加 `required`。所有 CST/DCO/FFI 细节仍只存在于 `bridge_generated.dart`。

## 同步与异步标记

未标记函数和 `fgb(sync)` 都只生成同步 Dart 方法；只有 `fgb(async)` 才生成异步 Dart 方法。
同一个 Go 方法不会同时生成同步、异步两个版本，Dart 方法名也不会添加 `Sync` 或 `Async` 后缀。

```go
func Add(a, b int) int { // 默认同步
	return a + b
}

// fgb(sync)
func Subtract(a, b int) int {
	return a - b
}

// fgb(async)
func LoadValue() int {
	return 42
}
```

生成的 Dart API：

```dart
final sum = add(20, 22);
final difference = subtract(22, 20);
final value = await loadValue();
```

## 输出结构

假设 Go 模块如下：

```text
go/
├── go.mod
└── api/
    ├── api.go
    └── account.go
```

推荐配置：

```yaml
go_input: go/api
go_output: go/bridge_generated.go
dart_output: lib/src/bridge_generated.dart
# library_name is optional; defaults to go_lib_<pubspec.yaml name>.
dart_entrypoint_class_name: FlutterGoBridge
dart_format: true
```

生成结果：

```text
go/
├── go.mod
├── bridge_generated.go       # package main、cgo ABI、codec、dispatcher
└── api/
    ├── api.go
    └── account.go

lib/src/
├── bridge_generated.dart     # 唯一的 FFI/runtime/codec 整合文件
├── api.dart                  # api.go 中的类型、函数和方法
└── account.dart              # account.go 中的类型、函数和方法
```

如果 `go_output` 省略，生成器会从本地 `go_input` 向上查找最近的 `go.mod`，并在其同级生成
`bridge_generated.go`。如果 `dart_output` 指向目录，整合文件名默认为 `bridge_generated.dart`。

## CLI

```text
go install github.com/star4277/flutter-go-bridge-gokit/cmd/flutter_go_bridge_codegen@latest
flutter_go_bridge_codegen generate
```

也可以直接传参：

```text
flutter_go_bridge_codegen generate \
  --go-input go/api \
  --go-output go/bridge_generated.go \
  --dart-output lib/src/bridge_generated.dart \
  --library-name go_lib_example
```

自动配置文件位置与 `flutter_rust_bridge_codegen` 类似：

- `.flutter_go_bridge.yml/.yaml/.json`
- `flutter_go_bridge.yml/.yaml/.json`
- `pubspec.yaml` 中的 `flutter_go_bridge:` 节

命令行参数覆盖配置文件。

## Dart 调用

生成代码只依赖 Dart SDK 自带库，可在纯 Dart VM 或 Flutter 中使用：

```dart
import 'api.dart';
import 'account.dart';

void main() async {
  FlutterGoBridge.initialize(libraryPath: 'path/to/mylib.dll');

  final answer = add(20, 22);          // 未标记：同步
  final account = await loadAccount(); // fgb(async)：异步
}
```

也可以只导入 `bridge_generated.dart`，它会统一 export 所有生成的 Go 源文件 API。

## 稳定 ABI

业务函数通过统一 dispatcher 分发，不会为每个方法生成独立 C 符号。公共符号为：

| 符号 | 作用 |
| --- | --- |
| `fgb_init` | 使用 `NativeApi.initializeApiDLData` 初始化 Dart API DL |
| `fgb_cst` | CST 参数 + DCO 返回值的同步首选入口 |
| `fgb_cst_async` | CST 参数；goroutine 完成后通过 `Dart_PostCObject` 投递 DCO 结果 |
| `fgb_dco_free` | 释放同步调用返回的 DCO 对象树 |
| `fgb` | Standard codec 同步 fallback 入口 |
| `fgb_async` | Standard codec 异步 fallback 入口 |
| `fgb_alloc` / `fgb_free` | FFI 请求与响应缓冲区管理 |
| `fgb_drop` | `NativeFinalizer` 自动释放 opaque Go 句柄 |

这些符号由 `bridge_generated.go` 中的 cgo 导出声明生成；代码生成器本身不会创建 C 源文件或头文件。

## Gokit

生成的 Go bridge 位于模块根目录，因此 `gokit.yaml` 的主包应指向模块根：

```yaml
library_name: go_lib_example
main_package: .
```

Windows/Linux 的 CMake 配置可继续使用 Gokit，并以 `fgb_init` 作为强制链接符号：

```cmake
include("../gokit/cmake/gokit.cmake")
apply_gokit(${PLUGIN_NAME} ../go mylib fgb_init)
```

## 类型映射

| Go | Dart/wire |
| --- | --- |
| `bool`, `string` | `bool`, `String` |
| `int8`…`int64`, `int` | `int` |
| `uint8`…`uint32` | `int`，生成范围检查 |
| `uint64`, `uint`, `uintptr` | `BigInt` |
| `float32`, `float64` | `double` |
| `[]byte` | `Uint8List` |
| `[]int32`, `[]int64`, `[]float64` | 对应 typed list |
| 其他 slice/array | `List<T>` |
| `map[K]V` | `Map<K, V>` |
| 值结构体 | 对应源文件中的 Dart `final class` |
| 普通 `*struct` 值 | 对应 Dart 可空 value class，字段继续参与 CST/DCO 序列化 |
| 具有指针 receiver 方法的 `*struct` | opaque Dart 类 + `NativeFinalizer` 自动释放 |
| `time.Time` | `DateTime` |
| `math/big.Int` | `BigInt` |
| 最后一个 `error` 返回值 | `FgbPlatformException` |
| `any` / `interface{}` | `Object?` |

当前支持零个或一个非 `error` 返回值；`error` 必须位于最后。泛型函数、可变参数、多非 error 返回值、
非空接口和复杂外部命名类型暂未支持。

## 其他指令

```go
// flutter_go_bridge:dart_name=fetchValue
func FetchValue() Value { /* ... */ }

// flutter_go_bridge:ignore
func InternalOnly() {}
```

## 验证

```text
go test ./...
go run ./cmd/flutter_go_bridge_codegen generate --config-file example/flutter_go_bridge.yaml --no-dart-format
(cd example/go && go build -buildvcs=false -buildmode=c-shared -o ../dart/example.dll .)
(cd example/dart && dart run bin/smoke.dart example.dll)
```
