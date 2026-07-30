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
## [LRN-20260730-002] correction

**Logged**: 2026-07-30T09:14:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: infra

### Summary
Codecov `ignore` filters uploaded coverage paths but does not remove a Go package from `go test ./...` or its CI log.

### Details
The workflow still enumerated `github.com/star4277/flutter_go_bridge/template` because the Go command selected every package before Codecov processed the coverage profile. Excluding a package from test execution requires filtering the package list passed to `go test`.

### Suggested Action
Keep `.codecov.yml` ignore rules for uploaded paths, and separately build the Go package list with `go list ./... | grep -v '/template$'` before running tests.

### Metadata
- Source: user_feedback
- Related Files: .github/workflows/test.yml, .codecov.yml
- Tags: codecov, go-test, coverage, github-actions

### Resolution
- **Resolved**: 2026-07-30T09:14:00+08:00
- **Notes**: The workflow now passes an explicitly filtered package array to `go test`.

---
