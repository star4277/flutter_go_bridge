# Dev server (`run`)

`run` combines generation, Flutter's machine daemon and two watch loops:

| Change | Action |
| --- | --- |
| Go input files | Generate, rebuild the platform artifact, restart the whole app process |
| Dart input files | Flutter hot reload |

When the selected device is Web (`chrome`, `edge`, or `web-server`), the first generation also
invokes Gokit's `build-web`. A Go change repeats both generation and the Wasm build before the
Flutter process is restarted. Native devices do not invoke the Web builder.

Hot reload and hot restart recreate only the Dart isolate. A library opened with `dlopen` remains
resident, and Android also needs the new `.so` copied into the APK. A process restart is therefore
required for Go edits.

## Keys

| Key | Action |
| --- | --- |
| `r` | Hot reload |
| `R` | Hot restart |
| `g` | Generate and restart |
| `q` | Stop and quit |
| `d` | Detach |
| `h` | Show help |

Arguments after `--` are passed unchanged to `flutter run`, for example
`run -d windows -- --flavor dev`.

For a synchronous release/debug artifact instead of a watched daemon, use `build`:

```sh
flutter_go_bridge_codegen build web -- --release
flutter_go_bridge_codegen build windows -- --release
```

This command generates once, selects the Web or Native platform builder, runs the Flutter build,
and passes the resulting artifact set through the platform signing interface.

## Running Flutter Directly

`flutter run -d chrome` and `flutter build web` do not invoke
`flutter_go_bridge_codegen` or an arbitrary Go compiler. Without a Flutter build hook, run the
same Gokit build step first so the package's `assets/wasm/` directory contains the current
`.wasm`, `wasm_exec.js`, and `fgb_wasm_manifest.json`:

```powershell
go run ./cmd/flutter_go_bridge_codegen generate --config-file flutter_go_bridge.yaml
dart run go_builder/gokit/build_tool/bin/build_tool.dart build-web `
  --manifest-dir "$PWD/go" `
  --output-dir "$PWD/go_builder/assets/wasm" `
  --root-project-dir "$PWD"
flutter run -d chrome
# or: flutter build web
```

For a plugin project, use `gokit/build_tool/bin/build_tool.dart` and `assets/wasm/` instead. The
command is platform-independent. The generated Web bridge reads the manifest through the package
asset bundle; Flutter only packages the files that are already present.

The default polling interval is 400ms. Generated files are excluded dynamically and content
hashes prevent identical rewrites from causing a rebuild. A failed regeneration keeps the current
app running after the initial successful start.
