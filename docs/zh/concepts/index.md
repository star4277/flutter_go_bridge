# 设计约定

- 使用官方 `go/packages`、`go/ast`、`go/types` 解析 Go 代码。
- 每个调用递归选择 codec：Dart → Go 首选 CST，Go → Dart 首选 DCO，无法表达时回退纯 Dart
  standard codec。
- FFI、动态库加载、内存管理和 Dart API DL 集中在一个生成 runtime 文件中。
- 所有业务调用经固定 dispatcher ABI，不为每个函数增加 C 导出符号。
- Dart 输出按 Go 源文件和模块目录镜像，便于定位 API。

进一步阅读[序列化策略](/zh/concepts/serialization)、[稳定 ABI](/zh/concepts/stable-abi)
和[类型映射](/zh/reference/type-mapping)。

