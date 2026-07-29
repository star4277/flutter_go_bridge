## [LRN-20260729-001] correction

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: backend

### Summary
External-struct support must be validated against the real `example/mihomoui` API returning `*statistic.Snapshot`, not only synthetic local modules.

### Details
The initial implementation passed a simplified regression fixture but the actual generator invocation still failed for `GetAllConnections() *statistic.Snapshot` from `github.com/metacubex/mihomo/tunnel/statistic`.

### Suggested Action
Reproduce generation in `example/mihomoui`, inspect the complete reachable field graph and generated Go/Dart output, then make that example a permanent regression test.

### Metadata
- Source: user_feedback
- Related Files: example/mihomoui/go/api/connection.go, internal/generator/builder.go
- Tags: external-struct, pointer, real-world-fixture

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: The real mihomoui fixture now generates `Snapshot?`, scalar atomic fields, `InternetAddress`, and `UuidValue`; its generated Go bridge compiles and Dart analysis reports no errors or warnings.

---
