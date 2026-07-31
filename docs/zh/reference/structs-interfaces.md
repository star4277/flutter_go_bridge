# 结构体与接口

导出字段都可翻译的结构体生成 Dart value class，指针字段变成可空字段。匿名嵌入结构体映射为
Dart 继承，被提升字段在 wire 上扁平化。

含不可支持字段、仅私有状态，或标记 `//fgb:opaque` 的结构体会变成 `GoOpaque` 句柄。
opaque 类型必须以 `*T` 出现在导出签名中，并由 `NativeFinalizer` 自动释放。

命名 Go 接口生成 `abstract interface class`，实现类型生成 `implements`。接口值通过 standard
codec 以 `[实现序号, 载荷]` tagged union 传输，并且至少需要一个已桥接的实现类型。

