# Contributing to the documentation

The site uses VitePress and Bun. Run all commands from `docs/`.

```sh
bun install
bun run dev
```

The same workflows are available from the repository root with Make. These targets run `bun install`
first:

```sh
make docs dev
make docs build
make docs preview
```

`make docs preview` runs the VitePress build before starting `vitepress preview`. The canonical target
names are also available as `make docs-dev`, `make docs-build`, and `make docs-preview`.

Before committing documentation changes, run:

```sh
bun run typecheck
bun run build
```

Merging documentation changes into `main` does not deploy the site. Production documentation is
published by the Release workflow after it creates a GitHub Release. The documentation workflow can
also be started manually with `workflow_dispatch` when a deployment needs to be retried.

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
