# Feature Requests

## [FEAT-20260807-001] standalone WebAssembly preparation command

**Logged**: 2026-08-07T16:24:00+08:00
**Priority**: high
**Status**: resolved
**Area**: cli

### Requested Capability
Expose the Gokit `build-web` Wasm step as a dedicated
`flutter_go_bridge_codegen build-web` command.

### User Context
The existing `run` and `build web` workflows already need to compile Go to Wasm. A direct
`flutter run/build web` workflow should be able to prepare those assets through the same codegen
tooling without creating another Flutter plugin or relying on an IDE extension.

### Complexity Estimate
simple

### Suggested Implementation
Generate the shared Native/Web bridge once, call the existing `WebBuilder.BuildWasm()` implementation,
and stop before invoking Flutter. Keep `run` and `build web` on the same lower-level builder path.

### Metadata
- Frequency: first_time
- Related Features: web-wasm-support

### Resolution
- **Resolved**: 2026-08-07T16:24:00+08:00
- **Notes**: Added the Cobra command, focused success/failure/order tests, and bilingual CLI/Gokit
  documentation.

---
