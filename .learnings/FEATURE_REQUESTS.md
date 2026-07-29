## [FEAT-20260729-001] external-go-struct-dart-generation

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: high
**Status**: resolved
**Area**: backend

### Requested Capability
Generate corresponding Dart value types when an exported Go API returns a struct declared in a third-party repository/package.

### User Context
The bridge currently needs external return structs to remain strongly typed and serializable in generated Dart APIs.

### Complexity Estimate
complex

### Suggested Implementation
Extend reachable-type discovery beyond the input package, assign external types stable Dart ownership/imports, and cover encoding/decoding plus generated output with regression tests.

### Metadata
- Frequency: first_time
- Related Features: Go struct Dart class generation, DCO/CST codecs

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Reachable external value structs and nested named types are generated with Go import aliases and Dart name collision handling.

---
