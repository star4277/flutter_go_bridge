package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
)

// webRuntimeParityAPI exercises every bridge capability the Web transport has
// to match: value structs, GoOpaque, interfaces, DartOpaque, callbacks, both
// stream forms, and a plain asynchronous call.
const webRuntimeParityAPI = `package api

import (
	"fmt"

	"example.com/webparity/internal/fgb"
)

type Point struct {
	X int
	Y int
}

func (p Point) Shift(delta int) Point { return Point{X: p.X + delta, Y: p.Y + delta} }

//fgb:opaque
type Counter struct{ total int }

func NewCounter() *Counter           { return &Counter{} }
func (c *Counter) Add(delta int) int { c.total += delta; return c.total }

type Shape interface{ Area() int }

type Circle struct{ Radius int }

func (c Circle) Area() int { return c.Radius * c.Radius }

func Describe(shape Shape) int { return shape.Area() }

func Keep(value fgb.DartOpaque) fgb.DartOpaque { return value }

//fgb:async
func Invoke(callback func(int) (int, error), value int) (int, error) {
	doubled, err := callback(value)
	if err != nil {
		return 0, fmt.Errorf("callback failed: %w", err)
	}
	return doubled + 1, nil
}

//fgb:async
func Emit(count int, sink fgb.StreamSink[int]) error {
	defer sink.Close()
	for i := 0; i < count; i++ {
		if err := sink.Add(i * 10); err != nil {
			return err
		}
	}
	return nil
}

//fgb:async
func EmitChannel(count int, sink chan<- int) error {
	defer close(sink)
	for i := 0; i < count; i++ {
		sink <- i * 100
	}
	return nil
}

//fgb:async
func SlowDouble(value int) int { return value * 2 }
`

// TestWebWasmRuntimeParity compiles the generated Web bridge to WebAssembly and
// drives it through a real JavaScript runtime, speaking the same standard codec
// and syscall/js entrypoints the generated Dart Web wire uses. Compilation
// alone cannot show that asynchronous calls actually suspend, that Go can call
// back into Dart mid-request, or that stream events reach the sink, so this is
// the test that keeps Web at feature parity with the Native FFI transport.
func TestWebWasmRuntimeParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; skipping the Web Wasm runtime parity check")
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("could not locate GOROOT: %v", err)
	}
	wasmExec := filepath.Join(strings.TrimSpace(string(goroot)), "lib", "wasm", "wasm_exec.js")
	if _, statErr := os.Stat(wasmExec); statErr != nil {
		t.Skipf("wasm_exec.js is missing from this Go toolchain: %v", statErr)
	}
	harness, err := filepath.Abs(filepath.Join("testdata", "web_wasm_harness.mjs"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	inputDir := filepath.Join(dir, "api")
	if mkErr := os.MkdirAll(inputDir, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/webparity\n\ngo 1.24\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	if _, writeErr := WriteSupportPackage(config.Resolved{BaseDir: dir, GoInput: inputDir, GoOutput: goOutput}); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(webRuntimeParityAPI), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	const libraryName = "webparity"
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir, Target: config.TargetWeb})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, config.Resolved{
		Target: config.TargetWeb, BaseDir: dir, GoInput: inputDir, GoOutput: goOutput,
		DartOutput: filepath.Join(dir, "lib", "src", "bridge_generated.dart"),
		LibraryName: libraryName, DartEntrypointClassName: "WebParityBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("pure-Go parity fixture unexpectedly warned: %v", result.Warnings)
	}

	build := exec.Command("go", "build", "-buildvcs=false", "-o", filepath.Join(dir, "bridge.wasm"), ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=js", "GOARCH=wasm")
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("generated Web bridge failed to compile: %v\n%s", buildErr, output)
	}
	runtimeSupport, err := os.ReadFile(wasmExec)
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), runtimeSupport, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	drive := exec.Command(node, harness, dir, libraryName)
	drive.Dir = dir
	output, err := drive.CombinedOutput()
	t.Logf("Web Wasm harness output:\n%s", output)
	if err != nil {
		t.Fatalf("Web Wasm runtime parity failed: %v", err)
	}
}
