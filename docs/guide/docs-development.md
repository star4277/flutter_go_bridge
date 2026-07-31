# Contributing to the documentation

The site uses VitePress and Bun. Run all commands from `docs/`.

```sh
bun install
bun run dev
```

Before committing documentation changes, run:

```sh
bun run typecheck
bun run build
```

The build treats dead links as errors. English pages live at the documentation root and Chinese
pages mirror them under `zh/`. When adding a page, update both locale sidebar files under
`.vitepress/locales/` and provide the translated counterpart where practical.

Shared site settings are in `.vitepress/shared.ts`; prose and navigation labels belong in locale
files. Do not edit `.vitepress/dist/` or `.vitepress/cache/` because they are generated.

Keep commands and behavior grounded in the CLI source and tests. Useful project checks from the
repository root are:

```sh
go test ./...
go run ./cmd/flutter_go_bridge_codegen --help
go run ./cmd/flutter_go_bridge_codegen <subcommand> --help
```
