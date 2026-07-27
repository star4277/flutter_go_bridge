package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	bridgeparser "github.com/star4277/flutter-go-bridge-gokit/internal/parser"
)

func TestGenerateStableABIAndDartAPIDL(t *testing.T) {
	input := filepath.Join("..", "parser", "testdata", "sample")
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: input, BaseDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	resolved := config.Resolved{
		BaseDir: dir, GoInput: input,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "lib", "bridge_generated.dart"),
		LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge",
		DartFormatLineLength: 100, StopOnError: true,
	}
	result, err := Generate(api, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("got %d files, want Go bridge, central Dart, and api.dart", len(result.Files))
	}
	for _, legacy := range []string{
		strings.TrimSuffix(resolved.GoOutput, ".go") + ".c",
		strings.TrimSuffix(resolved.GoOutput, ".go") + ".h",
	} {
		if _, err := os.Stat(legacy); !os.IsNotExist(err) {
			t.Fatalf("legacy C companion must not be generated: %s", legacy)
		}
	}
	goSource := mustRead(t, resolved.GoOutput)
	for _, expected := range []string{
		"//export fgb_init", "//export fgb", "//export fgb_async",
		"//export fgb_cst", "//export fgb_cst_async", "//export fgb_dco_free",
		"fgbStandardMethodCodec", "fgb_internal_post_bytes", "typedef FgbDartCObject Dart_CObject",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go source missing %q", expected)
		}
	}
	dartSource := mustRead(t, resolved.DartOutput)
	for _, expected := range []string{"final class FixtureBridge", "FgbPlatformException", "NativeApi.initializeApiDLData", "ReceivePort", "lookupFunction<_FgbSyncNative"} {
		if !strings.Contains(dartSource, expected) {
			t.Fatalf("generated Dart source missing %q", expected)
		}
	}
	if strings.Contains(dartSource, "native_assets") || strings.Contains(goSource, "native_assets") {
		t.Fatal("generated output must not depend on Native Assets")
	}
	if strings.Contains(dartSource, "package:flutter/") || strings.Contains(dartSource, "dispose(") {
		t.Fatal("generated Dart must be pure Dart and must not expose manual dispose")
	}
	sourceFile := mustRead(t, filepath.Join(dir, "lib", "api.dart"))
	for _, expected := range []string{
		"int calculate", "fgbInternalCall", `import "bridge_generated.dart";`,
		"final class Value",
	} {
		if !strings.Contains(sourceFile, expected) {
			t.Fatalf("generated per-source Dart library missing %q", expected)
		}
	}
	if strings.Contains(sourceFile, "\nexport ") {
		t.Fatalf("per-source Dart library must not re-export anything:\n%s", sourceFile)
	}
	if _, err := os.Stat(filepath.Join(dir, "lib", "_types.dart")); !os.IsNotExist(err) {
		t.Fatal("a separate _types.dart must not be generated")
	}
}

func TestGenerateCstDcoStructFieldsAndStandardFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/codec-modes\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

type Child struct { Value int }

type Value struct {
	Count int
	Optional *int
	Child *Child
}

func RoundTrip(value Value) Value { return value }

func Fallback(value map[string]int) map[string]int { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "codec_modes", DartEntrypointClassName: "CodecBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	apiDart := mustRead(t, filepath.Join(dir, "dart", "api.dart"))
	for _, expected := range []string{
		"final int count;", "final int? optional;", "final Child? child;",
		"required this.count", "this.optional", "this.child", "fgbInternalCall",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart API missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "dart:ffi") || strings.Contains(apiDart, "fgbInvokeCst") {
		t.Fatalf("per-source API leaked FFI details:\n%s", apiDart)
	}

	central := mustRead(t, dartOutput)
	for _, expected := range []string{
		"extends ffi.Struct", "external ffi.Pointer<ffi.Int64> optional;",
		"fgbInvokeCstSync(0", `fgbInvokeSync("Fallback"`, "if (value is List)",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("central Dart bridge missing %q", expected)
		}
	}

	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		"int64_t count;", "int64_t* optional;", "fgbDispatchCst", "fgbDcoEncode",
		`case "Fallback":`, "//export fgb_cst", "//export fgb_dco_free",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing %q", expected)
		}
	}
}

func TestGenerateCallModeEmitsExactlyOneDartEntrypoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/modes\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

func Plain(value int) int { return value }

//fgb:sync
func ExplicitSync(value int) int { return value }

//fgb:async
func ExplicitAsync(value int) int { return value }

type Worker struct{}

func NewWorker() *Worker { return &Worker{} }

//fgb:async
func (worker *Worker) Run(value int) int { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "bridge_generated.dart"),
		LibraryName: "modes", DartEntrypointClassName: "ModesBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}
	dart := mustRead(t, filepath.Join(dir, "api.dart"))
	for _, name := range []string{"plain", "explicitSync", "explicitAsync"} {
		if strings.Count(dart, " "+name+"(") != 1 {
			t.Fatalf("expected exactly one generated entrypoint named %s:\n%s", name, dart)
		}
	}
	if strings.Count(dart, "Future<int> run(") != 1 || strings.Contains(dart, "runSync(") || strings.Contains(dart, "runAsync(") {
		t.Fatalf("method mode/name generation is wrong:\n%s", dart)
	}
	if strings.Contains(dart, "plainSync") || strings.Contains(dart, "plainAsync") || strings.Contains(dart, "explicitSyncSync") || strings.Contains(dart, "explicitAsyncAsync") {
		t.Fatalf("mode suffix leaked into generated Dart:\n%s", dart)
	}
	central := mustRead(t, filepath.Join(dir, "bridge_generated.dart"))
	plainStart := strings.Index(central, "fgbInternalCall0")
	asyncStart := strings.Index(central, "fgbInternalCall2")
	if plainStart < 0 || !strings.Contains(central[plainStart:plainStart+280], "fgbInvokeCstSync") {
		t.Fatal("unmarked function did not use sync CST invocation")
	}
	if asyncStart < 0 || !strings.Contains(central[asyncStart:asyncStart+320], "fgbInvokeCstAsync") {
		t.Fatal("fgb(async) function did not use async CST invocation")
	}
}

func TestGenerateDirectMainPackageDoesNotDuplicateMain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/direct-main\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte("package main\n\nfunc main() {}\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "bridge_generated.dart")
	_, err = Generate(api, config.Resolved{
		BaseDir: dir, GoInput: dir, GoOutput: goOutput,
		DartOutput:  dartOutput,
		LibraryName: "direct_main", DartEntrypointClassName: "MyFlutterGoBridge",
		StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustRead(t, goOutput), "func main() {}") {
		t.Fatal("generated file duplicated the user's main function")
	}
	dartSource := mustRead(t, dartOutput)
	if !strings.Contains(dartSource, "final class MyFlutterGoBridge") || strings.Contains(dartSource, "MyMyFlutterGoBridge") {
		t.Fatal("Dart entrypoint class token replacement was applied more than once")
	}
	sourcePath := filepath.Join(dir, "api.dart")
	source := mustRead(t, sourcePath)
	if !strings.Contains(source, "MyFlutterGoBridge.instance") {
		t.Fatal("per-source Dart library did not use the configured bridge class")
	}
	if strings.Contains(source, "\nexport ") {
		t.Fatal("per-source Dart library must not re-export the bridge")
	}
}

func TestGenerateDartFilesMirrorGoSourceFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/multi-source\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	firstGo := `package api

type FirstValue struct { Value int }

func First(value FirstValue) FirstValue { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "first.go"), []byte(firstGo), 0o644); err != nil {
		t.Fatal(err)
	}
	secondGo := `package api

type SecondValue struct { Value int }

func Second(value SecondValue) SecondValue { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "second.go"), []byte(secondGo), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(dir, "dart")
	result, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput: filepath.Join(dir, "bridge_generated.go"), DartOutput: outputDir,
		LibraryName: "multi_source", DartEntrypointClassName: "MultiBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 4 {
		t.Fatalf("got %d files, want Go bridge, central, and two source Dart files", len(result.Files))
	}
	central := mustRead(t, filepath.Join(outputDir, "bridge_generated.dart"))
	if !strings.Contains(central, `import "first.dart";`) || !strings.Contains(central, `import "second.dart";`) {
		t.Fatalf("central Dart file does not mirror source files:\n%s", central)
	}
	if strings.Contains(central, `export "first.dart";`) || strings.Contains(central, `export "second.dart";`) {
		t.Fatalf("central Dart file must not re-export API files:\n%s", central)
	}
	first := mustRead(t, filepath.Join(outputDir, "first.dart"))
	second := mustRead(t, filepath.Join(outputDir, "second.dart"))
	if !strings.Contains(first, `import "bridge_generated.dart";`) || !strings.Contains(first, "FirstValue first({required FirstValue value})") || !strings.Contains(first, "final class FirstValue") || strings.Contains(first, "SecondValue") {
		t.Fatalf("first.dart contains the wrong API: %s", first)
	}
	if !strings.Contains(second, "SecondValue second({required SecondValue value})") || !strings.Contains(second, "final class SecondValue") || strings.Contains(second, "FirstValue") {
		t.Fatalf("second.dart contains the wrong API: %s", second)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
