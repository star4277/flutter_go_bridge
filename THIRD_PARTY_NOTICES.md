# Third-party notices

## flutter_rust_bridge architecture

The code-generation architecture and project organization of
`flutter_go_bridge` were designed with reference to
[`fzyzcjy/flutter_rust_bridge`](https://github.com/fzyzcjy/flutter_rust_bridge),
including its approach to generator structure, generated bridge separation,
and cross-language API workflow.

`flutter_go_bridge` is a Go-to-Dart implementation inspired by the
flutter_rust_bridge architecture. flutter_rust_bridge is distributed under the
MIT License, copyright (c) 2021 fzyzcjy.

The native build layer used by `flutter_go_bridge` is Gokit. Gokit's origin
and relationship to CargoKit are described separately below.

## Gokit / CargoKit build integration

The Gokit build integration bundled by this project was designed with
reference to and adapted from
[`irondash/cargokit`](https://github.com/irondash/cargokit). It carries
CargoKit's approach to integrating native library builds into Flutter's
platform build systems, with the Rust/Cargo-specific build flow adapted for
Go/CGO projects.

Gokit is an independent adaptation and is not affiliated with or endorsed by
the CargoKit project. CargoKit is copyright (c) 2022 Matej Knopp and is
distributed under the MIT License and the Apache License, Version 2.0. The
complete license text is retained in the Gokit submodules at
`template/plugin/gokit/LICENSE` and `template/app/go_builder/gokit/LICENSE`.

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
