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

`bridge_generated.go` contains the cgo exports, dispatcher and codec implementations. The
support package under `internal/fgb` is generated only when the API needs `DartOpaque` or
`StreamSink`.

Each Go source file produces a same-named Dart file. The mirror is anchored at the Go module root:
`go/api/api.go` becomes `lib/src/api/api.dart`. Generated files are implementation details; keep
application-facing exports in your own Dart files.

Do not edit generated files. Change Go declarations or configuration, then run `generate` again.

