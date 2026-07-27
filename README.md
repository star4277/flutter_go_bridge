# flutter-go-bridge-gokit

`flutter-go-bridge-gokit` 是面向 Gokit 的 Go → Dart/Flutter 代码生成器。它借鉴
`flutter_rust_bridge_codegen` 的 CLI 与生成结构，但不依赖 Flutter Native Assets，也不依赖
`package:flutter/services.dart`。

当前实现 `generate`、`generate --watch`、`create` 和 `integrate`。

## 设计约定

- 使用 Go 官方 `go/packages`、`go/ast`、`go/types` 解析源码和类型信息，不使用自定义 Go 语法解析器。
- 生成器内部使用可递归扩展的类型 IR 和 codec capability 判定，作用类似 FRB 的 `MirType`。
- 默认序列化方向与 FRB 一致：Dart → Go 使用 CST（C 结构体），Go → Dart 使用 DCO（`Dart_CObject`）。
- `map`、`any` 等当前无法安全表示为 CST/DCO 的调用，会整体回退到内置的纯 Dart Standard codec；不导入 Flutter SDK。
- Dart API DL 用于 `//fgb:async` 的 DCO 对象投递；同步 DCO 结果通过返回的 `Dart_CObject*` 解码。
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

未标记函数和 `//fgb:sync` 都只生成同步 Dart 方法；只有 `//fgb:async` 才生成异步 Dart 方法。
同一个 Go 方法不会同时生成同步、异步两个版本，Dart 方法名也不会添加 `Sync` 或 `Async` 后缀。

```go
func Add(a, b int) int { // 默认同步
	return a + b
}

//fgb:sync
func Subtract(a, b int) int {
	return a - b
}

//fgb:async
func LoadValue() int {
	return 42
}
```

生成的 Dart API：

```dart
final sum = add(a: 20, b: 22);
final difference = subtract(a: 22, b: 20);
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

### generate --watch

```text
flutter_go_bridge_codegen generate --watch
```

监听 `go_input`（目录递归，或单个 `.go` 文件）并在变更后自动重新生成，行为对应
`flutter_rust_bridge_codegen generate --watch`：

- 采用约 400ms 的轮询快照对比（无额外依赖，跨平台/网络盘/原子改名保存均可靠），
  `go_output`、`dart_output` 与点目录（如 `.git`）不会触发重跑；
- 每轮都重新加载配置文件，改配置在下一轮生效；
- 生成失败只打印告警并继续监听，Ctrl+C 退出；
- `--watch` 要求 `go_input` 是本地路径，包模式（package pattern）会直接报错。

## create

```text
flutter_go_bridge_codegen create my_app
flutter_go_bridge_codegen create my_plugin -t plugin
```

对应 `flutter_rust_bridge_codegen create`，从零建出一个可直接运行的 Flutter + Go 工程：

1. 执行 `flutter create`（app 用 `--template app`，plugin 用 `--template plugin_ffi`；
   `--org`、`--platforms` 透传）；
2. 删除会与模板冲突的脚手架文件（app 的 `lib/`、`test/`；plugin 的 `lib/`、`src/`、
   `ffigen.yaml`、各平台构建文件、`example/lib/` 等），因此入口文件是全新模板，
   不会带 integrate 那样的注释保留；
3. 在新工程上运行与 `integrate` 完全相同的注入流程（模板覆盖、pub 依赖、
   gokit build_tool、dart fix/format）。

目标目录已存在时会报错并提示改用 `integrate`。`--library-name` / `--go-mod-dir` /
`--platforms` 与 `integrate` 同义。

## integrate

在已有 Flutter 工程内（任意子目录均可，会向上查找 `pubspec.yaml`）执行：

```text
flutter_go_bridge_codegen integrate                 # app 模板
flutter_go_bridge_codegen integrate -t plugin       # FFI plugin 模板
```

它参照 `flutter_rust_bridge_codegen integrate` 的流程初始化项目：

- 覆盖模板：`flutter_go_bridge.yaml`、`go/`（Go 模块 + 示例 API + 预生成
  `bridge_generated.go`）、`lib/src/`（预生成 Dart bridge）、`test_driver/`，app 模板附加
  `go_builder/`（内含 gokit），plugin 模板附加平台构建文件和工程根的 `gokit/`。
- 已存在的文件一律跳过并告警；只有 `lib/main.dart`（app）或 `lib/<package>.dart`（plugin）
  会把原内容整体注释后写入模板，便于生成可运行的自包含 demo。
- app 模板执行 `flutter pub add <library_name> --path=go_builder`；按需为工程（及 plugin 的
  `example/`）添加 `integration_test`。
- 在 gokit `build_tool` 中执行 `flutter pub get`，把 gokit 目录加入 `analysis_options.yaml`
  的 analyzer exclude，最后运行 `dart fix --apply` 与 `dart format`。

库名默认 `go_lib_<pubspec name>`（plugin 为 `<pubspec name>`），Go 模块目录默认 `go`，可用
`--library-name` / `--go-mod-dir` 覆盖。`--platforms` 指定平台列表（缺省会通过
`flutter create --help` 探测 ohos 支持）；其余开关与 FRB 一致：`--no-write-lib`、
`--no-integration-test`、`--no-dart-fix`、`--no-dart-format`。

模板通过 `go:embed` 内嵌进 codegen 二进制；从源码构建前需要
`git submodule update --init --recursive` 拉取 gokit 子模块，否则 `integrate` 会在运行时报错。

## Dart 调用

生成代码只依赖 Dart SDK 自带库，可在纯 Dart VM 或 Flutter 中使用。所有生成的
Dart 入口（函数、方法、构造函数）一律使用命名参数；`bridge_generated.dart` 不再
re-export 各 API 文件，需要什么就 import 什么：

```dart
import 'bridge_generated.dart'; // FlutterGoBridge / FgbPlatformException / GoOpaque
import 'api.dart';
import 'account.dart';

void main() async {
  FlutterGoBridge.initialize(libraryPath: 'path/to/mylib.dll');

  final answer = add(a: 20, b: 22);    // 未标记：同步
  final account = await loadAccount(); // Go 侧标记 //fgb:async：异步
}
```

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
| `fgb_drop` | `NativeFinalizer` 自动释放 GoOpaque 句柄 |
| `fgb_dart_opaque_port` | 注册 DartOpaque 释放通知端口 |

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
| 可翻译结构体（默认） | 对应源文件中的 Dart class：带字段、命名构造参数；指针字段可空 |
| 普通 `*struct` 值 | 可空 Dart value class，字段继续参与 CST/DCO 序列化 |
| GoOpaque 结构体 | `extends GoOpaque` 的句柄类 + `NativeFinalizer` 自动释放 |
| `fgb.DartOpaque` / `*fgb.DartOpaque` | `Object` / `Object?`（Dart 对象按句柄穿透 Go） |
| `time.Time` | `DateTime` |
| `math/big.Int` | `BigInt` |
| 最后一个 `error` 返回值 | `FgbPlatformException` |
| `any` / `interface{}` | `Object?` |

结构体分类与 FRB 一致：字段全部可序列化的结构体默认按字段翻译（指针 receiver 方法照常生成，
但 receiver 按值序列化，Go 侧的修改不会写回 Dart 对象）；含不可序列化字段（func、chan、
非空接口、外部类型等）的结构体自动降级为 GoOpaque 并给出警告；`//fgb:opaque` 可强制句柄
语义——需要在 Go 侧保存内部状态、或私有字段承载状态的类型建议显式标记。GoOpaque 类型必须
以 `*T` 出现在签名中。小写开头的私有类型、字段、方法、函数、常量一律不参与生成。

`fgb.DartOpaque`（来自 `github.com/star4277/flutter-go-bridge-gokit/fgb` 运行时包）把任意
Dart 对象按句柄交给 Go 保存、之后原样传回；Go 侧最后一份拷贝被 GC 后会自动通知 Dart 释放。
只有 API 用到它时 go.mod 才需要这一个依赖。

当前支持零个或一个非 `error` 返回值；`error` 必须位于最后。泛型函数、可变参数、多非 error 返回值、
非空接口、函数类型（回调）和复杂外部命名类型暂未支持。

## 指令与字段 tag

声明上的注释指令采用 Go 指令语法 `//fgb:xxx`（`//` 后不加空格，gofmt 不会改写，
也不会混入文档注释）；多个指令可以分行写，或在一行内用逗号组合：

```go
//fgb:ignore                       // 跳过该函数/方法/类型/常量；被忽略类型的方法一并跳过
func InternalOnly() {}

//fgb:async, rename = "fetchValue" // 异步 + 重命名，可逗号组合
func LoadValue() Value { /* ... */ }

//fgb:opaque                       // 强制 GoOpaque 句柄语义
type Counter struct{ total int }
```

结构体字段的 `fgb` tag（逗号组合；`defaultValue` 会吞掉其后的所有内容，须放最后）：

```go
type Item struct {
	Name   string `fgb:"rename:title"`          // Dart 字段与 wire key 改名
	Count  int    `fgb:"non-final,defaultValue: 0"` // 非 final 字段 + 构造默认值
	Hidden string `fgb:"ignore"`                // 不参与生成
	Note   *string                              // 指针字段：可空、构造参数不加 required
}
```

## 验证

```text
go test ./...
go run ./cmd/flutter_go_bridge_codegen generate --config-file example/flutter_go_bridge.yaml --no-dart-format
(cd example/go && go build -buildvcs=false -buildmode=c-shared -o ../dart/example.dll .)
(cd example/dart && dart run bin/smoke.dart example.dll)
```
