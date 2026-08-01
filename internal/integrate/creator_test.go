package integrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubCreateCommands stubs the runners like stubCommands, but additionally
// simulates `flutter create` by materializing a minimal scaffold.
func stubCreateCommands(t *testing.T, scaffold func(t *testing.T, projectDir string)) *[]commandCall {
	t.Helper()
	calls := stubCommands(t)
	recorder := runCommand
	runCommand = func(dir, name string, args ...string) error {
		if name == "flutter" && len(args) >= 2 && args[0] == "create" {
			scaffold(t, filepath.Join(dir, args[1]))
		}
		return recorder(dir, name, args...)
	}
	return calls
}

func makeAppScaffold(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: "+filepath.Base(dir)+"\n")
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "include: package:flutter_lints/flutter.yaml\n")
	writeFile(t, filepath.Join(dir, "lib", "main.dart"), "void main() {} // flutter demo\n")
	writeFile(t, filepath.Join(dir, "test", "widget_test.dart"), "void main() {}\n")
}

func makePluginScaffold(t *testing.T, dir string) {
	t.Helper()
	name := filepath.Base(dir)
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: "+name+"\n")
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "include: package:flutter_lints/flutter.yaml\n")
	writeFile(t, filepath.Join(dir, "lib", name+".dart"), "// old plugin api\n")
	writeFile(t, filepath.Join(dir, "lib", name+"_bindings_generated.dart"), "// ffigen output\n")
	writeFile(t, filepath.Join(dir, "src", name+".c"), "int sum(int a, int b) { return a + b; }\n")
	writeFile(t, filepath.Join(dir, "src", "CMakeLists.txt"), "# scaffold\n")
	writeFile(t, filepath.Join(dir, "ffigen.yaml"), "name: bindings\n")
	writeFile(t, filepath.Join(dir, "android", "build.gradle"), "// scaffold\n")
	writeFile(t, filepath.Join(dir, "android", "src", "main", "AndroidManifest.xml"), "<manifest/>\n")
	writeFile(t, filepath.Join(dir, "ios", "Classes", name+".c"), "// forwarder\n")
	writeFile(t, filepath.Join(dir, "ios", name+".podspec"), "# scaffold\n")
	writeFile(t, filepath.Join(dir, "linux", "CMakeLists.txt"), "# scaffold\n")
	writeFile(t, filepath.Join(dir, "macos", "Classes", name+".swift"), "// scaffold\n")
	writeFile(t, filepath.Join(dir, "macos", name+".podspec"), "# scaffold\n")
	writeFile(t, filepath.Join(dir, "windows", "CMakeLists.txt"), "# scaffold\n")
	writeFile(t, filepath.Join(dir, "example", "pubspec.yaml"), "name: "+name+"_example\n")
	writeFile(t, filepath.Join(dir, "example", "lib", "main.dart"), "// flutter demo example\n")
}

func TestCreateAppScaffoldsAndIntegrates(t *testing.T) {
	calls := stubCreateCommands(t, makeAppScaffold)
	workDir := t.TempDir()

	err := create(CreateConfig{
		Name:      "myapp",
		WorkDir:   workDir,
		Template:  TemplateApp,
		Platforms: "android,ios,windows",
	}, fakeTemplates())
	if err != nil {
		t.Fatal(err)
	}
	dartRoot := filepath.Join(workDir, "myapp")

	// The scaffold entrypoint was removed first, so the template is written
	// fresh instead of being appended below a commented-out original.
	mainDart := readFile(t, filepath.Join(dartRoot, "lib", "main.dart"))
	if strings.Contains(mainDart, "temporarily commented out") || strings.Contains(mainDart, "flutter demo") {
		t.Fatalf("create must write a fresh main.dart: %q", mainDart)
	}
	if !strings.Contains(mainDart, "package:myapp/src/bridge_generated.dart") {
		t.Fatalf("main.dart should be the template: %q", mainDart)
	}

	entries, err := os.ReadDir(filepath.Join(dartRoot, "test"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("test/ should exist and be empty, entries=%v err=%v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(dartRoot, "go", "go.mod")); err != nil {
		t.Fatalf("go module missing: %v", err)
	}

	var flutterCreateCall *commandCall
	for i, call := range *calls {
		if call.name == "flutter" && len(call.args) > 0 && call.args[0] == "create" {
			flutterCreateCall = &(*calls)[i]
			break
		}
	}
	if flutterCreateCall == nil {
		t.Fatalf("flutter create was not invoked: %v", *calls)
	}
	joined := flutterCreateCall.String()
	if !strings.Contains(joined, "--template app") || !strings.Contains(joined, "--platforms android,ios,windows") {
		t.Fatalf("unexpected flutter create call: %s", joined)
	}
	if flutterCreateCall.dir != workDir {
		t.Fatalf("flutter create ran in %s, want %s", flutterCreateCall.dir, workDir)
	}
}

func TestCreatePluginCleansScaffoldAndIntegrates(t *testing.T) {
	calls := stubCreateCommands(t, makePluginScaffold)
	workDir := t.TempDir()

	err := create(CreateConfig{
		Name:      "my_plugin",
		WorkDir:   workDir,
		Template:  TemplatePlugin,
		Platforms: "android,ios,windows",
	}, fakeTemplates())
	if err != nil {
		t.Fatal(err)
	}
	dartRoot := filepath.Join(workDir, "my_plugin")

	for _, gone := range []string{
		filepath.Join(dartRoot, "src"),
		filepath.Join(dartRoot, "ffigen.yaml"),
		filepath.Join(dartRoot, "lib", "my_plugin_bindings_generated.dart"),
	} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s should be removed, got err=%v", gone, err)
		}
	}

	pluginDart := readFile(t, filepath.Join(dartRoot, "lib", "my_plugin.dart"))
	if strings.Contains(pluginDart, "old plugin api") || strings.Contains(pluginDart, "temporarily commented out") {
		t.Fatalf("plugin entrypoint must be written fresh: %q", pluginDart)
	}
	if !strings.Contains(pluginDart, "library my_plugin;") {
		t.Fatalf("plugin entrypoint should be the template: %q", pluginDart)
	}

	windowsCMake := readFile(t, filepath.Join(dartRoot, "windows", "CMakeLists.txt"))
	if !strings.Contains(windowsCMake, "apply_gokit") {
		t.Fatalf("windows CMakeLists should come from the gokit template: %q", windowsCMake)
	}

	exampleMain := readFile(t, filepath.Join(dartRoot, "example", "lib", "main.dart"))
	if !strings.Contains(exampleMain, "FlutterGoBridge.initialize") || strings.Contains(exampleMain, "flutter demo example") {
		t.Fatalf("example main.dart should be the template: %q", exampleMain)
	}

	exampleAdd := false
	for _, call := range *calls {
		if call.dir == filepath.Join(dartRoot, "example") && strings.Contains(call.String(), "integration_test") {
			exampleAdd = true
		}
	}
	if !exampleAdd {
		t.Fatalf("integration_test should be added to example/: %v", *calls)
	}
}

func TestCreateRefusesExistingTarget(t *testing.T) {
	stubCommands(t)
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "taken"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := create(CreateConfig{Name: "taken", WorkDir: workDir, Platforms: "android"}, fakeTemplates())
	if err == nil || !strings.Contains(err.Error(), "integrate") {
		t.Fatalf("expected already-exists error suggesting integrate, got %v", err)
	}
}

func TestRemoveFilesInDirPreservesNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "top.txt"), "x")
	writeFile(t, filepath.Join(dir, "nested", "child.txt"), "x")

	if err := removeFilesInDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "top.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("top-level file should be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "child.txt")); err != nil {
		t.Fatalf("nested content should be preserved: %v", err)
	}
}

func TestRemoveUnnecessaryPluginFilesCleansPlatformLayout(t *testing.T) {
	dir := t.TempDir()
	makePluginScaffold(t, dir)
	writeFile(t, filepath.Join(dir, "ohos", "src", "main", "module.json5"), "{}\n")

	if err := removeUnnecessaryPluginFiles(dir); err != nil {
		t.Fatal(err)
	}

	for _, gone := range []string{
		filepath.Join(dir, "src"),
		filepath.Join(dir, "ffigen.yaml"),
		filepath.Join(dir, "android", "src"),
		filepath.Join(dir, "ios", "Classes"),
		filepath.Join(dir, "macos", "Classes"),
		filepath.Join(dir, "ohos"),
	} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s should be removed, got err=%v", gone, err)
		}
	}
	// Platform dirs stay (emptied) so the templates can repopulate them.
	for _, kept := range []string{"android", "ios", "linux", "macos", "windows"} {
		info, err := os.Stat(filepath.Join(dir, kept))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s should remain a directory, err=%v", kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pubspec.yaml")); err != nil {
		t.Fatalf("pubspec.yaml must survive: %v", err)
	}
}
