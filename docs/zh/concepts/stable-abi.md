# 稳定 ABI

业务函数通过索引和统一 dispatcher 调用，不导出独立 C 符号。固定符号包括：

`fgb_init`、`fgb_cst`、`fgb_cst_async`、`fgb_dco_free`、`fgb`、`fgb_async`、
`fgb_alloc`、`fgb_free`、`fgb_drop`、`fgb_dart_opaque_port`、`fgb_callback_port`、
`fgb_callback_result`、`fgb_stream_port`、`fgb_stream_cancel`。

因此新增或重命名 Go 函数只会改变 dispatcher 表，不需要修改 CMake、Gradle 或 podspec。
cgo 导出声明都位于 `bridge_generated.go`，不会生成额外 C/H 文件。

