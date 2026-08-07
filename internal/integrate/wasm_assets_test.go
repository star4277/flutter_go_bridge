package integrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWebWasmAssetsAddsPluginAssetDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pubspec.yaml")
	if err := os.WriteFile(path, []byte("name: example\nflutter:\n  plugin:\n    platforms:\n      web:\n        pluginClass: Example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureWebWasmAssets(dir, TemplatePlugin); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "assets/wasm/") {
		t.Fatalf("asset declaration missing: %s", first)
	}
	if err := ensureWebWasmAssets(dir, TemplatePlugin); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(second), "assets/wasm/") != 1 {
		t.Fatalf("asset declaration should be idempotent: %s", second)
	}
}

func TestEnsureWebWasmAssetsLeavesAppRootPubspecUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pubspec.yaml")
	original := "name: example\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureWebWasmAssets(dir, TemplateApp); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != original {
		t.Fatalf("app root pubspec changed: %q", actual)
	}
}
