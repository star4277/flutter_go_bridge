# Output structure

The generator writes one Go entrypoint and mirrors the Go source tree into Dart.

```text
go/
├── go.mod
├── bridge_generated.go
├── bridge_generated_web.go
├── fgb_web_build.json       # Web metadata used by Gokit build-web
├── internal/fgb/fgb_generated.go
└── api/
    ├── api.go
    └── account.go

lib/src/
├── bridge_generated.dart
├── bridge_generated.io.dart
├── bridge_generated.web.dart
└── api/
    ├── api.dart
    └── account.dart
```

One `generate` invocation emits both platform implementations. `bridge_generated.go` contains the
Native cgo exports, dispatcher, and codec; `bridge_generated_web.go` contains the pure-Go
`syscall/js` registry, dispatcher, and codec with no C ABI. `bridge_generated.dart` contains the
shared API, models, and standard codec, then conditionally selects `bridge_generated.io.dart` or
`bridge_generated.web.dart`. The support package under `internal/fgb` is generated only when the
API needs `DartOpaque` or `StreamSink`.

Each Go source file produces a same-named Dart file. The mirror is anchored at the Go module root:
`go/api/api.go` becomes `lib/src/api/api.dart`. Generated files are implementation details; keep
application-facing exports in your own Dart files.

Generated Dart uses lint-compatible lower-camel identifiers, control-flow blocks and string
interpolation, so it remains clean under the recommended Dart and Flutter analyzer rules. App
projects do not receive a named `library` directive in `bridge_generated.dart`. Plugin projects keep
their `library <plugin_name>;` declaration in the plugin's public entrypoint, which exports the
generated implementation.
Generation order is deterministic across runs: source files are ordered by normalized path and
declarations retain their order within each file, so unchanged input produces unchanged output.

The Web platform wire contains the Flutter asset loader (`FgbWasmManifest` and `FgbWasmLoader`), so
no separate loader Dart files are needed. Web generation also writes `fgb_web_build.json` beside the Go bridge. Gokit includes its protocol,
generator, library, and API hash in `fgb_wasm_manifest.json`; the Web loader validates the manifest
before starting `wasm_exec.js`. The manifest target is `web-wasm`. Flutter's asset bundle reads the
manifest through the logical `packages/<plugin>/assets/wasm/...` key, while browser requests for
`wasm_exec.js` and the `.wasm` artifact use the packaged
`assets/packages/<plugin>/assets/wasm/...` URL.

Do not edit generated files. Change Go declarations or configuration, then run `generate` again.
