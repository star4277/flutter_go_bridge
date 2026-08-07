package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
)

func TestGenerateAllEmitsSharedAPIAndPlatformWires(t *testing.T) {
	dir := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "pubspec.yaml"), "name: dual_fixture\ndependencies:\n  flutter:\n    sdk: flutter\n")
	write(filepath.Join(dir, "go.mod"), "module example.com/dual\n\ngo 1.24\n")
	input := filepath.Join(dir, "api")
	write(filepath.Join(input, "lib.go"), "package api\n\nfunc Add(a, b int) int { return a + b }\n")
	write(filepath.Join(input, "native.go"), "package api\n\nimport \"C\"\n\nfunc NativeOnly() int { return 1 }\n")
	resolved := config.Resolved{
		BaseDir: dir, GoInput: input,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "lib", "src", "bridge_generated.dart"),
		LibraryName: "go_lib_dual_fixture", DartEntrypointClassName: "FlutterGoBridge",
		StopOnError: true,
	}
	if _, err := WriteSupportPackage(resolved); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: input, BaseDir: dir, Target: config.TargetWeb})
	if err != nil {
		t.Fatal(err)
	}
	result, err := GenerateAll(api, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 8 {
		t.Fatalf("dual generation wrote an unexpected file set: %#v", result.Files)
	}
	if !containsError(result.Warnings, "NativeOnly is unavailable on Web") {
		t.Fatalf("dual generation must retain an explicit Web warning: %#v", result.Warnings)
	}

	nativeGo := mustRead(t, resolved.GoOutput)
	webGo := mustRead(t, webGoOutputPath(resolved.GoOutput))
	if !strings.HasPrefix(nativeGo, "//go:build !(js && wasm)") || !strings.Contains(nativeGo, `import "C"`) {
		t.Fatalf("native Go bridge is missing its build constraint or C ABI:\n%s", nativeGo)
	}
	if !strings.HasPrefix(webGo, "//go:build js && wasm") || !strings.Contains(webGo, `"syscall/js"`) {
		t.Fatalf("Web Go bridge is missing its build constraint or JS transport:\n%s", webGo)
	}
	if strings.Contains(webGo, `case "NativeOnly":`) {
		t.Fatalf("Web Go bridge must omit the cgo callable:\n%s", webGo)
	}

	common := mustRead(t, resolved.DartOutput)
	if !strings.Contains(common, "bridge_generated.io.dart") || !strings.Contains(common, "bridge_generated.web.dart") ||
		!strings.Contains(common, "dart.library.js_interop") {
		t.Fatalf("shared Dart bridge does not select a platform wire:\n%s", common)
	}
	if strings.Contains(common, "import 'dart:ffi'") || strings.Contains(common, "import 'dart:js_interop'") ||
		!strings.Contains(common, "final class _FgbCodec") {
		t.Fatalf("shared Dart bridge contains platform code or lost its shared codec:\n%s", common)
	}

	ioDart := mustRead(t, filepath.Join(dir, "lib", "src", "bridge_generated.io.dart"))
	if !strings.Contains(ioDart, "import 'dart:ffi' as ffi;") || !strings.Contains(ioDart, "extension FgbGeneratedCalls") ||
		strings.Contains(ioDart, "final class _FgbCodec") || strings.Contains(ioDart, "final class FgbPlatformException") {
		t.Fatalf("Native Dart wire is not transport-only:\n%s", ioDart)
	}
	webDart := mustRead(t, filepath.Join(dir, "lib", "src", "bridge_generated.web.dart"))
	if !strings.Contains(webDart, "WidgetsFlutterBinding.ensureInitialized()") || !strings.Contains(webDart, "FgbWasmLoader.ensureReady()") {
		t.Fatalf("Flutter Web runtime is missing its default initializer:\n%s", webDart)
	}
	for _, want := range []string{
		`const _fgbWasmAssetPackage = "go_lib_dual_fixture";`,
		"final class FgbWasmManifest",
		"final class FgbWasmLoader",
		"import 'package:flutter/services.dart';",
		"if (_ready != null) {",
		"for (var attempt = 0; attempt < 200; attempt++) {",
	} {
		if !strings.Contains(webDart, want) {
			t.Fatalf("generated Web wire is missing embedded loader fragment %q:\n%s", want, webDart)
		}
	}
	if strings.Contains(webDart, "fgb_wasm_loader.dart") {
		t.Fatalf("generated Web wire still imports a separate loader:\n%s", webDart)
	}
	if strings.Contains(webDart, "final class _FgbCodec") || strings.Contains(webDart, "final class FgbPlatformException") {
		t.Fatalf("Web Dart wire duplicates shared runtime declarations:\n%s", webDart)
	}

	sharedAPI := mustRead(t, filepath.Join(dir, "lib", "src", "api", "lib.dart"))
	if !strings.Contains(sharedAPI, `import "../bridge_generated.dart";`) || !strings.Contains(sharedAPI, "int add(") ||
		strings.Contains(sharedAPI, "dart.library.js_interop") {
		t.Fatalf("API source is not shared:\n%s", sharedAPI)
	}
	for _, duplicate := range []string{filepath.Join(dir, "lib", "src", "native"), filepath.Join(dir, "lib", "src", "web")} {
		if _, err := os.Stat(duplicate); !os.IsNotExist(err) {
			t.Fatalf("duplicated platform API directory exists: %s", duplicate)
		}
	}
}
