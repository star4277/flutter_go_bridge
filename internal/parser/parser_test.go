package parser

import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/star4277/flutter-go-bridge-gokit/internal/model"
)

func TestParseUsesOfficialTypedAST(t *testing.T) {
	api, err := Parse(Options{Input: filepath.Join("testdata", "sample"), BaseDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if api.Package.PkgPath != "example.com/flutter-go-bridge-parser-fixture" {
		t.Fatalf("unexpected package path %q", api.Package.PkgPath)
	}
	if len(api.Callables) != 4 {
		t.Fatalf("got %d callables, want 4", len(api.Callables))
	}
	if api.Callables[0].DartName != "calculate" {
		t.Fatalf("directive was not applied: %q", api.Callables[0].DartName)
	}
	method := api.Callables[3]
	if !method.PointerRecv || method.Receiver == nil || method.Receiver.Obj().Name() != "Ref" {
		t.Fatalf("method receiver was not resolved through go/types: %#v", method)
	}
	var state *types.Named
	for _, declaration := range api.Types {
		if declaration.Object.Name() == "State" {
			state = declaration.Named
		}
	}
	if state == nil || len(api.Constants[state]) != 2 {
		t.Fatalf("typed constants were not collected")
	}
	if DumpAST(api) == "" {
		t.Fatal("expected AST dump")
	}
}

func TestParseCallModeDirectives(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/modes\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `package modes

// fgb(sync)
func Sync(value int) int { return value }

// fgb(async)
func Async(value int) int { return value }

// fgb:mode=sync
func Explicit(value int) int { return value }
`
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Callables) != 3 {
		t.Fatalf("got %d callables, want 3", len(api.Callables))
	}
	if api.Callables[0].Mode != model.CallModeSync || api.Callables[1].Mode != model.CallModeAsync || api.Callables[2].Mode != model.CallModeSync {
		t.Fatalf("unexpected call modes: %#v", api.Callables)
	}
}

func TestParseRejectsInvalidCompactCallMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/invalid-mode\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte("package api\n\n// fgb(background)\nfunc Work() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(Options{Input: dir, BaseDir: dir}); err == nil {
		t.Fatal("expected invalid fgb mode to be rejected")
	}
}

func TestParseKeepsCgoSourceAndMirrorsItsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/cgo-source\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "api.go")
	source := "package api\n\nimport \"C\"\n\n// fgb(sync)\nfunc Greeting(value string) string { return value }\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Callables) != 1 || api.Callables[0].Func.Name() != "Greeting" {
		t.Fatalf("cgo source API was discarded: %#v", api.Callables)
	}
	if len(api.SourceFiles) != 1 || !sameFilePath(api.SourceFiles[0], sourcePath) {
		t.Fatalf("got source files %#v, want %q", api.SourceFiles, sourcePath)
	}
	if api.InputDir != dir {
		t.Fatalf("got input dir %q, want %q", api.InputDir, dir)
	}
}
