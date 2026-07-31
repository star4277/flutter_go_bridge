# flutter_go_bridge

<p align="center">
  <strong>连接 Go 与 Dart 的代码生成桥梁。</strong>
</p>

<p align="center">
  将 Go 服务能力转换为类型安全的 Dart/Flutter API，通过自动生成绑定、稳定 FFI ABI 和一致的类型映射，降低跨语言开发成本。
</p>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://star4277.github.io/flutter_go_bridge/zh/"><img alt="中文文档" src="https://img.shields.io/badge/文档-中文-4969ed"></a>
  <a href="./LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/License-MIT-green"></a>
  <a href="https://github.com/star4277/flutter_go_bridge/stargazers"><img alt="GitHub stars" src="https://img.shields.io/github/stars/star4277/flutter_go_bridge?style=flat"></a>
  <a href="https://github.com/star4277/flutter_go_bridge/actions"><img alt="GitHub Actions" src="https://img.shields.io/github/actions/workflow/status/star4277/flutter_go_bridge/docs.yml?branch=main&label=docs"></a>
</p>

## 文档

完整中文文档：

**https://star4277.github.io/flutter_go_bridge/zh/**

文档详细介绍安装、配置、生成目录、序列化、类型映射、指令、结构体、接口、Stream、Dart
闭包回调、返回值和 error 处理。

## 它做什么

你只需要编写普通 Go 代码，`flutter_go_bridge_codegen` 负责生成两端桥梁：

```text
Go package
    │
    │ go/packages + go/types
    ▼
flutter_go_bridge_codegen
    ├── bridge_generated.go
    └── 镜像的 Dart API 目录树
            └── 所有 FFI 细节集中在 bridge_generated.dart
```

```go
package api

import "errors"

type User struct {
	ID   int64
	Name string
}

func Add(a, b int) int {
	return a + b
}

//fgb:async
func LoadUser(id int64) (User, error) {
	if id <= 0 {
		return User{}, errors.New("id must be positive")
	}
	return User{ID: id, Name: "Gopher"}, nil
}
```

生成的 Dart API 使用命名参数和正常的 Dart 类型：

```dart
final total = add(a: 20, b: 22);
final user = await loadUser(id: 1);
print(user.name);
```

没有标记的函数默认同步。只有希望 Dart API 返回 `Future` 时，才需要添加 `//fgb:async`。

## 为什么选择 flutter_go_bridge

- **Go 代码保持原样**：通过官方 `go/packages`、`go/ast` 和 `go/types` 读取源码，不需要 IDL，
  也不需要自定义 Go 语法。
- **生成符合 Dart 使用习惯的 API**：每个 Go 源文件对应一个 Dart 文件，函数、方法和构造器全部
  使用命名参数，结构体生成强类型 Dart class。
- **FFI 细节完全隔离**：动态库加载、内存管理、codec、handle 和 Dart API DL 都只存在于
  `bridge_generated.dart`。
- **生成代码不依赖 Flutter SDK**：不使用 Flutter Native Assets，也不导入
  `package:flutter/services.dart`，生成绑定只依赖 Dart SDK 库。
- **逐调用选择 codec**：能使用 CST/DCO 的调用走高效路径；map、动态值和接口等类型按需回退到
  内置 standard codec。
- **稳定的 native ABI**：新增 Go 函数不会增加新的 C 导出符号，Gokit 和 CMake 配置不需要随 API
  数量变化。
- **支持有状态 Go 对象**：可序列化结构体生成 Dart value class；有状态或不能序列化的结构体生成
  `GoOpaque` handle，并由 `NativeFinalizer` 自动释放。
- **支持 Stream 与闭包回调**：Go producer 可以生成 Dart `Stream<T>`，Dart 同步或 async 闭包也可以
  直接传入 Go 函数。

## 安装

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

安装后的命令名是 `flutter_go_bridge_codegen`。

如果从源码构建 codegen，需要先初始化 Gokit 子模块：

```sh
git submodule update --init --recursive
```

## 快速开始

### 创建新项目

```sh
flutter_go_bridge_codegen create my_app
```

创建 FFI plugin：

```sh
flutter_go_bridge_codegen create my_plugin -t plugin
```

`create` 会执行 `flutter create`，注入 Go/Gokit 模板，并产出一个可以直接运行的工程。

### 接入已有项目

在 Flutter 工程的任意子目录运行：

```sh
flutter_go_bridge_codegen integrate
```

已有 FFI plugin 使用：

```sh
flutter_go_bridge_codegen integrate -t plugin
```

命令会向上查找最近的 `pubspec.yaml`，添加 Go 模块和 Gokit 构建文件，生成初始 bridge，并尽量
保留工程内已有文件。

### 生成绑定

```sh
flutter_go_bridge_codegen generate
```

开发时自动重新生成：

```sh
flutter_go_bridge_codegen generate --watch
```

也可以让 CLI 同时管理 Flutter 运行流程：

```sh
flutter_go_bridge_codegen run -d emulator-5554
```

Dart 代码变化使用 hot reload；Go 代码变化会重新生成并重启应用进程，让新的动态库真正被加载。

## 配置

CLI 会自动查找：

- `.flutter_go_bridge.yml`、`.flutter_go_bridge.yaml` 或 `.flutter_go_bridge.json`；
- `flutter_go_bridge.yml`、`flutter_go_bridge.yaml` 或 `flutter_go_bridge.json`；
- `pubspec.yaml` 中的 `flutter_go_bridge` 配置段。

最小配置示例：

```yaml
go_input: go/api
go_output: go/bridge_generated.go
dart_output: lib/src/bridge_generated.dart
dart_entrypoint_class_name: FlutterGoBridge
dart_format: true
```

`library_name` 可以省略，默认值是 `go_lib_<pubspec package name>`。命令行参数的优先级高于配置
文件。

所有配置项见[配置文档](https://star4277.github.io/flutter_go_bridge/zh/guide/configuration)。

## 生成结果

假设 Go 模块结构为：

```text
go/
├── go.mod
└── api/
    ├── api.go
    └── account.go
```

生成器会产出一个 Go bridge 和一棵镜像的 Dart 目录树：

```text
go/
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

- `bridge_generated.go` 包含 cgo 导出、dispatcher 和 Go codec。
- `bridge_generated.dart` 包含 FFI runtime、动态库绑定、codec 和 handle 管理。
- 镜像 Dart 文件只放公开的 class、函数、方法、接口和常量。

## 序列化模型

生成器会递归检查每个调用的参数、receiver 和返回值，然后选择传输方式：

| 方向 | 首选路径 | 用途 |
| --- | --- | --- |
| Dart → Go | CST | 使用真实 C wire struct；标量内联，嵌套值由短生命周期 arena 管理 |
| Go → Dart | DCO | 使用 `Dart_CObject` 直接返回或投递到 Dart |
| 双向 | Standard codec | map、`any`、命名接口和其他动态结构的 fallback |

codec 由生成器按调用确定。业务代码只使用公开 Dart API，不需要手动选择 codec。

## 支持的 API 类型

| Go | 生成的 Dart 类型 |
| --- | --- |
| `bool`、`string` | `bool`、`String` |
| `int8` 到 `int64`、`int` | `int` |
| `uint8`、`uint16`、`uint32` | 带范围检查的 `int` |
| `uint64`、`uint`、`uintptr` | `BigInt` |
| `float32`、`float64` | `double` |
| `[]byte`、`[]int32`、`[]int64`、`[]float64` | Dart typed list |
| `[]T`、`[N]T`、`map[K]V` | `List<T>`、`List<T>`、`Map<K, V>` |
| `time.Time`、`time.Duration`、`math/big.Int` | `DateTime`、`Duration`、`BigInt` |
| `net/netip.Addr`、`net/netip.Prefix`、`net/url.URL` | `InternetAddress`、`String`、`Uri` |
| `github.com/gofrs/uuid/v5.UUID` | `UuidValue` |
| `type XXX struct { ... }` | `class XXX` 或 `class XXX extends GoOpaque` |
| `type XXX interface { ... }` | `abstract interface class XXX` |
| `error` | `FgbPlatformException` |
| `chan<- T`、`fgb.StreamSink[T]` | `Stream<T>` 或 `StreamSink<T>` |
| `func(A) R` 参数 | `FutureOr<R> Function(A)` |

指针、nullable、集合、接口和不支持类型的完整规则见
[类型映射](https://star4277.github.io/flutter_go_bridge/zh/reference/type-mapping)。

## 核心能力

### 结构体与接口

可序列化的 Go 结构体会生成 Dart value class。匿名嵌入结构体映射为 Dart 继承，被提升字段在 wire
上扁平化。命名 Go 接口会生成 `abstract interface class`，并绑定生成器已发现的 Go 实现集合。

不能按字段序列化或需要保持 Go 端状态的类型可以显式声明：

```go
//fgb:opaque
type Counter struct {
	total int
}
```

它会生成 `GoOpaque` handle class，并在多次调用之间保留同一个 Go 对象身份。

### Stream

只需一个只写 channel，就能生成 Go 持有的 Dart Stream：

```go
//fgb:async
func Count(out chan<- int) {
	for value := range 5 {
		out <- value
	}
}
```

```dart
await for (final value in count()) {
  print(value);
}
```

需要 Go 主动发送 error event 或关闭 Stream 时，可以使用 `fgb.StreamSink[T]`。

### Dart 闭包回调

```go
//fgb:async
func Transform(input string, mapper func(string) string) string {
	return mapper(input)
}
```

```dart
final value = await transform(
  input: 'go',
  mapper: (text) => text.toUpperCase(),
);
```

生成的 callback 类型使用 `FutureOr`，因此 Dart 可以传同步闭包，也可以传 async 闭包。

### 返回值与 error

- 一个非 error 返回值保持普通 Dart 类型。
- 多个非 error 返回值生成 Dart record。
- Go 命名返回值生成命名 record 字段。
- `error` 可以位于 Go 返回列表中的任意位置。
- 非 nil error 抛出 `FgbPlatformException`；声明多个 error 时可以从
  `FgbPlatformException.goErrors` 读取逐条消息。

## CLI 一览

| 命令 | 作用 |
| --- | --- |
| `generate` | 生成 Go bridge 和镜像 Dart API |
| `generate --watch` | 监听 Go 源码并自动重新生成 |
| `run` | 运行 Flutter；Dart 热重载，Go 变化后重启进程 |
| `create` | 创建带 Go 集成的新 Flutter app 或 FFI plugin |
| `integrate` | 将 bridge 接入已有 Flutter 工程 |

完整参数见 [CLI 文档](https://star4277.github.io/flutter_go_bridge/zh/guide/cli)。

## 开发

克隆仓库并初始化子模块：

```sh
git clone --recurse-submodules https://github.com/star4277/flutter_go_bridge.git
cd flutter_go_bridge
go test ./...
```

本地构建 CLI：

```sh
go build ./cmd/flutter_go_bridge_codegen
```

使用 Makefile 构建发布产物：

```sh
make windows-amd64
make linux-amd64
make macos-arm64
```

文档站使用 Bun：

```sh
cd docs
bun install
bun run typecheck
bun run build
```

## License

`flutter_go_bridge` 使用 [MIT License](./LICENSE)。

生成 codec 还引用或遵循部分第三方组件的许可，详见
[THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。

## Star 历史

<a href="https://www.star-history.com/?repos=StarHistory%2FStarHistory&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=StarHistory/StarHistory&type=date&theme=dark&legend=top-left&sealed_token=8-XdVIGYbLUMT2OW1UD3vhaPTXAYI-xmNygNomKQG0TEe55jBQ4b58v8TnuAk5oOXjmoOFoXTY4VF4eOxv_sjz6VFm-1bOxcifQHdNsO1wVh4Ev7j4rAxy8ilGaiQN28FTirR1onjT4e8JTAXIl8j9utInP5dfwDn_ZveiEIF1_Nav_TcbyafatZszB7" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=StarHistory/StarHistory&type=date&legend=top-left&sealed_token=8-XdVIGYbLUMT2OW1UD3vhaPTXAYI-xmNygNomKQG0TEe55jBQ4b58v8TnuAk5oOXjmoOFoXTY4VF4eOxv_sjz6VFm-1bOxcifQHdNsO1wVh4Ev7j4rAxy8ilGaiQN28FTirR1onjT4e8JTAXIl8j9utInP5dfwDn_ZveiEIF1_Nav_TcbyafatZszB7" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=StarHistory/StarHistory&type=date&legend=top-left&sealed_token=8-XdVIGYbLUMT2OW1UD3vhaPTXAYI-xmNygNomKQG0TEe55jBQ4b58v8TnuAk5oOXjmoOFoXTY4VF4eOxv_sjz6VFm-1bOxcifQHdNsO1wVh4Ev7j4rAxy8ilGaiQN28FTirR1onjT4e8JTAXIl8j9utInP5dfwDn_ZveiEIF1_Nav_TcbyafatZszB7" />
 </picture>
</a>
