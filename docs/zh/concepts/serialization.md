# 序列化策略

每个调用会根据参数、receiver 和返回值递归选择 codec：

| 方向 | 首选 | 表示方式 |
| --- | --- | --- |
| Dart → Go | CST | 生成 C 兼容 wire struct，嵌套值由短生命周期 arena 管理 |
| Go → Dart | DCO | 生成 `Dart_CObject` 树，在 Dart 侧解码 |
| 双向回退 | Standard codec | 处理 `map`、`any`、接口等当前无法安全表示的类型 |

结构体按字段声明顺序编码。standard codec 是生成器内置的纯 Dart 实现，不是 Flutter 的
`StandardMessageCodec`，因此不依赖 Flutter SDK。

