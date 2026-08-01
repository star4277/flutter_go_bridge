package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/model"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
)

// writeFixtureModule writes a module dir with api/api.go containing source.
func writeFixtureModule(t *testing.T, dir, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func parseFixtureAPI(t *testing.T, dir, input string) *model.API {
	t.Helper()
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: input, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func TestGenerateGoOutputExtension(t *testing.T) {
	dir := t.TempDir()
	writeFixtureModule(t, dir, "package api\n\nfunc F() int { return 1 }\n")
	api := parseFixtureAPI(t, dir, filepath.Join(dir, "api"))
	_, err := Generate(api, config.Resolved{GoOutput: filepath.Join(dir, "out.txt")})
	if err == nil || !strings.Contains(err.Error(), "go_output must be a .go file") {
		t.Fatalf("expected extension error, got %v", err)
	}
}

func TestGenerateDirectOutputInsideInput(t *testing.T) {
	dir := t.TempDir()
	writeFixtureModule(t, dir, "package api\n\nfunc F() int { return 1 }\n")
	inputDir := filepath.Join(dir, "api")
	api := parseFixtureAPI(t, dir, inputDir)
	_, err := Generate(api, config.Resolved{GoOutput: filepath.Join(inputDir, "bridge_generated.go")})
	if err == nil || !strings.Contains(err.Error(), "go_output is inside input package") {
		t.Fatalf("expected inside-input error, got %v", err)
	}
}

func TestGenerateMainPackageNotDirect(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mainmod\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n\nfunc Main() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: mainPath, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Generate(api, config.Resolved{GoOutput: filepath.Join(dir, "bridge", "g.go")})
	if err == nil || !strings.Contains(err.Error(), "package main cannot be imported") {
		t.Fatalf("expected main-not-direct error, got %v", err)
	}
}

func TestGenerateMainPackageDirect(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mainmod\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n\nfunc Main() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: mainPath, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Generate(api, config.Resolved{
		GoOutput: filepath.Join(dir, "bridge_generated.go"),
		GoInput:  mainPath,
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
