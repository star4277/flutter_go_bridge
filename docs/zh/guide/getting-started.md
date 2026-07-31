# 快速开始

## 安装 CLI

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

该命令安装 `flutter_go_bridge_codegen`。

::: tip 从源码构建
模板通过 `go:embed` 内嵌，并包含 Gokit 子模块。构建前先执行
`git submodule update --init --recursive`。
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
  FlutterGoBridge.initialize(libraryPath: 'path/to/library');
  final result = add(a: 20, b: 22);
  final greeting = await greet(name: 'world');
}
```

继续阅读[配置](/zh/guide/configuration)、[输出结构](/zh/guide/output-structure)和
[类型映射](/zh/reference/type-mapping)。
