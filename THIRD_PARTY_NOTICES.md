# Third-party notices

## flutter_rust_bridge architecture

The code-generation architecture and project organization of
`flutter_go_bridge` were designed with reference to
[`fzyzcjy/flutter_rust_bridge`](https://github.com/fzyzcjy/flutter_rust_bridge),
including its approach to generator structure, generated bridge separation,
and cross-language API workflow.

`flutter_go_bridge` is an independent Go-to-Dart implementation built for
Gokit and is not affiliated with or endorsed by the flutter_rust_bridge
project. flutter_rust_bridge is distributed under the MIT License, copyright
(c) 2021 fzyzcjy.

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
