package integrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestEnsureWebWasmAssetsReportsInvalidPluginPubspec(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "missing file", want: "read plugin pubspec"},
		{name: "malformed yaml", data: "flutter: [", want: "parse plugin pubspec"},
		{name: "non mapping root", data: "- example\n", want: "must contain a YAML mapping"},
		{name: "assets scalar", data: "flutter:\n  assets: assets/\n", want: "flutter.assets must be a list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.data != "" {
				if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte(test.data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := ensureWebWasmAssets(dir, TemplatePlugin)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ensureWebWasmAssets() error = %v, want fragment %q", err, test.want)
			}
		})
	}
}

func TestMappingValueRejectsNonMappingsAndMissingKeys(t *testing.T) {
	if mappingValue(nil, "flutter") != nil {
		t.Fatal("nil mapping should not contain a value")
	}
	if mappingValue(&yaml.Node{Kind: yaml.SequenceNode}, "flutter") != nil {
		t.Fatal("sequence node should not be treated as a mapping")
	}
	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "flutter"},
		{Kind: yaml.MappingNode},
	}}
	if mappingValue(mapping, "missing") != nil {
		t.Fatal("missing mapping key should return nil")
	}
	if mappingValue(mapping, "flutter") == nil {
		t.Fatal("existing mapping key should be returned")
	}
}
