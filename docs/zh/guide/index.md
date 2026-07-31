# flutter_go_bridge 是什么？

`flutter_go_bridge` 是面向 Gokit 的 Go → Dart/Flutter 代码生成器。你编写普通 Go 代码，
执行一次生成命令，就能得到通过 FFI 调用 Go 动态库的 Dart API。

它不依赖 Flutter Native Assets，因此既能在 Flutter 中运行，也能在普通 Dart VM 中使用。

## 工作流程

```text
Go 源码 -> go/packages 类型分析 -> codec/ABI 代码生成 -> Dart API -> Gokit 跨平台构建
```

| 命令 | 用途 |
| --- | --- |
| [`generate`](/zh/guide/cli#generate) | 生成 Go bridge 与 Dart API |
| [`generate --watch`](/zh/guide/cli#generate-watch) | 监听 Go 代码并重新生成 |
| [`run`](/zh/guide/dev-server) | Dart 改动热重载，Go 改动重启应用 |
| [`create`](/zh/guide/cli#create) | 新建 Flutter + Go 工程 |
| [`integrate`](/zh/guide/cli#integrate) | 接入已有 Flutter 工程 |

核心决策包括[序列化策略](/zh/concepts/serialization)、[同步与异步](/zh/concepts/sync-async)
和[结构体与接口](/zh/reference/structs-interfaces)。

