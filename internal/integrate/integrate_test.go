package integrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

type commandCall struct {
	dir  string
	name string
	args []string
}

func (c commandCall) String() string {
	return c.name + " " + strings.Join(c.args, " ")
}

// stubCommands replaces the flutter/dart runners with recorders so tests never
// spawn external tools.
func stubCommands(t *testing.T) *[]commandCall {
	t.Helper()
	var calls []commandCall
	originalRun := runCommand
	originalCapture := captureCommand
	runCommand = func(dir, name string, args ...string) error {
		calls = append(calls, commandCall{dir: dir, name: name, args: args})
		return nil
	}
	captureCommand = func(string, string, ...string) (string, error) {
		return "", errors.New("captureCommand disabled in tests")
	}
	t.Cleanup(func() {
		runCommand = originalRun
		captureCommand = originalCapture
	})
	return &calls
}

func fakeTemplates() fstest.MapFS {
	file := func(content string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(content)}
	}
	return fstest.MapFS{
		"shared/common/flutter_go_bridge.yaml":                             file("go_input: REPLACE_ME_GO_MOD_DIR/api\nlibrary_name: REPLACE_ME_GO_MOD_NAME\n"),
		"shared/common/REPLACE_ME_GO_MOD_DIR/go.mod.template":              file("module com.flutter_go_bridge/REPLACE_ME_GO_MOD_NAME\n\ngo 1.24.5\n"),
		"shared/common/REPLACE_ME_GO_MOD_DIR/bridge_generated.go.template": file("package main\n\nimport api \"com.flutter_go_bridge/REPLACE_ME_GO_MOD_NAME/api\"\n"),
		"shared/common/REPLACE_ME_GO_MOD_DIR/api/lib.go.template":          file("package api\n"),
		"shared/common/lib/src/bridge_generated.dart":                      file("const libraryName = \"REPLACE_ME_GO_MOD_NAME\";\n"),
		"shared/common/test_driver/integration_test.dart":                  file("void main() {}\n"),
		"shared/app/lib/main.dart":                                         file("import 'package:REPLACE_ME_DART_PACKAGE_NAME/src/bridge_generated.dart';\nvoid main() {}\n"),
		"shared/plugin/lib/REPLACE_ME_DART_PACKAGE_NAME.dart":              file("library REPLACE_ME_DART_PACKAGE_NAME;\n"),
		"app/go_builder/pubspec.yaml":                                      file("name: REPLACE_ME_GO_MOD_NAME\nflutter:\n  plugin:\n    platforms:\n      windows:\n        ffiPlugin: trueREPLACE_ME_OHOS_PLUGIN_PLATFORM_TEXT\n"),
		"app/go_builder/windows/CMakeLists.txt":                            file("apply_gokit(REPLACE_ME_GO_MOD_NAME ../../../../../../REPLACE_ME_GO_MOD_DIR REPLACE_ME_GO_MOD_NAME \"\")\n"),
		"app/go_builder/ohos/module.json5":                                 file("{}\n"),
		"app/go_builder/gokit/run_build_tool.sh":                           file("#!/bin/sh\n"),
		"app/go_builder/gokit/build_tool/pubspec.yaml":                     file("name: build_tool\n"),
		"app/go_builder/gokit/docs/architecture.md":                        file("internal docs\n"),
		"plugin/gokit/run_build_tool.sh":                                   file("#!/bin/sh\n"),
		"plugin/gokit/build_tool/pubspec.yaml":                             file("name: build_tool\n"),
		"plugin/windows/CMakeLists.txt":                                    file("apply_gokit(REPLACE_ME_DART_PACKAGE_NAME ../REPLACE_ME_GO_MOD_DIR REPLACE_ME_GO_MOD_NAME \"\")\n"),
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func baseConfig(dir string, template Template) Config {
	return Config{
		WorkDir:               dir,
		Template:              template,
		Platforms:             "android,ios,windows",
		EnableWriteLib:        true,
		EnableIntegrationTest: true,
		EnableDartFix:         true,
		EnableDartFormat:      true,
	}
}

func TestRunAppOverlaysTemplates(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: myapp\n")
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "include: package:flutter_lints/flutter.yaml\n")
	writeFile(t, filepath.Join(dir, "lib", "main.dart"), "void main() {}\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err != nil {
		t.Fatal(err)
	}

	goMod := readFile(t, filepath.Join(dir, "go", "go.mod"))
	if !strings.Contains(goMod, "module com.flutter_go_bridge/go_lib_myapp") {
		t.Fatalf("go.mod placeholders not replaced: %q", goMod)
	}
	bridgeGo := readFile(t, filepath.Join(dir, "go", "bridge_generated.go"))
	if !strings.Contains(bridgeGo, "api \"com.flutter_go_bridge/go_lib_myapp/api\"") {
		t.Fatalf("bridge_generated.go placeholders not replaced: %q", bridgeGo)
	}
	if _, err := os.Stat(filepath.Join(dir, "go", "api", "lib.go")); err != nil {
		t.Fatalf("api/lib.go not written: %v", err)
	}

	builderPubspec := readFile(t, filepath.Join(dir, "go_builder", "pubspec.yaml"))
	if !strings.Contains(builderPubspec, "name: go_lib_myapp") {
		t.Fatalf("go_builder pubspec name not replaced: %q", builderPubspec)
	}
	if strings.Contains(builderPubspec, "ohos") || strings.Contains(builderPubspec, "REPLACE_ME") {
		t.Fatalf("ohos platform should be dropped without ohos support: %q", builderPubspec)
	}

	mainDart := readFile(t, filepath.Join(dir, "lib", "main.dart"))
	if !strings.HasPrefix(mainDart, "// The original content is temporarily commented out") {
		t.Fatalf("existing main.dart should be commented out: %q", mainDart)
	}
	if !strings.Contains(mainDart, "// void main() {}") || !strings.Contains(mainDart, "package:myapp/src/bridge_generated.dart") {
		t.Fatalf("main.dart should contain commented original and template: %q", mainDart)
	}

	buildToolPubspec := readFile(t, filepath.Join(dir, "go_builder", "gokit", "build_tool", "pubspec.yaml"))
	if !strings.HasPrefix(buildToolPubspec, "# This is copied from Gokit") {
		t.Fatalf("gokit yaml should carry the provenance prelude: %q", buildToolPubspec)
	}
	script := readFile(t, filepath.Join(dir, "go_builder", "gokit", "run_build_tool.sh"))
	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Fatalf("shell scripts must stay untouched: %q", script)
	}

	if _, err := os.Stat(filepath.Join(dir, "go_builder", "gokit", "docs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("gokit docs should be filtered, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go_builder", "ohos")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ohos dir should be filtered, got err=%v", err)
	}

	analysis := readFile(t, filepath.Join(dir, "analysis_options.yaml"))
	if !strings.Contains(analysis, "- go_builder/gokit/**") {
		t.Fatalf("analyzer exclude missing: %q", analysis)
	}

	var flat []string
	for _, call := range *calls {
		flat = append(flat, call.String())
	}
	joined := strings.Join(flat, "\n")
	for _, want := range []string{
		"flutter pub add go_lib_myapp --path=go_builder",
		"flutter pub add integration_test --dev --sdk=flutter",
		"flutter pub get",
		"fix --apply .",
		"format --line-length 80 .",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected a %q command, got:\n%s", want, joined)
		}
	}
	for _, call := range *calls {
		if call.args[0] == "pub" && call.args[1] == "get" {
			want := filepath.Join(dir, "go_builder", "gokit", "build_tool")
			if call.dir != want {
				t.Fatalf("pub get ran in %s, want %s", call.dir, want)
			}
		}
	}
}

func TestRunAppKeepsExistingFilesAndWarns(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: myapp\n")
	writeFile(t, filepath.Join(dir, "lib", "src", "bridge_generated.dart"), "// my custom bridge\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err != nil {
		t.Fatal(err)
	}

	kept := readFile(t, filepath.Join(dir, "lib", "src", "bridge_generated.dart"))
	if kept != "// my custom bridge\n" {
		t.Fatalf("existing non-entrypoint files must not be overwritten: %q", kept)
	}
}

func TestRunAppWithOhosPlatform(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: myapp\n")

	cfg := baseConfig(dir, TemplateApp)
	cfg.Platforms = "android,ohos"
	if err := run(cfg, fakeTemplates()); err != nil {
		t.Fatal(err)
	}

	builderPubspec := readFile(t, filepath.Join(dir, "go_builder", "pubspec.yaml"))
	if !strings.Contains(builderPubspec, "ohos:\n        ffiPlugin: true") {
		t.Fatalf("ohos platform entry missing: %q", builderPubspec)
	}
	if _, err := os.Stat(filepath.Join(dir, "go_builder", "ohos", "module.json5")); err != nil {
		t.Fatalf("ohos template files missing: %v", err)
	}
}

func TestRunPluginDefaultsAndCommentOut(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_plugin\n")
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "include: package:flutter_lints/flutter.yaml\n")
	writeFile(t, filepath.Join(dir, "lib", "my_plugin.dart"), "class MyPlugin {}\n")
	writeFile(t, filepath.Join(dir, "example", "pubspec.yaml"), "name: my_plugin_example\n")

	if err := run(baseConfig(dir, TemplatePlugin), fakeTemplates()); err != nil {
		t.Fatal(err)
	}

	goMod := readFile(t, filepath.Join(dir, "go", "go.mod"))
	if !strings.Contains(goMod, "module com.flutter_go_bridge/my_plugin") {
		t.Fatalf("plugin library name should default to the package name: %q", goMod)
	}

	pluginDart := readFile(t, filepath.Join(dir, "lib", "my_plugin.dart"))
	if !strings.Contains(pluginDart, "// class MyPlugin {}") || !strings.Contains(pluginDart, "library my_plugin;") {
		t.Fatalf("plugin entrypoint should be commented out and replaced: %q", pluginDart)
	}

	analysis := readFile(t, filepath.Join(dir, "analysis_options.yaml"))
	if !strings.Contains(analysis, "- gokit/**") || strings.Contains(analysis, "go_builder") {
		t.Fatalf("plugin analyzer exclude wrong: %q", analysis)
	}

	if _, err := os.Stat(filepath.Join(dir, "gokit", "run_build_tool.sh")); err != nil {
		t.Fatalf("plugin gokit missing: %v", err)
	}

	exampleAdd := false
	for _, call := range *calls {
		if call.dir == filepath.Join(dir, "example") && strings.Contains(call.String(), "integration_test") {
			exampleAdd = true
		}
		if strings.Contains(call.String(), "--path=go_builder") {
			t.Fatalf("plugins must not add a go_builder dependency: %v", call)
		}
	}
	if !exampleAdd {
		t.Fatalf("integration_test should be added to example/, got %v", *calls)
	}
}

func TestRunNoWriteLibSkipsLibraryFiles(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: myapp\n")

	cfg := baseConfig(dir, TemplateApp)
	cfg.EnableWriteLib = false
	cfg.EnableIntegrationTest = false
	if err := run(cfg, fakeTemplates()); err != nil {
		t.Fatal(err)
	}

	for _, missing := range []string{
		filepath.Join(dir, "go"),
		filepath.Join(dir, "lib"),
		filepath.Join(dir, "flutter_go_bridge.yaml"),
		filepath.Join(dir, "test_driver"),
	} {
		if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s should be skipped with --no-write-lib, got err=%v", missing, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "go_builder", "pubspec.yaml")); err != nil {
		t.Fatalf("go_builder must still be written: %v", err)
	}
}

func TestRunFailsOutsideDartPackage(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	err := run(baseConfig(dir, TemplateApp), fakeTemplates())
	if err == nil || !strings.Contains(err.Error(), "pubspec.yaml") {
		t.Fatalf("expected missing pubspec error, got %v", err)
	}
}

func TestRunFailsWithoutGokitTemplates(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: myapp\n")
	templates := fakeTemplates()
	delete(templates, "app/go_builder/gokit/run_build_tool.sh")

	err := run(baseConfig(dir, TemplateApp), templates)
	if err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("expected gokit submodule error, got %v", err)
	}
}

func TestFindDartPackageDirWalksUp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: myapp\n")
	nested := filepath.Join(dir, "lib", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := findDartPackageDir(nested)
	if err != nil {
		t.Fatal(err)
	}
	if found != dir {
		t.Fatalf("got %s, want %s", found, dir)
	}
}

func TestNormalizeGoModDir(t *testing.T) {
	if got, err := normalizeGoModDir(""); err != nil || got != "go" {
		t.Fatalf("default: got %q err=%v", got, err)
	}
	if got, err := normalizeGoModDir("native/go"); err != nil || got != "native/go" {
		t.Fatalf("nested: got %q err=%v", got, err)
	}
	for _, invalid := range []string{"..", "../go", "."} {
		if _, err := normalizeGoModDir(invalid); err == nil {
			t.Fatalf("%q should be rejected", invalid)
		}
	}
}

func TestParseTemplate(t *testing.T) {
	for value, want := range map[string]Template{"": TemplateApp, "app": TemplateApp, "Plugin": TemplatePlugin} {
		got, err := ParseTemplate(value)
		if err != nil || got != want {
			t.Fatalf("ParseTemplate(%q) = %v, %v; want %v", value, got, err, want)
		}
	}
	if _, err := ParseTemplate("library"); err == nil {
		t.Fatal("unknown template should error")
	}
}
