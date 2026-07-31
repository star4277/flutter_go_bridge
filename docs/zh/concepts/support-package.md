# 生成的支持包

当 API 使用 `fgb.DartOpaque` 或 `fgb.StreamSink` 时，生成器会在 Go 模块中写入：

```text
internal/fgb/fgb_generated.go
```

使用模块路径导入：

```go
import "example.com/my_app/internal/fgb"
```

支持包必须留在当前 Go 模块内，供 bridge 和 API 共同引用。它属于生成产物，不要手动修改。

