# 文档站开发

文档站基于 VitePress，并统一使用 Bun。以下命令都在 `docs/` 目录执行：

```sh
bun install
bun run dev
```

提交文档前执行：

```sh
bun run typecheck
bun run build
```

文档改动合并到 `main` 后不会直接部署站点。生产文档由 Release workflow 在 GitHub Release 创建成功
后发布；如果部署失败，也可以通过文档 workflow 的 `workflow_dispatch` 手动重新执行。

构建会把死链视为错误。英文页面位于 `docs/` 根目录，中文页面在 `docs/zh/` 下保持对应结构。
新增页面时，需要同步更新 `.vitepress/locales/en.ts` 和 `zh.ts` 中的导航；条件允许时同时提供双语页面。

站点共享配置位于 `.vitepress/shared.ts`，文本与导航放在 locale 文件中。不要手动修改
`.vitepress/dist/` 和 `.vitepress/cache/`，它们都是生成目录。

文档中的命令和行为应以 CLI 源码、帮助信息和测试为准。仓库根目录常用校验命令：

```sh
go test ./...
go run ./cmd/flutter_go_bridge_codegen --help
go run ./cmd/flutter_go_bridge_codegen <subcommand> --help
```
