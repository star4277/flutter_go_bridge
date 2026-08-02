# CLI

安装命令：

```sh
go install github.com/star4277/flutter_go_bridge/cmd/flutter_go_bridge_codegen@latest
```

## 查询版本

以下两种参数等价：

```sh
flutter_go_bridge_codegen -v
flutter_go_bridge_codegen --version
```

输出同时包含 flutter_go_bridge 版本和构建当前可执行文件的 Go 工具链版本：

```text
flutter_go_bridge_codegen version v1.2.3
Build with go1.25.0
```

排查源码兼容问题时应同时检查 Go 版本。若 codegen 可执行文件使用的 Go 工具链低于目标项目，可能
无法理解较新 Go 版本引入的语法或类型系统行为，应改用更新工具链构建的 codegen。

## `generate`

```sh
flutter_go_bridge_codegen generate
```

常用参数包括 `--config-file`、`--go-input`、`--go-output`、`--dart-output`、
`--library-name`、`--no-dart-format`、`--print-ast` 和 `--stop-on-error`。

### `generate --watch`

```sh
flutter_go_bridge_codegen generate --watch
```

默认约每 400ms 轮询本地 Go 输入，排除生成文件。生成失败会打印警告并继续监听。
watch 只接受本地目录或文件，不接受 package pattern。

## `run`

```sh
flutter_go_bridge_codegen run -d emulator-5554 -- --flavor dev
```

启动 `flutter run --machine`，Dart 改动 hot reload，Go 改动重新生成并重启进程。
`--` 后参数原样传给 Flutter；不支持 `-d all`。

## `create`

```sh
flutter_go_bridge_codegen create <name> [-t app|plugin]
```

先执行 `flutter create`，再应用与 `integrate` 相同的模板流程。支持 `--org`、
`--platforms`、`--go-mod-dir` 和 `--library-name`。
若某个 Flutter 版本没有生成可选脚手架目录，清理步骤会幂等跳过。普通平台目录中的嵌套内容会保留；
iOS/macOS 的 `Classes/` 脚手架目录则整体替换。

## `integrate`

```sh
flutter_go_bridge_codegen integrate [-t app|plugin]
```

向上寻找最近的 `pubspec.yaml` 并写入模板。已有文件默认跳过并告警；可用
`--no-write-lib`、`--no-integration-test`、`--no-dart-fix`、`--no-dart-format`
关闭可选步骤。
