package integrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	templatefs "github.com/star4277/flutter_go_bridge/template"
)

// The existing creator_test.go and integrate_test.go already cover the
// `create`/`run` internals. These tests cover the thin public wrappers and a
// few command helpers that nothing else drives.

func TestRunPublicWrapper(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "include: package:flutter_lints/flutter.yaml\n")
	writeFile(t, filepath.Join(dir, "lib", "main.dart"), "void main() {}\n")

	if err := Run(baseConfig(dir, TemplateApp)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go", "go.mod")); err != nil {
		t.Fatalf("Run wrapper did not integrate: %v", err)
	}
}

func TestCreatePublicWrapper(t *testing.T) {
	calls := stubCreateCommands(t, makeAppScaffold)
	workDir := t.TempDir()
	err := Create(CreateConfig{
		Name:      "myproj",
		WorkDir:   workDir,
		Template:  TemplateApp,
		Platforms: "android,ios,windows",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = calls
	dartRoot := filepath.Join(workDir, "myproj")
	if _, err := os.Stat(filepath.Join(dartRoot, "go", "go.mod")); err != nil {
		t.Fatalf("Create wrapper did not scaffold+integrate: %v", err)
	}
}

func TestCreatePublicWrapperPlugin(t *testing.T) {
	calls := stubCreateCommands(t, makePluginScaffold)
	workDir := t.TempDir()
	err := Create(CreateConfig{
		Name:      "plugproj",
		WorkDir:   workDir,
		Template:  TemplatePlugin,
		Platforms: "linux,windows",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = calls
	dartRoot := filepath.Join(workDir, "plugproj")
	if _, err := os.Stat(filepath.Join(dartRoot, "go", "go.mod")); err != nil {
		t.Fatalf("Create(plugin) wrapper did not integrate: %v", err)
	}
}

func TestCreateUsesEmbeddedTemplates(t *testing.T) {
	calls := stubCreateCommands(t, makeAppScaffold)
	workDir := t.TempDir()
	if err := create(CreateConfig{Name: "reald", WorkDir: workDir, Template: TemplateApp, Platforms: "android,ios,windows"}, templatefs.FS); err != nil {
		t.Fatal(err)
	}
	_ = calls
	if _, err := os.Stat(filepath.Join(workDir, "reald", "go", "go.mod")); err != nil {
		t.Fatalf("create with real templates failed: %v", err)
	}
}

func TestFindDartExecutableFallback(t *testing.T) {
	// In the test environment `dart` is typically absent, so the helper
	// returns the literal "dart"; the SDK-binary rewiring branch is hard to
	// reach portably but the fallback is what most environments use.
	got := FindDartExecutable()
	if got == "" {
		t.Fatal("FindDartExecutable must never be empty")
	}
}

func TestFlutterCreateArgForms(t *testing.T) {
	calls := stubCommands(t)
	if err := flutterCreate("dir", "appname", "com.example", TemplateApp, "android,ios"); err != nil {
		t.Fatal(err)
	}
	if err := flutterCreate("dir", "plug", "", TemplatePlugin, "linux"); err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, call := range *calls {
		seen = append(seen, call.String())
	}
	joined := strings.Join(seen, "\n")
	if !strings.Contains(joined, "create appname --org com.example --template app --platforms android,ios") {
		t.Fatalf("app create args wrong:\n%s", joined)
	}
	if !strings.Contains(joined, "create plug --template plugin_ffi --platforms linux") {
		t.Fatalf("plugin create args wrong:\n%s", joined)
	}
}

func TestResolveFlutterPlatformsExplicit(t *testing.T) {
	stubCommands(t)
	if got := resolveFlutterPlatforms(TemplateApp, "web,ohos"); got != "web,ohos" {
		t.Fatalf("explicit platforms should pass through, got %q", got)
	}
	if got := resolveFlutterPlatforms(TemplatePlugin, "  "); got != "android,ios,linux,macos,windows" {
		t.Fatalf("plugin default wrong: %q", got)
	}
}

func TestFlutterCreateHelpSupportsOhos(t *testing.T) {
	help := `Usage: flutter create

Options:
  --platforms  The platforms supported by this project.
               Platforms available: android ios linux macos windows web ohos
`
	if !flutterCreateHelpSupportsPlatform(help, "ohos") {
		t.Fatal("ohos should be detected in help")
	}
	if flutterCreateHelpSupportsPlatform("--platforms android ios", "ohos") {
		t.Fatal("ohos should not be detected absent the option")
	}
}

func TestModifyPermissions(t *testing.T) {
	dir := t.TempDir()
	gokit := filepath.Join(dir, "gokit")
	if err := os.MkdirAll(gokit, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build_pod.sh", "run_build_tool.sh", "run_build_tool.cmd"} {
		writeFile(t, filepath.Join(gokit, name), "#!/bin/sh\n")
	}
	if err := modifyPermissions(dir, TemplateApp); err != nil {
		t.Fatal(err)
	}
}

func TestFindDartPackageDirAndReadNameErrors(t *testing.T) {
	// findDartPackageDir outside any pubspec returns an error.
	if _, err := findDartPackageDir(filepath.Join(t.TempDir(), "no", "package")); err == nil {
		t.Fatal("expected an error outside a dart package")
	}
}
