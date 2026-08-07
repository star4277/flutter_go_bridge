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
	if _, statErr := os.Stat(filepath.Join(dir, "fgb_web_build.json")); statErr != nil {
		t.Fatalf("Web bridge metadata was not generated: %v", statErr)
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
