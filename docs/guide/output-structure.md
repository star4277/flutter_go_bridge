# Output structure

The generator writes one Go entrypoint and mirrors the Go source tree into Dart.

```text
go/
├── go.mod
├── bridge_generated.go
├── internal/fgb/fgb_generated.go
└── api/
    ├── api.go
    └── account.go

lib/src/
├── bridge_generated.dart
└── api/
    ├── api.dart
    └── account.dart
```

For `target: native`, `bridge_generated.go` contains cgo exports, the dispatcher, and codec
implementations. For `target: web`, the same path instead contains a pure-Go `syscall/js` registry,
dispatcher, and codec with no C ABI. `bridge_generated.dart` similarly selects FFI or JS interop
imports for the configured target. The support package under `internal/fgb` is generated only when
the API needs `DartOpaque` or `StreamSink`.

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

Do not edit generated files. Change Go declarations or configuration, then run `generate` again.
