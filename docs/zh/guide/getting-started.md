# 快速开始

## 安装 CLI

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

该命令安装 `flutter_go_bridge_codegen`。

::: tip 从源码构建
模板通过 `go:embed` 内嵌，并包含 Gokit 子模块。构建前先执行
`git submodule update --init --recursive`。

从源码检出构建时，`make local` 会按当前系统和架构构建，并安装到 `GOBIN`；如果 `GOBIN` 为空，
则安装到 `GOPATH/bin`。
:::

## 创建或接入项目

新项目：

```sh
flutter_go_bridge_codegen create my_app
flutter_go_bridge_codegen create my_plugin -t plugin
```

已有项目（可在项目任意子目录执行）：

```sh
flutter_go_bridge_codegen integrate
flutter_go_bridge_codegen integrate -t plugin
```

## 编写 Go API

```go
package api

import "errors"

func Add(a, b int) int { return a + b }

//fgb:async
func Greet(name string) (string, error) {
    if name == "" {
        return "", errors.New("name is required")
    }
    return "hello, " + name, nil
}
```

## 生成并调用

```sh
flutter_go_bridge_codegen generate
# 开发时持续监听
flutter_go_bridge_codegen generate --watch
```

```dart
import 'src/bridge_generated.dart';
import 'src/api/api.dart';

void main() async {
  await FlutterGoBridge.initialize(libraryPath: 'path/to/library');
  final result = add(a: 20, b: 22);
  final greeting = await greet(name: 'world');
}
```

编译 Web 时，`FlutterGoBridge.initialize()` 自带默认的 Web 初始化流程：先调用
`WidgetsFlutterBinding.ensureInitialized()`，再调用内嵌在 `bridge_generated.web.dart` 中的
`FgbWasmLoader`，最后打开 bridge。如果项目需要自定义 loader，可以传入 `webInitializer` 覆盖默认流程。
Native 和纯 Dart 构建不会导入 Flutter widgets；纯 Dart Web 调用方应自行提供 `webInitializer`，不能使用
Flutter 资源 loader。生成器不再生成独立的 `fgb_wasm_loader*.dart` 文件。

继续阅读[配置](/zh/guide/configuration)、[输出结构](/zh/guide/output-structure)和
[类型映射](/zh/reference/type-mapping)。
