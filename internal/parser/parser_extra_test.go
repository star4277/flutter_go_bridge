package parser

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/model"
)

func TestSourcePositionLessOrdersFilesBeforeTokenBases(t *testing.T) {
	if !sourcePositionLess(`C:\fixture\a.go`, token.Pos(100), `C:\fixture\z.go`, token.Pos(1)) {
		t.Fatal("source filename must determine cross-file declaration order")
	}
	if sourcePositionLess(`C:\fixture\z.go`, token.Pos(1), `C:\fixture\a.go`, token.Pos(100)) {
		t.Fatal("later source filename must not sort before an earlier one")
	}
	if !sourcePositionLess(`C:\fixture\a.go`, token.Pos(1), `C:\fixture\a.go`, token.Pos(2)) {
		t.Fatal("token position must determine declaration order within one source file")
	}
}

func TestParseInterfaceMethodDirectives(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/interfaces\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `package interfaces


type Loader interface {
	//fgb:async, rename = "retrieve"
	Load(value string) string
	//fgb:ignore
	Hidden() int
	Plain() int
}

type Needs interface {
	Other
}

type Other interface {
	Embedded() int
}
`
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
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
	if loader == nil || loader.Methods == nil {
		t.Fatalf("interface methods not collected: %#v", loader)
	}
	load := loader.Methods["Load"]
	if load == nil || load.DartName != "retrieve" || load.Mode != model.CallModeAsync || load.Ignore {
		t.Fatalf("Load directive not honored: %#v", load)
	}
	if hidden := loader.Methods["Hidden"]; hidden == nil || !hidden.Ignore {
		t.Fatalf("Hidden directive not honored: %#v", hidden)
	}
	if plain := loader.Methods["Plain"]; plain == nil || plain.DartName != "plain" || plain.Mode != model.CallModeSync {
		t.Fatalf("Plain defaults not applied: %#v", plain)
	}
	// Needs embeds Other; embedded methods are not copied.
	for _, decl := range api.Types {
		if decl.Object.Name() == "Needs" && decl.Methods != nil && len(decl.Methods) != 0 {
			t.Fatalf("embedded interface should contribute no methods: %#v", decl.Methods)
		}
	}
}

func TestParseTypeLevelRename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/type-rename\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `package tyrename

//fgb:rename = "WidgetCard"
type Card struct {
	Title string
}
`
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, decl := range api.Types {
		if decl.Object.Name() == "Card" && decl.DartName == "WidgetCard" {
			found = true
		}
	}
	if !found {
		t.Fatalf("type-level rename not applied")
	}
}

func TestParseConstantGroupCommentsAndInvalidDirective(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/const-dir\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// invalid fgb directive inside a const group must surface constSpecName in the error
	source := `package constdir

type Kind int

const (
	//fgb:rename = 
	KindA Kind = iota
)
`
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(Options{Input: dir, BaseDir: dir}); err == nil {
		t.Fatal("expected invalid const directive to be rejected")
	}
}

func TestParsePackageCompileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/broken\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte("package broken\n\nfunc F() int { return Undefined }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(Options{Input: dir, BaseDir: dir}); err == nil {
		t.Fatal("expected compile error to be surfaced")
	}
}

func TestParseFileInputTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/filetarget\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(dir, "api.go")
	if err := os.WriteFile(apiPath, []byte("package filetarget\n\nfunc F() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := Parse(Options{Input: apiPath, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Callables) != 1 || api.Callables[0].Func.Name() != "F" {
		t.Fatalf("file input target failed: %#v", api.Callables)
	}
	if !strings.HasSuffix(apiPath, filepath.Base(apiPath)) {
		t.Fatalf("sanity")
	}
}

func TestParseFileTargetNonGoExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/badext\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "api.txt")
	if err := os.WriteFile(bad, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(Options{Input: bad, BaseDir: dir}); err == nil {
		t.Fatal("expected non-go file input to be rejected")
	}
}

func TestParseEmptyInput(t *testing.T) {
	if _, err := Parse(Options{Input: ""}); err == nil {
		t.Fatal("expected empty input error")
	}
}

func TestParseSkippedGeneratedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/genfile\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "// Code generated by tool. DO NOT EDIT.\n\npackage genfile\n\nfunc F() int { return 1 }\n"
	apiPath := filepath.Join(dir, "gen.go")
	if err := os.WriteFile(apiPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := Parse(Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.Callables) != 0 {
		t.Fatalf("generated file should be skipped, got %#v", api.Callables)
	}
	if len(api.SourceFiles) != 0 {
		t.Fatalf("generated source files should be skipped, got %#v", api.SourceFiles)
	}
}
