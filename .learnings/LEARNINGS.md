# Learnings

## [LRN-20260731-001] correction

**Logged**: 2026-07-31T09:30:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: docs

### Summary

The documentation site uses Bun, not npm.

### Details

The repository contains `docs/bun.lock`, and Bun 1.3.13 is available. Documentation dependency, development, type-check, and build commands should use `bun install` and `bun run ...`.

### Suggested Action

Use Bun for all docs commands and document Bun as the package manager in contributor instructions.

### Metadata

- Source: user_feedback
- Related Files: docs/bun.lock, docs/package.json
- Tags: bun, vitepress, docs

### Resolution

- **Resolved**: 2026-07-31T09:30:00+08:00
- **Notes**: Switched validation and authored commands to Bun.

---
## [LRN-20260731-001] correction

**Logged**: 2026-07-31T12:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: docs

### Summary

A request to rewrite bilingual README files means redesigning their content, not migrating the old README verbatim.

### Details

The initial interpretation planned to preserve the existing long Chinese README as the Chinese version and translate its structure into English. The user explicitly required both README files to be rewritten around the project's current positioning and documentation site instead.

### Suggested Action

For documentation rewrite requests, first define a new information architecture and remove material better served by the full documentation site. Treat translation/migration as a separate request that requires explicit wording.

### Metadata

- Source: user_feedback
- Related Files: README.md, README.zh-CN.md
- Tags: readme, rewrite, bilingual

### Resolution

- **Resolved**: 2026-07-31T12:00:00+08:00
- **Notes**: Replaced the migration approach with fresh English and Chinese README outlines and content.

---
