package parser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/model"
)

func writeFixture(t *testing.T, module string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+module+"\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if parent := filepath.Dir(path); parent != dir {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestParseDefaultBaseDirAndPrintAST(t *testing.T) {
	dir := writeFixture(t, "example.com/defaultbase", map[string]string{
		"api.go": "package defaultbase\n\nfunc F() int { return 1 }\n",
	})
	var buf bytes.Buffer
	api, err := Parse(Options{Input: dir, PrintAST: true, ASTOut: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Callables) != 1 {
		t.Fatalf("got %d callables, want 1", len(api.Callables))
	}
	if buf.Len() == 0 {
		t.Fatal("expected AST output when PrintAST is enabled")
	}
}

func TestParseRejectsMultiPackagePattern(t *testing.T) {
	dir := writeFixture(t, "example.com/multi", map[string]string{
		"a/api.go": "package a\n\nfunc A() int { return 1 }\n",
		"b/api.go": "package b\n\nfunc B() int { return 1 }\n",
	})
	_, err := Parse(Options{Input: "example.com/multi/...", BaseDir: dir})
	if err == nil || !strings.Contains(err.Error(), "exactly one package") {
		t.Fatalf("expected exactly-one-package error, got %v", err)
	}
}

func TestParseDropsMethodOnUnexportedReceiver(t *testing.T) {
	dir := writeFixture(t, "example.com/unexported-recv", map[string]string{
		"api.go": "package p\n\ntype helper struct{ v int }\n\nfunc (h helper) Visible() int { return h.v }\n",
	})
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Callables) != 0 {
		t.Fatalf("methods on unexported receiver types must be dropped, got %d callables", len(api.Callables))
	}
}

func TestParseRejectsModeOnType(t *testing.T) {
	dir := writeFixture(t, "example.com/mode-type", map[string]string{
		"api.go": "package p\n\n//fgb:sync\ntype T int\n",
	})
	_, err := Parse(Options{Input: dir, BaseDir: dir})
	if err == nil || !strings.Contains(err.Error(), "not type declarations") {
		t.Fatalf("expected mode-on-type rejection, got %v", err)
	}
}

func TestParseAliasTypeNotCollected(t *testing.T) {
	dir := writeFixture(t, "example.com/alias-type", map[string]string{
		"api.go": "package p\n\ntype Good = int\n",
	})
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Types) != 0 {
		t.Fatalf("type alias should not produce a named TypeDecl, got %d", len(api.Types))
	}
}

func TestParseConstIgnoreUnexportedAndUntyped(t *testing.T) {
	dir := writeFixture(t, "example.com/const-branches", map[string]string{
		"api.go": "package p\n\nconst (\n\t//fgb:ignore\n\tDropped = 1\n\thidden = 2\n\tKept = 3\n)\n",
	})
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Constants) != 0 {
		t.Fatalf("ignored, unexported, and untyped constants should not be collected, got %d", len(api.Constants))
	}
}

func TestParseInterfaceInvalidMethodDirective(t *testing.T) {
	dir := writeFixture(t, "example.com/iface-bad-method", map[string]string{
		"api.go": "package p\n\ntype Loader interface {\n\t//fgb:bogus\n\tM() int\n}\n",
	})
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var loader *model.TypeDecl
	for _, decl := range api.Types {
		if decl.Object.Name() == "Loader" {
			loader = decl
		}
	}
	if loader == nil || loader.Methods == nil || loader.Methods["M"] == nil {
		t.Fatalf("interface method directives should fall back to defaults on invalid directive: %#v", loader)
	}
	if m := loader.Methods["M"]; m.DartName != "m" || m.Mode != model.CallModeSync {
		t.Fatalf("default fallback not applied: %#v", m)
	}
}

func TestParseTypeGroupDeclDocInvalidDirective(t *testing.T) {
	dir := writeFixture(t, "example.com/type-decl-doc", map[string]string{
		"api.go": "package p\n\n//fgb:bogus\ntype (\n\tA int\n)\n",
	})
	_, err := Parse(Options{Input: dir, BaseDir: dir})
	if err == nil || !strings.Contains(err.Error(), "type A") {
		t.Fatalf("invalid decl-group directive should be surfaced with the type name, got %v", err)
	}
}

func TestParseEnumTypeDirective(t *testing.T) {
	dir := writeFixture(t, "example.com/enum-directive", map[string]string{
		"api.go": "package p\n\n//fgb:enum\ntype Status int\n\nconst (\n\tStatusUnknown Status = 0\n)\n",
	})
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var status *model.TypeDecl
	for _, decl := range api.Types {
		if decl.Object.Name() == "Status" {
			status = decl
		}
	}
	if status == nil || !status.Enum {
		t.Fatalf("enum directive was not recorded: %#v", status)
	}
}

func TestParseRejectsEnumOnFunctionAndConstant(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"function", "package p\n\n//fgb:enum\nfunc Work() int { return 1 }\n", "enum"},
		{"constant", "package p\n\nconst (\n\t//fgb:enum\n\tValue = 1\n)\n", "enum"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dir := writeFixture(t, "example.com/enum-"+test.name, map[string]string{"api.go": test.source})
			_, err := Parse(Options{Input: dir, BaseDir: dir})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected enum placement rejection, got %v", err)
			}
		})
	}
}
