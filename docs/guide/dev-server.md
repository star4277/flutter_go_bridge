# Dev server (`run`)

`run` combines generation, Flutter's machine daemon and two watch loops:

| Change | Action |
| --- | --- |
| Go input files | Generate, rebuild the native library, restart the whole app process |
| Dart input files | Flutter hot reload |

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

The default polling interval is 400ms. Generated files are excluded dynamically and content
hashes prevent identical rewrites from causing a rebuild. A failed regeneration keeps the current
app running after the initial successful start.

