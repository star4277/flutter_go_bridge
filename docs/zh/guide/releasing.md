# 发布版本

版本发布通过 GitHub Actions 手动触发。workflow 使用 Makefile 的发布矩阵构建
`flutter_go_bridge_codegen` CLI，并将各平台和架构的压缩包上传到 GitHub Release。

## 发起发布

进入 **Actions → Release → Run workflow**，填写：

- `version`：基础语义化版本，例如 `v1.2.3` 或 `1.2.3`；
- `pre_release`：是否发布下一个 beta 版本。

workflow 会自动补充缺失的 `v` 前缀。正式版本直接使用输入版本：

```text
输入：v1.2.3
标签：v1.2.3
```

如果选择 pre-release，workflow 会查询 GitHub 上已有的 Release 并递增 beta 序号：

```text
没有 v1.2.3-beta*  -> v1.2.3-beta1
已有 v1.2.3-beta1 -> v1.2.3-beta2
已有 v1.2.3-beta2 -> v1.2.3-beta3
```

workflow 使用 concurrency 串行化发布任务，避免两个手动任务同时计算出相同的 beta 序号。

## 替换已有 Release

最终 release tag 是替换依据。如果该 tag 已经存在 GitHub Release，workflow 会：

1. 删除旧 Release 及其 tag；
2. 使用当前源码重新构建压缩包；
3. 使用新产物和重新生成的 release notes 创建同名 Release。

如果只有 tag 没有 Release，也会先删除该 tag。也就是说，重新发布 `v1.2.3` 会有意替换旧的
`v1.2.3` 内容。

## 版本传递

workflow 会把最终版本写入统一环境变量：

```text
FLUTTER_GO_BRIDGE_VERSION
```

Makefile 和 CLI 构建都使用该变量：

```sh
FLUTTER_GO_BRIDGE_VERSION=v1.2.3 make linux-amd64
```

Makefile 使用它命名压缩包，并通过 Go `-ldflags` 注入 `main.version`。因此 `-v` 与 `--version`：

```sh
flutter_go_bridge_codegen -v
```

会同时输出构建时固化的发布版本和构建该可执行文件的 Go 工具链版本：

```text
flutter_go_bridge_codegen version v1.2.3
Build with go1.25.0
```

Release workflow 会在上传附件前验证这两行信息。

运行时如果显式设置 `FLUTTER_GO_BRIDGE_VERSION`，会覆盖二进制中的固化版本。如果环境变量和构建
参数都没有提供版本，CLI 默认使用：

```text
v0.0.1-snapshot
```

这个默认值仅为本地开发方便；Release workflow 总会传入明确版本。

## 构建边界

本 workflow 发布的是 `flutter_go_bridge_codegen` 命令。Makefile 中的 `ldflags` 不会配置应用 Go
cgo 动态库的 Gokit 构建；应用 native library 的构建参数仍由 Gokit 负责。

当前发布矩阵包含 Windows、Linux 和 macOS。压缩包名称包含命令名、架构、操作系统和版本，并作为
GitHub Release 附件上传。
