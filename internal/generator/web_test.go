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

func TestGenerateWebPureGoAndCgoFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/web-generator\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "pure.go"), []byte("package api\n\nfunc Add(a, b int) int { return a + b }\n\n//fgb:async\nfunc Multiply(a, b int) int { return a * b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "native.go"), []byte("package api\n\nimport \"C\"\n\nfunc NativeOnly() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir, Target: config.TargetWeb})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	result, err := Generate(api, config.Resolved{
		Target: config.TargetWeb, BaseDir: dir, GoInput: inputDir,
		GoOutput: goOutput, DartOutput: dartOutput, LibraryName: "go_lib_web_generator",
		DartEntrypointClassName: "WebBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0].Error(), "unavailable on Web") {
		t.Fatalf("missing cgo warning: %#v", result.Warnings)
	}
	goSource := mustRead(t, goOutput)
	for _, forbidden := range []string{`import "C"`, "//export ", "C.Fgb", "fgb_cst"} {
		if strings.Contains(goSource, forbidden) {
			t.Fatalf("Web Go bridge contains Native ABI fragment %q", forbidden)
		}
	}
	if !strings.Contains(goSource, `"syscall/js"`) || !strings.Contains(goSource, `"context"`) || !strings.Contains(goSource, `case "Add":`) || strings.Contains(goSource, `case "NativeOnly":`) {
		t.Fatalf("Web Go dispatch is incorrect:\n%s", goSource)
	}
	dartSource := mustRead(t, dartOutput)
	for _, forbidden := range []string{"import 'dart:ffi", "import 'dart:io", "import 'dart:isolate"} {
		if strings.Contains(dartSource, forbidden) {
			t.Fatalf("Web Dart bridge contains Native import %q", forbidden)
		}
	}
	if !strings.Contains(dartSource, "dart:js_interop") || !strings.Contains(dartSource, "Go method NativeOnly is unavailable on Web") {
		t.Fatalf("Web Dart bridge is missing JS transport or cgo fallback:\n%s", dartSource)
	}
	if !strings.Contains(dartSource, "rawResponse.isA<JSUint8Array>()") || strings.Contains(dartSource, "rawResponse is! JSUint8Array") {
		t.Fatalf("Web Dart bridge must use Wasm-compatible JS interop type checks:\n%s", dartSource)
	}
	if !strings.Contains(dartSource, "Future<void> initialize") || !strings.Contains(dartSource, "webInitializer") ||
		!strings.Contains(dartSource, "custom webInitializer in a pure-Dart package") {
		t.Fatalf("pure-Dart Web initialize must preserve the optional override and explain the default:\n%s", dartSource)
	}
	if strings.Contains(dartSource, "package:flutter/widgets.dart") || strings.Contains(dartSource, "FgbWasmLoader.ensureReady()") {
		t.Fatalf("standalone Web generation must not import the Flutter loader:\n%s", dartSource)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "fgb_web_build.json")); statErr != nil {
		t.Fatalf("Web bridge metadata was not generated: %v", statErr)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dartOutput), "fgb_wasm_loader.dart"), []byte(`final class FgbWasmLoader {
  static Future<void> ensureReady() async {}
}

`), 0o644); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "build", "-buildvcs=false", "-o", filepath.Join(dir, "bridge.wasm"), ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=js", "GOARCH=wasm")
	if output, buildErr := command.CombinedOutput(); buildErr != nil {
		t.Fatalf("generated Web bridge failed to compile: %v\n%s", buildErr, output)
	}
	if dart, lookErr := exec.LookPath("dart"); lookErr == nil {
		analyze := exec.Command(dart, "analyze", filepath.Dir(dartOutput))
		analyze.Dir = dir
		analyze.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true", "CI=true")
		if output, analyzeErr := analyze.CombinedOutput(); analyzeErr != nil {
			t.Fatalf("generated Web Dart bridge failed analysis: %v\n%s", analyzeErr, output)
		}
	}
}

func TestGenerateWebFfiParityFixtureCompiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/web-parity\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	if _, err := WriteSupportPackage(config.Resolved{BaseDir: dir, GoInput: inputDir, GoOutput: goOutput}); err != nil {
		t.Fatal(err)
	}
	const source = `package api

import "example.com/web-parity/internal/fgb"

type Point struct {
	X int
	Y int
}

func (p Point) Shift(delta int) Point { return Point{X: p.X + delta, Y: p.Y + delta} }

//fgb:opaque
type Counter struct{ total int }

func NewCounter() *Counter                 { return &Counter{} }
func (c *Counter) Add(delta int) int       { c.total += delta; return c.total }
type Shape interface{ Area() int }
type Circle struct{ Radius int }
func (c Circle) Area() int                 { return c.Radius * c.Radius }
func Describe(shape Shape) int             { return shape.Area() }
func Keep(value fgb.DartOpaque) fgb.DartOpaque { return value }

//fgb:async
func Invoke(callback func(int) (int, error), value int) (int, error) {
	return callback(value)
}

//fgb:async
func Emit(count int, sink fgb.StreamSink[int]) error {
	defer sink.Close()
	for i := 0; i < count; i++ {
		if err := sink.Add(i); err != nil { return err }
	}
	return nil
}

//fgb:async
func EmitChannel(count int, sink chan<- int) error {
	defer close(sink)
	for i := 0; i < count; i++ { sink <- i }
	return nil
}
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir, Target: config.TargetWeb})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, config.Resolved{
		Target: config.TargetWeb, BaseDir: dir, GoInput: inputDir,
		GoOutput: goOutput, DartOutput: filepath.Join(dir, "dart", "bridge_generated.dart"),
		LibraryName: "go_lib_web_parity", DartEntrypointClassName: "WebParityBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("pure-Go parity fixture unexpectedly warned: %v", result.Warnings)
	}
	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		`case "Point.Shift":`, `case "NewCounter":`, `case "Counter.Add":`,
		`case "Describe":`, `case "Keep":`, `case "Invoke":`,
		`case "Emit":`, `case "EmitChannel":`,
		"fgbInvokeCallback", "fgbPostStreamEvent", "fgbCurrentDartOpaqueGeneration",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Web bridge is missing parity implementation %q:\n%s", expected, goSource)
		}
	}
	command := exec.Command("go", "build", "-buildvcs=false", "-o", filepath.Join(dir, "bridge.wasm"), ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=js", "GOARCH=wasm")
	if output, buildErr := command.CombinedOutput(); buildErr != nil {
		t.Fatalf("generated Web parity bridge failed to compile: %v\n%s", buildErr, output)
	}
	if dart, lookErr := exec.LookPath("dart"); lookErr == nil {
		analyze := exec.Command(dart, "analyze", filepath.Dir(filepath.Join(dir, "dart", "bridge_generated.dart")))
		analyze.Dir = dir
		analyze.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true", "CI=true")
		if output, analyzeErr := analyze.CombinedOutput(); analyzeErr != nil {
			t.Fatalf("generated Web parity Dart bridge failed analysis: %v\n%s", analyzeErr, output)
		}
	}
}

func TestGeneratedFlutterWebInitializerBindsBeforeWasm(t *testing.T) {
	source, err := dartWebRuntimeSource(&unit{
		ClassName:           "FlutterGoBridge",
		LibraryName:         "go_lib_fixture",
		UseFlutterWebLoader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding := strings.Index(source, "WidgetsFlutterBinding.ensureInitialized()")
	loader := strings.Index(source, "FgbWasmLoader.ensureReady()")
	if binding < 0 || loader < 0 || binding > loader {
		t.Fatalf("Flutter Web initializer must bind before loading Wasm:\n%s", source)
	}
}

func TestGeneratedNativeInitializeAcceptsWebInitializer(t *testing.T) {
	_, central, _, _, err := generateFixture(t, `package api

func Add(a, b int) int { return a + b }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(central, "Future<void> initialize") || !strings.Contains(central, "webInitializer") {
		t.Fatalf("Native initialize must keep the same async API:\n%s", central)
	}
}
