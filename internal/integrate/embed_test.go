package integrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	templatefs "github.com/star4277/flutter_go_bridge/template"
)

// TestRunAppWithEmbeddedTemplates exercises the real embedded template tree,
// catching placeholder leftovers and rename mistakes in template/.
func TestRunAppWithEmbeddedTemplates(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "include: package:flutter_lints/flutter.yaml\n")
	writeFile(t, filepath.Join(dir, "lib", "main.dart"), "void main() {}\n")

	if err := run(baseConfig(dir, TemplateApp), templatefs.FS); err != nil {
		t.Fatal(err)
	}

	goMod := readFile(t, filepath.Join(dir, "go", "go.mod"))
	if !strings.Contains(goMod, "module com.flutter_go_bridge/go_lib_my_app") {
		t.Fatalf("unexpected go.mod: %q", goMod)
	}
	bridgeGo := readFile(t, filepath.Join(dir, "go", "bridge_generated.go"))
	if !strings.Contains(bridgeGo, "\"com.flutter_go_bridge/go_lib_my_app/api\"") {
		t.Fatal("bridge_generated.go should import the templated api package")
	}
	if _, err := os.Stat(filepath.Join(dir, "go", "api", "lib.go")); err != nil {
		t.Fatalf("go/api/lib.go missing: %v", err)
	}
	bridgeDart := readFile(t, filepath.Join(dir, "lib", "src", "bridge_generated.dart"))
	if !strings.Contains(bridgeDart, `const libraryName = "go_lib_my_app";`) {
		t.Fatal("bridge_generated.dart should embed the library name")
	}
	webLoader := readFile(t, filepath.Join(dir, "lib", "src", "fgb_wasm_loader_web.dart"))
	for _, want := range []string{
		"const _assetPackage = 'go_lib_my_app';",
		"const _assetKeyRoot = 'packages/$_assetPackage/assets/wasm';",
		"const _assetUrlRoot = 'assets/$_assetKeyRoot';",
		"value['target'] != 'web-wasm'",
		"assets.loadString('$_assetKeyRoot/fgb_wasm_manifest.json')",
		"result.isA<JSObject>()",
		"script.isA<JSObject>()",
	} {
		if !strings.Contains(webLoader, want) {
			t.Fatalf("Web loader missing %q: %s", want, webLoader)
		}
	}
	bridgeConfig := readFile(t, filepath.Join(dir, "flutter_go_bridge.yaml"))
	for _, want := range []string{"go_input: go/api", "go_output: go/bridge_generated.go", "library_name: go_lib_my_app"} {
		if !strings.Contains(bridgeConfig, want) {
			t.Fatalf("flutter_go_bridge.yaml missing %q: %q", want, bridgeConfig)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "go_builder", "gokit", "build_tool", "pubspec.yaml")); err != nil {
		t.Fatalf("gokit build_tool missing (submodule not embedded?): %v", err)
	}

	// No placeholder may survive in produced paths or text files.
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(path, "REPLACE_ME") {
			t.Errorf("path with placeholder: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "REPLACE_ME") {
			t.Errorf("content with placeholder: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEmbeddedTemplatesContainGokit fails when the gokit submodules were not
// initialized at build time, which would ship a broken integrate command.
func TestEmbeddedTemplatesContainGokit(t *testing.T) {
	for _, sentinel := range []string{
		"app/go_builder/gokit/run_build_tool.sh",
		"app/go_builder/gokit/build_tool/pubspec.yaml",
		"app/go_builder/gokit/cmake/gokit.cmake",
		"plugin/gokit/run_build_tool.sh",
		"plugin/gokit/build_tool/pubspec.yaml",
	} {
		if _, err := fs.Stat(templatefs.FS, sentinel); err != nil {
			t.Errorf("embedded template missing %s: %v (run `git submodule update --init --recursive`)", sentinel, err)
		}
	}
}
