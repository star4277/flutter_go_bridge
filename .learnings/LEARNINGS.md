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
