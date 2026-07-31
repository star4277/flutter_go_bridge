# Learnings Log

Captured learnings, corrections, and discoveries. Review before major tasks.

---

## [LRN-20260731-001] correction

**Logged**: 2026-07-31T14:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: docs

### Summary

Gokit is based on and architecturally adapted from `irondash/cargokit`, so attribution must name CargoKit explicitly.

### Details

The previous third-party notice acknowledged flutter_rust_bridge for code-generation architecture but described Gokit only as an independent project component. Repository submodules and retained build architecture show that Gokit adapts CargoKit's Flutter native build integration from Rust/Cargo to Go/CGO. CargoKit is copyright 2022 Matej Knopp and is distributed under MIT and Apache-2.0 licenses.

### Suggested Action

Keep code-generation attribution (`flutter_rust_bridge`) separate from native build integration attribution (`irondash/cargokit`). Include CargoKit's dual-license and copyright information in third-party notices whenever Gokit provenance is described.

### Metadata

- Source: user_feedback
- Related Files: THIRD_PARTY_NOTICES.md, template/plugin/gokit/LICENSE
- Tags: gokit, cargokit, attribution, licensing

### Resolution

- **Resolved**: 2026-07-31T14:00:00+08:00
- **Notes**: Updated the third-party notice with a dedicated CargoKit/Gokit attribution section.

---
