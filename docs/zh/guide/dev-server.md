# 开发服务器（run）

`run` 把代码生成、Flutter machine daemon 和文件监听组合到一起：

| 变更 | 动作 |
| --- | --- |
| Go 输入 | 重新生成、重新构建 native 库、重启整个应用进程 |
| Dart 输入 | hot reload |

hot reload/hot restart 只重建 Dart isolate，无法卸载已打开的动态库；Android 还需要重新把
`.so` 打进 APK 并安装，所以 Go 改动必须重启进程。

## 快捷键

| 键 | 行为 |
| --- | --- |
| `r` | hot reload |
| `R` | hot restart |
| `g` | 生成并重启 |
| `q` | 停止并退出 |
| `d` | detach |
| `h` | 显示帮助 |

默认轮询间隔为 400ms。生成文件会动态加入排除集，并用内容哈希避免“内容相同但 mtime 变化”
造成完整重构建。首次启动成功后，后续生成失败不会停止当前 app。

