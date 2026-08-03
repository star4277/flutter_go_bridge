package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
)

// dartTopLevelTypeDeclaration matches the generated top-level type declarations
// whose names could shadow an ambient Dart type.
var dartTopLevelTypeDeclaration = regexp.MustCompile(
	`(?m)^(?:final class|abstract interface class|abstract base class|class|extension type const) (\w+)`)

func declaredDartTypeNames(source string) []string {
	var declared []string
	for _, match := range dartTopLevelTypeDeclaration.FindAllStringSubmatch(source, -1) {
		declared = append(declared, match[1])
	}
	return declared
}

// generateAllDartFiles runs the full pipeline and returns every generated Dart
// file by base name. Synthetic types land in their own file, so a test that
// only reads api.dart and bridge_generated.dart would miss them.
func generateAllDartFiles(t *testing.T, source string) map[string]string {
	t.Helper()
	dir := t.TempDir()
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
	resolved := config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "dart", "bridge_generated.dart"),
		LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	}
	if _, err := WriteSupportPackage(resolved); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, resolved)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, path := range result.Files {
		if filepath.Ext(path) != ".dart" {
			continue
		}
		files[filepath.Base(path)] = mustRead(t, path)
	}
	return files
}

// A Go type named List or Duration used to be emitted as `final class List` in
// the same library that also writes `List<String>` and `Duration(microseconds:)`.
// dart:core needs no import, so the generated class shadowed the core type and
// the library could not compile.
func TestGenerateRenamesTypesShadowingAmbientDartTypes(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

import "time"

type List struct{ Size int }

type Duration struct{ Ticks int }

type Holder struct {
	Names   []string
	Elapsed time.Duration
	Tagged  List
	Span    Duration
}

func Make(h Holder) Holder { return h }
`)
	if err != nil {
		t.Fatal(err)
	}

	for _, declared := range declaredDartTypeNames(apiDart) {
		if declared == "List" || declared == "Duration" {
			t.Fatalf("generated a top-level %q, which shadows the ambient Dart type:\n%s", declared, apiDart)
		}
	}
	for _, expected := range []string{"final class GoList {", "final class GoDuration {"} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing renamed declaration %q:\n%s", expected, apiDart)
		}
	}
	// The ambient meanings must survive for the fields that need them.
	for _, expected := range []string{"final List<String> names;", "final Duration elapsed;"} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("ambient Dart type no longer reachable, missing %q:\n%s", expected, apiDart)
		}
	}
	// And the renamed classes must actually be used by the fields that hold them.
	for _, expected := range []string{"final GoList tagged;", "final GoDuration span;"} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("renamed class not referenced, missing %q:\n%s", expected, apiDart)
		}
	}

	renameWarnings := 0
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "would shadow the ambient Dart type") {
			renameWarnings++
		}
	}
	if renameWarnings != 2 {
		t.Fatalf("expected one shadowing warning per renamed type, got %d: %v", renameWarnings, warnings)
	}
}

// The dependency-interface fallback names its Dart class after the Go type, so
// `error` produced `abstract interface class Error`, shadowing dart:core.Error
// under a name the user has no way to rename.
func TestGenerateRenamesDependencyInterfaceShadowingAmbientDartType(t *testing.T) {
	files := generateAllDartFiles(t, `package api

type Box struct {
	Name string
	Err  error
}

func Make() Box { return Box{} }
`)
	ambient := map[string]bool{"Error": true, "Exception": true, "StackTrace": true}
	joined := make([]string, 0, len(files))
	for name, content := range files {
		for _, declared := range declaredDartTypeNames(content) {
			if ambient[declared] {
				t.Fatalf("%s declares %q, which shadows the ambient Dart type:\n%s", name, declared, content)
			}
		}
		joined = append(joined, content)
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "abstract interface class GoError") {
		t.Fatalf("expected the error interface to be renamed:\n%s", all)
	}
	if !strings.Contains(files["api.dart"], "final GoError err;") {
		t.Fatalf("field should use the renamed interface:\n%s", files["api.dart"])
	}
}

// An explicit //fgb:rename to an ambient name must not defeat the guard: the
// generated library would still fail to compile.
func TestGenerateRenamesExplicitRenameShadowingAmbientDartType(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

//fgb:rename = "Duration"
type Span struct{ Ticks int }

func Make(s Span) Span { return s }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, declared := range declaredDartTypeNames(apiDart) {
		if declared == "Duration" {
			t.Fatalf("an explicit rename must not shadow the ambient Dart type:\n%s", apiDart)
		}
	}
	if !strings.Contains(apiDart, "final class GoDuration {") {
		t.Fatalf("expected the explicit rename to be adjusted:\n%s", apiDart)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "would shadow the ambient Dart type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a shadowing warning, got %v", warnings)
	}
}
