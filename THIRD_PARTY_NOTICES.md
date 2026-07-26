# Third-party notices

## go-flutter StandardMessageCodec / StandardMethodCodec

The generated Go codec follows the binary format and implementation approach
of `github.com/go-flutter-desktop/go-flutter/plugin` (BSD 3-Clause License,
copyright Pierre Champion and contributors). The generated source keeps the
same type tags, size encoding, alignment rules, method-call layout, and result
envelopes. It is intentionally emitted as source so generated bridges do not
need to import the desktop `go-flutter` module.

The complete license text is in `licenses/go-flutter.LICENSE`.

## Dart API DL definitions

The small C declarations used by the generated bridge are derived from the
Dart SDK `runtime/include` headers (BSD-style license, copyright Dart project
authors). Only the `Dart_PostCObject` entry point and the corresponding
`Dart_CObject` typed-data layout are needed; the full Dart headers are not
vendored into generated projects.

The complete license text is in `licenses/dart.LICENSE`.
