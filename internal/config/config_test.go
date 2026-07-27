package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeStrictRejectsUnknownField(t *testing.T) {
	_, err := decodeStrict([]byte("go_input: api\nmisspelled: true\n"))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestLoadAutoFromPubspec(t *testing.T) {
	dir := t.TempDir()
	pubspec := "name: demo\nflutter_go_bridge:\n  go_input: go/api\n  dart_format: false\n"
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte(pubspec), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAuto(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GoInput == nil || *loaded.GoInput != "go/api" {
		t.Fatalf("unexpected go_input: %#v", loaded.GoInput)
	}
	if loaded.DartFormat == nil || *loaded.DartFormat {
		t.Fatalf("unexpected dart_format: %#v", loaded.DartFormat)
	}
}

func TestResolveDefaults(t *testing.T) {
	dir := t.TempDir()
	input := "example.com/demo/api"
	resolved, err := Resolve(Config{GoInput: &input}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GoInput != input {
		t.Fatalf("got %q, want package pattern %q", resolved.GoInput, input)
	}
	if resolved.GoOutput != filepath.Join(dir, "bridge_generated.go") {
		t.Fatalf("unexpected go output %q", resolved.GoOutput)
	}
	if resolved.DartOutput != filepath.Join(dir, "lib", "src", "bridge_generated.dart") {
		t.Fatalf("unexpected dart output %q", resolved.DartOutput)
	}
	if !resolved.DartFormat || !resolved.StopOnError {
		t.Fatal("expected opinionated defaults")
	}
}

func TestResolveLibraryNameFromPubspec(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte("name: mihomoui\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := "go/api"
	resolved, err := Resolve(Config{GoInput: &input}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.LibraryName != "go_lib_mihomoui" {
		t.Fatalf("got library name %q, want go_lib_<pubspec name>", resolved.LibraryName)
	}
	if resolved.DartOutput != filepath.Join(dir, "lib", "src", "bridge_generated.dart") {
		t.Fatalf("got Dart output %q, want lib/src", resolved.DartOutput)
	}
}

func TestResolveDoesNotDuplicateGoLibraryPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte("name: go_lib_mihomoui\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := "go/api"
	resolved, err := Resolve(Config{GoInput: &input}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.LibraryName != "go_lib_mihomoui" {
		t.Fatalf("got library name %q, want a single go_lib_ prefix", resolved.LibraryName)
	}
}

func TestLoadAutoNotFound(t *testing.T) {
	_, err := LoadAuto(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestResolveDefaultsBesideNestedGoModule(t *testing.T) {
	dir := t.TempDir()
	goDir := filepath.Join(dir, "go")
	if err := os.MkdirAll(filepath.Join(goDir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := "go/api"
	resolved, err := Resolve(Config{GoInput: &input}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GoOutput != filepath.Join(goDir, "bridge_generated.go") {
		t.Fatalf("got %q, want bridge beside nested go.mod", resolved.GoOutput)
	}
}
