# Stable ABI

Business functions are dispatched by an index and are not exported as individual C symbols. The
bridge exposes a fixed set of symbols:

`fgb_init`, `fgb_cst`, `fgb_cst_async`, `fgb_dco_free`, `fgb`, `fgb_async`, `fgb_alloc`, `fgb_free`,
`fgb_drop`, `fgb_isolate_attach`, `fgb_callback_result`,
and `fgb_stream_cancel`.

Adding or renaming a Go function changes generated dispatch tables, not CMake, Gradle or podspec
linkage. `bridge_generated.go` contains the cgo export declarations; no C header or source file is
generated.
