# Generated support package

When an API uses `fgb.DartOpaque` or `fgb.StreamSink`, generation writes a small support package
into the Go module:

```text
internal/fgb/fgb_generated.go
```

Import it from Go with the module path, for example:

```go
import "example.com/my_app/internal/fgb"
```

The package is generated as part of the same run and must stay inside the module so the generated
bridge can import it. Do not edit it by hand; change the API signature and regenerate.

