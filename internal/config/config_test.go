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
	if resolved.Target != TargetNative {
		t.Fatalf("default target = %q, want native", resolved.Target)
	}
}

func TestResolveWebTarget(t *testing.T) {
	dir := t.TempDir()
	input, target := "go/api", " WEB "
	resolved, err := Resolve(Config{GoInput: &input, Target: &target}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Target != TargetWeb {
		t.Fatalf("target = %q, want web", resolved.Target)
	}
}

func TestResolveRejectsUnknownTarget(t *testing.T) {
	input, target := "go/api", "wasm32"
	if _, err := Resolve(Config{GoInput: &input, Target: &target}, t.TempDir()); err == nil {
		t.Fatal("expected invalid target error")
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

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }

func TestLoadExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(path, []byte("go_input: go/api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadExplicit(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GoInput == nil || *cfg.GoInput != "go/api" {
		t.Fatalf("unexpected go_input %#v", cfg.GoInput)
	}
	if cfg.BaseDir == nil || *cfg.BaseDir != filepath.Clean(dir) {
		t.Fatalf("unexpected base dir %#v", cfg.BaseDir)
	}
}

func TestLoadExplicitErrors(t *testing.T) {
	if _, err := LoadExplicit(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("go_input: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadExplicit(bad); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadAutoConfigFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range autoConfigNames[:1] {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("go_input: go/api\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		break
	}
	cfg, err := LoadAuto(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GoInput == nil || *cfg.GoInput != "go/api" {
		t.Fatalf("unexpected go_input %#v", cfg.GoInput)
	}
}

func TestLoadAutoPubspecMissingSection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte("name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuto(dir); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestLoadAutoPubspecBadYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte(":\n  : bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuto(dir); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestResolveOverrides(t *testing.T) {
	dir := t.TempDir()
	input := "go/api"
	goOut := "gen/custom.go"
	dartOut := "lib/custom.dart"
	libName := "custom_lib"
	class := "MyBridge"
	lineLen := 120
	preamble := "// dart"
	goPreamble := "// go"
	dartFormat := false
	stopOnError := false
	cfg := Config{
		BaseDir:                 strPtr(dir),
		GoInput:                 &input,
		GoOutput:                &goOut,
		DartOutput:              &dartOut,
		LibraryName:             &libName,
		DartEntrypointClassName: &class,
		DartFormatLineLength:    &lineLen,
		DartPreamble:            &preamble,
		GoPreamble:              &goPreamble,
		DartFormat:              &dartFormat,
		StopOnError:             &stopOnError,
	}
	res, err := Resolve(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.BaseDir != filepath.Clean(dir) {
		t.Fatalf("base = %q", res.BaseDir)
	}
	if res.GoOutput != filepath.Join(dir, goOut) {
		t.Fatalf("go out = %q", res.GoOutput)
	}
	if res.DartOutput != resolvePath(dir, dartOut) {
		t.Fatalf("dart out = %q", res.DartOutput)
	}
	if res.LibraryName != libName {
		t.Fatalf("lib = %q", res.LibraryName)
	}
	if res.DartEntrypointClassName != class {
		t.Fatalf("class = %q", res.DartEntrypointClassName)
	}
	if res.DartFormatLineLength != lineLen {
		t.Fatalf("line len = %d", res.DartFormatLineLength)
	}
	if res.DartPreamble != preamble || res.GoPreamble != goPreamble {
		t.Fatalf("preambles %q %q", res.DartPreamble, res.GoPreamble)
	}
	if res.DartFormat != dartFormat || res.StopOnError != stopOnError {
		t.Fatalf("flags %v %v", res.DartFormat, res.StopOnError)
	}
}

func TestResolveAbsoluteOutputs(t *testing.T) {
	dir := t.TempDir()
	input := "go/api"
	absOut := filepath.Join(dir, "abs", "bridge.go")
	cfg := Config{GoInput: &input, GoOutput: &absOut, DartOutput: &absOut}
	res, err := Resolve(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.GoOutput != filepath.Clean(absOut) {
		t.Fatalf("go out = %q", res.GoOutput)
	}
	if res.DartOutput != filepath.Clean(absOut) {
		t.Fatalf("dart out = %q", res.DartOutput)
	}
}

func TestResolveErrors(t *testing.T) {
	if _, err := Resolve(Config{}, t.TempDir()); err == nil {
		t.Fatal("expected go_input required error")
	}
	empty := ""
	if _, err := Resolve(Config{GoInput: &empty}, t.TempDir()); err == nil {
		t.Fatal("expected go_input required error for empty")
	}
	input := "go/api"
	badLen := 30
	if _, err := Resolve(Config{GoInput: &input, DartFormatLineLength: &badLen}, t.TempDir()); err == nil {
		t.Fatal("expected line length error")
	}
}

func TestGoLibraryName(t *testing.T) {
	if got := GoLibraryName("demo"); got != "go_lib_demo" {
		t.Fatalf("got %q", got)
	}
	if got := GoLibraryName("go_lib_demo"); got != "go_lib_demo" {
		t.Fatalf("got %q", got)
	}
	if got := GoLibraryName("   demo  "); got != "go_lib_demo" {
		t.Fatalf("trim got %q", got)
	}
}

func TestResolvePathAndLocal(t *testing.T) {
	dir := t.TempDir()
	if got := resolvePath(dir, filepath.Join(dir, "x.go")); got != filepath.Clean(filepath.Join(dir, "x.go")) {
		t.Fatalf("abs = %q", got)
	}
	if got := resolvePath(dir, "rel.go"); got != filepath.Join(dir, "rel.go") {
		t.Fatalf("rel = %q", got)
	}
	if got := resolvePathIfLocal(dir, filepath.Join(dir, "x")); got != filepath.Clean(filepath.Join(dir, "x")) {
		t.Fatalf("abs local = %q", got)
	}
	// relative path that exists
	ex := filepath.Join(dir, "exists")
	if err := os.MkdirAll(ex, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolvePathIfLocal(dir, "exists"); got != ex {
		t.Fatalf("existing rel = %q", got)
	}
	// relative path that does not exist
	if got := resolvePathIfLocal(dir, "missingdir"); got != "missingdir" {
		t.Fatalf("missing rel = %q", got)
	}
}

func TestResolveDefaultsGoModuleRootBranches(t *testing.T) {
	dir := t.TempDir()
	// package-pattern input -> falls back to baseDir (no go.mod anywhere)
	if got := defaultGoModuleRoot(dir, "example.com/demo/api"); got != dir {
		t.Fatalf("pattern fallback = %q", got)
	}
	// input is a file path
	goDir := filepath.Join(dir, "go")
	apiDir := filepath.Join(goDir, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := defaultGoModuleRoot(dir, filepath.Join(apiDir, "api.go")); got != goDir {
		t.Fatalf("file input root = %q", got)
	}
	if got := defaultGoModuleRoot(dir, "go/api"); got != goDir {
		t.Fatalf("dir input root = %q", got)
	}
}

func TestMergePrecedence(t *testing.T) {
	hi := Config{Target: strPtr("web"), GoInput: strPtr("hi"), DartFormat: boolPtr(false)}
	lo := Config{Target: strPtr("native"), GoInput: strPtr("lo"), DartFormat: boolPtr(true), LibraryName: strPtr("low")}
	merged := Merge(hi, lo)
	if merged.Target == nil || *merged.Target != "web" {
		t.Fatalf("high target should win, got %#v", merged.Target)
	}
	if merged.GoInput == nil || *merged.GoInput != "hi" {
		t.Fatalf("high go_input should win, got %#v", merged.GoInput)
	}
	if merged.DartFormat == nil || *merged.DartFormat != false {
		t.Fatalf("high dart_format should win, got %#v", merged.DartFormat)
	}
	if merged.LibraryName == nil || *merged.LibraryName != "low" {
		t.Fatalf("low library_name should fill, got %#v", merged.LibraryName)
	}
	if merged.BaseDir != nil {
		t.Fatalf("unexpected base dir %#v", merged.BaseDir)
	}
	if got := first[int](); got != nil {
		t.Fatalf("first of none should be nil")
	}
}

func TestPubspecNameErrorPaths(t *testing.T) {
	if got := pubspecName(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("missing base should be empty, got %q", got)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte(":\n  bad: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pubspecName(dir); got != "" {
		t.Fatalf("bad yaml should be empty, got %q", got)
	}
}

func TestLoadAutoReadErrorNonNotExist(t *testing.T) {
	dir := t.TempDir()
	// Make pubspec.yaml a directory so os.ReadFile fails with a non-NotExist error.
	if err := os.MkdirAll(filepath.Join(dir, "pubspec.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuto(dir); err == nil {
		t.Fatal("expected ReadFile error")
	}
}

func TestLoadAutoPubspecSectionDecodeError(t *testing.T) {
	dir := t.TempDir()
	pubspec := "name: demo\nflutter_go_bridge:\n  unknown_option: true\n"
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte(pubspec), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuto(dir); err == nil {
		t.Fatal("expected decode error for unknown option")
	}
}

func TestDefaultGoModuleRootFileInput(t *testing.T) {
	dir := t.TempDir()
	apiDir := filepath.Join(dir, "go", "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "api.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go", "go.mod"), []byte("module m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := defaultGoModuleRoot(dir, filepath.Join(apiDir, "api.go")); got != filepath.Join(dir, "go") {
		t.Fatalf("file input root = %q, want %q", got, filepath.Join(dir, "go"))
	}
}

func TestDefaultGoModuleRootFileAsParent(t *testing.T) {
	dir := t.TempDir()
	// Pathological input: the parent component of the (nonexistent) path is a file.
	f := filepath.Join(dir, "blocked.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nonexistent := filepath.Join(f, "sub")
	if got := defaultGoModuleRoot(dir, nonexistent); got != dir {
		t.Fatalf("file-as-parent root = %q, want %q", got, dir)
	}
}
