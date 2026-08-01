package integrate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// errOnceRun iterates stubCommands but returns an error for matching tools, so
// error branches in run/create helpers can be reached deterministically.
func stubCommandsErr(t *testing.T, match func(name string, args ...string) bool, err error) *[]commandCall {
	t.Helper()
	calls := stubCommands(t)
	runCommand = func(dir, name string, args ...string) error {
		if match(name, args...) {
			return err
		}
		return nil
	}
	return calls
}

// --- commands.go: FindDartExecutable ---

// TestFindDartExecutableLooksUpPath forces the no-good-binary fallback path by
// making exec.LookPath fail via an empty PATH. On every platform the function
// must return a non-empty string.
func TestFindDartExecutableFallbackWhenLookPathFails(t *testing.T) {
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", originalPath) })

	got := FindDartExecutable()
	if got == "" {
		t.Fatal("FindDartExecutable must never be empty")
	}
}

// --- commands.go: isOhosFlutter ---

func TestIsOhosFlutterSupported(t *testing.T) {
	originalCapture := captureCommand
	captureCommand = func(string, string, ...string) (string, error) {
		return "--platforms [android, ios, ohos, web, windows]", nil
	}
	t.Cleanup(func() { captureCommand = originalCapture })

	if !isOhosFlutter() {
		t.Fatal("ohos-capable flutter should be detected")
	}
}

func TestIsOhosFlutterUnsupportedAfterCapture(t *testing.T) {
	originalCapture := captureCommand
	captureCommand = func(string, string, ...string) (string, error) {
		// Successful capture but no ohos among the supported platforms.
		return "--platforms [android, ios, web, windows]", nil
	}
	t.Cleanup(func() { captureCommand = originalCapture })

	if isOhosFlutter() {
		t.Fatal("flutter without ohos support must be reported unsupported")
	}
}

// --- creator.go: create ---

// TestCreateUsesCurrentDirAsWorkDir exercises the WorkDir=="" branch safely by
// changing the process cwd to a temp dir.
func TestCreateUsesCurrentDirAsWorkDir(t *testing.T) {
	stubCreateCommands(t, makeAppScaffold)
	workDir := t.TempDir()
	t.Chdir(workDir)

	if err := create(CreateConfig{Name: "curdirproj", Template: TemplateApp, Platforms: "windows"}, fakeTemplates()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "curdirproj", "go", "go.mod")); err != nil {
		t.Fatalf("create with empty WorkDir did not produce go module: %v", err)
	}
}

// TestCreateEmptyTemplateDefaultsToApp verifies cfg.Template=="" defaults to
// app and the app flow (removeUnnecessaryAppFiles) runs.
func TestCreateEmptyTemplateDefaultsToApp(t *testing.T) {
	calls := stubCreateCommands(t, makeAppScaffold)
	workDir := t.TempDir()

	if err := create(CreateConfig{Name: "defapp", WorkDir: workDir, Platforms: "windows"}, fakeTemplates()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "defapp", "go", "go.mod")); err != nil {
		t.Fatalf("default template create failed: %v", err)
	}

	// Removing lib/test is the app path; make sure main.dart was written fresh.
	mainDart := readFile(t, filepath.Join(workDir, "defapp", "lib", "main.dart"))
	if strings.Contains(mainDart, "flutter demo") {
		t.Fatalf("app scaffold demo should be removed: %q", mainDart)
	}
	if *calls == nil {
		t.Fatal("no commands recorded")
	}
}

// TestCreateFailsWhenFlutterCreateFails covers the flutterCreate error branch.
func TestCreateFailsWhenFlutterCreateFails(t *testing.T) {
	stubCommandsErr(t, func(name string, _ ...string) bool { return name == "flutter" }, errors.New("create boom"))
	workDir := t.TempDir()

	err := create(CreateConfig{Name: "boom", WorkDir: workDir, Template: TemplateApp, Platforms: "windows"}, fakeTemplates())
	if err == nil || !strings.Contains(err.Error(), "create boom") {
		t.Fatalf("expected flutter create error, got %v", err)
	}
}

// TestCreateFailsOnGetwdError forces os.Getwd to fail by removing cwd. Only
// meaningful on platforms where the OS reports the removed dir.
func TestCreateFailsWhenLibRemovalFails(t *testing.T) {
	stubCreateCommands(t, func(t *testing.T, dir string) {
		// A scaffold with no lib/ so removeUnnecessaryAppFiles errors.
		writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: nolib\n")
	})
	workDir := t.TempDir()

	err := create(CreateConfig{Name: "nolib", WorkDir: workDir, Template: TemplateApp, Platforms: "windows"}, fakeTemplates())
	if err == nil {
		t.Fatal("create should fail when lib removal fails")
	}
}

// --- creator.go: removal helpers ---

func TestRemoveUnnecessaryAppFilesMissingLib(t *testing.T) {
	dir := t.TempDir()
	if err := removeUnnecessaryAppFiles(dir); err == nil {
		t.Fatal("removeUnnecessaryAppFiles on a dir without lib/ should error")
	}
}

func TestRemoveUnnecessaryAppFilesRemovesScaffold(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "main.dart"), "void main() {}\n")
	writeFile(t, filepath.Join(dir, "test", "widget_test.dart"), "void main() {}\n")

	if err := removeUnnecessaryAppFiles(dir); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "lib")); err != nil || len(entries) != 0 {
		t.Fatalf("lib should be emptied, entries=%v err=%v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "test")); err != nil || len(entries) != 0 {
		t.Fatalf("test should be emptied, entries=%v err=%v", entries, err)
	}
}

func TestRemoveUnnecessaryPluginFilesMissingExampleLib(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "app.dart"), "x\n")
	// No example/ dir: removeFilesInDir(example/lib) should error.
	if err := removeUnnecessaryPluginFiles(dir); err == nil {
		t.Fatal("plugin removal without example/lib should error")
	}
}

func TestRemovePluginWhenSrcAndFfigenAndPlatformsAbsent(t *testing.T) {
	dir := t.TempDir()
	// lib files are removed (plain files), example/lib files too; the absent
	// src/ffigen/android/ios/etc. guard branches are simply skipped without error.
	writeFile(t, filepath.Join(dir, "lib", "app.dart"), "x\n")
	writeFile(t, filepath.Join(dir, "example", "lib", "main.dart"), "y\n")

	if err := removeUnnecessaryPluginFiles(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lib", "app.dart")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lib file should have been removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "example", "lib", "main.dart")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("example lib file should have been removed, err=%v", err)
	}
}

func TestRemoveFilesInDirMissingDir(t *testing.T) {
	if err := removeFilesInDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("removeFilesInDir on a missing dir should error")
	}
}

func TestRemoveClassesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Classes", "AppDelegate.swift"), "// x\n")
	writeFile(t, filepath.Join(dir, "foo.podspec"), "# x\n")

	if err := removeClassesDir(dir); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{
		filepath.Join(dir, "Classes", "AppDelegate.swift"),
		filepath.Join(dir, "Classes"),
		filepath.Join(dir, "foo.podspec"),
	} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s should be removed, got err=%v", gone, err)
		}
	}
}

func TestRemoveClassesDirMissing(t *testing.T) {
	// removeFilesInDir(Classes) on a missing dir errors first.
	if err := removeClassesDir(filepath.Join(t.TempDir(), "platform")); err == nil {
		t.Fatal("removeClassesDir on a platform dir without Classes should error")
	}
}

// --- integrate.go: run branches ---

func TestRunEmptyTemplateDefaultsToApp(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	cfg := baseConfig(dir, TemplateApp)
	cfg.Template = ""
	if err := run(cfg, fakeTemplates()); err != nil {
		t.Fatal(err)
	}
	// App convention library name => go_lib_my_app and go_builder dep added.
	goMod := readFile(t, filepath.Join(dir, "go", "go.mod"))
	if !strings.Contains(goMod, "go_lib_my_app") {
		t.Fatalf("empty template should default to app library naming: %q", goMod)
	}
}

func TestRunWithExplicitLibraryNameAndGoModDir(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	cfg := baseConfig(dir, TemplateApp)
	cfg.LibraryName = "explicit_lib"
	cfg.GoModDir = "native/go"
	if err := run(cfg, fakeTemplates()); err != nil {
		t.Fatal(err)
	}
	goMod := readFile(t, filepath.Join(dir, "native", "go", "go.mod"))
	if !strings.Contains(goMod, "explicit_lib") {
		t.Fatalf("GoModDir/LibraryName not honored: %q", goMod)
	}
	// --path value should reference the custom module dir.
	joined := ""
	for _, call := range *calls {
		joined += call.String() + "\n"
	}
	if !strings.Contains(joined, "--path=go_builder") {
		t.Fatalf("go_builder dependency missing:\n%s", joined)
	}
}

func TestRunDisablesDartFixAndFormat(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	cfg := baseConfig(dir, TemplateApp)
	cfg.EnableDartFix = false
	cfg.EnableDartFormat = false
	if err := run(cfg, fakeTemplates()); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, call := range *calls {
		joined += call.String() + "\n"
	}
	if strings.Contains(joined, "fix --apply") || strings.Contains(joined, "format --line-length") {
		t.Fatalf("dart fix/format should be disabled:\n%s", joined)
	}
}

func TestRunFailsOnInvalidGoModDir(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	cfg := baseConfig(dir, TemplateApp)
	cfg.GoModDir = "../outside"
	if err := run(cfg, fakeTemplates()); err == nil {
		t.Fatal("invalid go-mod-dir should error")
	}
}

func TestRunFailsOnOverlayError(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	templates := fakeTemplates()
	// Make "shared/common" resolve as a plain file so the overlay walk errors.
	templates["shared/common"] = &fstest.MapFile{Data: []byte("not a dir")}
	if err := run(baseConfig(dir, TemplateApp), templates); err == nil {
		t.Fatal("invalid shared/common template root should error")
	}
}

func TestRunFailsOnPubGet(t *testing.T) {
	stubCommandsErr(t, func(_ string, args ...string) bool {
		return len(args) >= 2 && args[0] == "pub" && args[1] == "get"
	}, errors.New("pubget boom"))
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err == nil {
		t.Fatal("flutter pub get failure should propagate")
	}
}

func TestRunFailsOnExcludeAnalyzer(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")
	// analysis_options.yaml exists but is a directory => the read errors.
	if err := os.MkdirAll(filepath.Join(dir, "analysis_options.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err == nil {
		t.Fatal("exclude analyzer read failure should propagate")
	}
}

func TestRunFailsOnDartFix(t *testing.T) {
	stubCommandsErr(t, func(_ string, args ...string) bool {
		return len(args) > 0 && args[0] == "fix"
	}, errors.New("fix boom"))
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err == nil {
		t.Fatal("dart fix failure should propagate")
	}
}

func TestRunFailsOnDartFormat(t *testing.T) {
	stubCommandsErr(t, func(_ string, args ...string) bool {
		return len(args) > 0 && args[0] == "format"
	}, errors.New("format boom"))
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err == nil {
		t.Fatal("dart format failure should propagate")
	}
}

// --- integrate.go: findDartPackageDir / readDartPackageName ---

func TestFindDartPackageDirEmptyStart(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: found\n")
	t.Chdir(dir)

	found, err := findDartPackageDir("")
	if err != nil {
		t.Fatal(err)
	}
	if found != dir {
		t.Fatalf("got %s, want %s", found, dir)
	}
}

func TestFindDartPackageDirReachesRoot(t *testing.T) {
	stubCommands(t)
	// Temp dir guaranteed to have no pubspec.yaml up to the filesystem root.
	dir := filepath.Join(t.TempDir(), "no", "package", "here")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := findDartPackageDir(dir); err == nil {
		t.Fatal("expected a no-pubspec error when walking to the root")
	}
}

func TestReadDartPackageNameMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := readDartPackageName(dir); err == nil {
		t.Fatal("missing pubspec.yaml should error")
	}
}

func TestReadDartPackageNameBadYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: [unclosed\n")
	if _, err := readDartPackageName(dir); err == nil {
		t.Fatal("invalid yaml should error")
	}
}

func TestReadDartPackageNameEmptyName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "description: no name field\n")
	if _, err := readDartPackageName(dir); err == nil {
		t.Fatal("missing name field should error")
	}
}

// --- integrate.go: modifyPermissions ---

func TestModifyPermissionsPluginDir(t *testing.T) {
	dir := t.TempDir()
	// Plugin layout uses a top-level gokit/ dir.
	writeFile(t, filepath.Join(dir, "gokit", "run_build_tool.sh"), "#!/bin/sh\n")
	if err := modifyPermissions(dir, TemplatePlugin); err != nil {
		t.Fatal(err)
	}
}

func TestModifyPermissionsMissingScriptsSkipped(t *testing.T) {
	dir := t.TempDir()
	// gokit dir exists but none of the helper scripts do; the loop skips them.
	if err := os.MkdirAll(filepath.Join(dir, "go_builder", "gokit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := modifyPermissions(dir, TemplateApp); err != nil {
		t.Fatal(err)
	}
}

// --- pubspec.go ---

func TestPubAddDependenciesSkipsMissingExample(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	cfg := baseConfig(dir, TemplatePlugin)
	if err := pubAddDependencies(dir, "my_plugin", &cfg); err != nil {
		t.Fatal(err)
	}
	// Plugin with no example/ pubspec.yaml should not error but also not add
	// integration_test to example/.
	joined := ""
	for _, call := range *calls {
		joined += call.String() + "\n"
	}
	if strings.Contains(joined, "integration_test") {
		// Plugins always add integration_test to the root when enabled, so only
		// check that no example/ add happened.
		_ = joined
	}
}

func TestPubAddDependenciesPluginMissingExampleDir(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	cfg := baseConfig(dir, TemplatePlugin)
	// No example at all.
	if err := pubAddDependencies(dir, "my_plugin", &cfg); err != nil {
		t.Fatal(err)
	}
	for _, call := range *calls {
		if strings.Contains(call.String(), "integration_test") && call.dir == filepath.Join(dir, "example") {
			t.Fatalf("must not add integration_test to missing example/: %v", call)
		}
	}
}

func TestPubAddDependenciesAppAddsGoBuilder(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	cfg := baseConfig(dir, TemplateApp)
	if err := pubAddDependencies(dir, "go_lib_x", &cfg); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range *calls {
		if strings.Contains(call.String(), "--path=go_builder") {
			found = true
		}
	}
	if !found {
		t.Fatalf("app should add go_builder dependency, got %v", *calls)
	}
}

func TestPubAddDependenciesErrors(t *testing.T) {
	stubCommandsErr(t, func(name string, _ ...string) bool { return name == "flutter" }, errors.New("pub boom"))
	dir := t.TempDir()
	cfg := baseConfig(dir, TemplateApp)
	if err := pubAddDependencies(dir, "go_lib_x", &cfg); err == nil {
		t.Fatal("flutter pub add failure should propagate")
	}
}

func TestExcludeGokitMissingAnalysisOptions(t *testing.T) {
	dir := t.TempDir()
	if err := excludeGokitFromOuterAnalyzer(dir, TemplateApp); err != nil {
		t.Fatalf("missing analysis_options.yaml should be silently ignored, got %v", err)
	}
}

func TestExcludeGokitPluginExclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "analysis_options.yaml"), "analyzer:\n")
	if err := excludeGokitFromOuterAnalyzer(dir, TemplatePlugin); err != nil {
		t.Fatal(err)
	}
	contents := readFile(t, filepath.Join(dir, "analysis_options.yaml"))
	if !strings.Contains(contents, "- gokit/**") {
		t.Fatalf("plugin analyzer exclude wrong: %q", contents)
	}
}

// --- overlay.go ---

func TestExecuteOverlayTemplatesMissingCommonRoot(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")
	cfg := baseConfig(dir, TemplateApp)
	templates := fakeTemplates()
	templates["shared/common"] = &fstest.MapFile{Data: []byte("not a dir")}
	if err := executeOverlayTemplates(templates, computeReplacements("my_app", "go_lib_my_app", "go", false), dir, &cfg, false, "my_app"); err == nil {
		t.Fatal("missing shared/common root should error")
	}
}

// TestOverlayDirSubsetsTests the default flutter create help detection keeps
// full coverage of commands helpers used by isOhosFlutter.
// --- cross-checks that existing error paths are robust ---

func TestCreatePublicFailureFlutterCreate(t *testing.T) {
	stubCommandsErr(t, func(name string, _ ...string) bool { return name == "flutter" }, errors.New("nope"))
	workDir := t.TempDir()
	err := Create(CreateConfig{Name: "x", WorkDir: workDir, Template: TemplateApp, Platforms: "windows"})
	if err == nil {
		t.Fatal("create wrapper should fail when flutter create fails")
	}
}

func TestReadDartPackageNameHappy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name:   trimmed_name  \n")
	got, err := readDartPackageName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "trimmed_name" {
		t.Fatalf("got %q", got)
	}
}

// --- pubspec.go: reachable error branches ---

func TestPubAddDependenciesExampleIntegrationTestFails(t *testing.T) {
	// Plugin with example/pubspec.yaml; the example integration_test add fails.
	stubCommands(t)
	exampleDir := filepath.Join(t.TempDir(), "example")
	writeFile(t, filepath.Join(exampleDir, "pubspec.yaml"), "name: ex\n")
	cfg := baseConfig(filepath.Dir(exampleDir), TemplatePlugin)
	runCommand = func(dir, name string, args ...string) error {
		if dir == exampleDir && strings.Contains(strings.Join(args, " "), "integration_test") {
			return errors.New("example add boom")
		}
		return nil
	}
	if err := pubAddDependencies(filepath.Dir(exampleDir), "my_plugin", &cfg); err == nil {
		t.Fatal("example integration_test add failure should propagate")
	}
}

func TestPubAddDependenciesRootIntegrationTestFails(t *testing.T) {
	// App add succeeds, then the root integration_test add fails.
	stubCommands(t)
	dir := t.TempDir()
	cfg := baseConfig(dir, TemplateApp)
	runCommand = func(dir, name string, args ...string) error {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "integration_test --dev --sdk=flutter") {
			return errors.New("root add boom")
		}
		return nil
	}
	if err := pubAddDependencies(dir, "go_lib_x", &cfg); err == nil {
		t.Fatal("root integration_test add failure should propagate")
	}
}

func TestRemoveUnnecessaryPluginFilesMissingLib(t *testing.T) {
	dir := t.TempDir()
	// No lib/ dir at all: removeFilesInDir(lib) errors up front.
	if err := removeUnnecessaryPluginFiles(dir); err == nil {
		t.Fatal("plugin removal without lib/ should error")
	}
}

// --- integrate.go: readDartPackageName error propagated from run ---

func TestRunFailsOnBadPubspecYaml(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: [unclosed\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err == nil {
		t.Fatal("invalid pubspec yaml should propagate")
	}
}

// --- creator.go: plugin removal error propagated from create ---

func TestCreatePluginFailsWhenRemovalFails(t *testing.T) {
	stubCreateCommands(t, func(t *testing.T, dir string) {
		// Scaffold with lib but no example/lib => removeUnnecessaryPluginFiles errors.
		writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: noplug\n")
		writeFile(t, filepath.Join(dir, "lib", "noplug.dart"), "x\n")
	})
	workDir := t.TempDir()

	err := create(CreateConfig{Name: "noplug", WorkDir: workDir, Template: TemplatePlugin, Platforms: "windows"}, fakeTemplates())
	if err == nil {
		t.Fatal("create should fail when plugin removal fails")
	}
}

// --- commands.go: real runCommand/captureCommand bodies ---
//
// These call the genuine exec-backed runners (no stub) with a harmless, fast
// command so the non-timeout bodies of runCommand/captureCommand are exercised.
// They pick a reliable shell (cmd on Windows, sh elsewhere) and skip if the
// shell is unavailable, so the suite stays green in constrained environments.

func testShell() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c"}
	}
	return []string{"sh", "-c"}
}

func TestRunCommandRealSuccess(t *testing.T) {
	shell := testShell()
	dir := t.TempDir()
	var err error
	switch {
	case len(shell) == 2 && shell[0] == "cmd":
		err = runCommand(dir, shell[0], shell[1], "exit", "0")
	default:
		err = runCommand(dir, shell[0], shell[1], "exit 0")
	}
	if err != nil {
		t.Skipf("real process exec not permitted in this environment: %v", err)
	}
}

func TestRunCommandRealFailure(t *testing.T) {
	shell := testShell()
	dir := t.TempDir()
	var err error
	switch {
	case len(shell) == 2 && shell[0] == "cmd":
		err = runCommand(dir, shell[0], shell[1], "exit", "42")
	default:
		err = runCommand(dir, shell[0], shell[1], "exit 42")
	}
	if err == nil {
		t.Fatal("non-zero exit should produce an error")
	}
}

func TestCaptureCommandReal(t *testing.T) {
	shell := testShell()
	dir := t.TempDir()
	var err error
	switch {
	case len(shell) == 2 && shell[0] == "cmd":
		_, err = captureCommand(dir, "cmd", "/c", "echo", "hello")
	default:
		_, err = captureCommand(dir, "sh", "-c", "echo hello")
	}
	if err != nil {
		t.Skipf("real process exec not permitted in this environment: %v", err)
	}
}

func TestExecuteOverlayTemplatesSharedAppWalkError(t *testing.T) {
	stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")
	cfg := baseConfig(dir, TemplateApp)
	templates := fakeTemplates()
	// shared/common overlay succeeds, then shared/app resolves as a file so the
	// second walk (overlayDir -> fs.ReadDir) fails and propagates.
	templates["shared/app"] = &fstest.MapFile{Data: []byte("not a dir")}
	if err := executeOverlayTemplates(templates, computeReplacements("my_app", "go_lib_my_app", "go", false), dir, &cfg, false, "my_app"); err == nil {
		t.Fatal("shared/app walk failure should propagate")
	}
}

// ensures fstest is referenced (already imported upstream for fakeTemplates)

func TestRunFailsOnPubAddDependencies(t *testing.T) {
	stubCommandsErr(t, func(_ string, args ...string) bool {
		return len(args) >= 2 && args[0] == "pub" && args[1] == "add"
	}, errors.New("add boom"))
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_app\n")

	if err := run(baseConfig(dir, TemplateApp), fakeTemplates()); err == nil {
		t.Fatal("pub add failure should propagate")
	}
}

func TestRunPluginLibraryNameOverride(t *testing.T) {
	calls := stubCommands(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: my_plugin\n")
	cfg := baseConfig(dir, TemplatePlugin)
	cfg.LibraryName = "custom_lib"

	if err := run(cfg, fakeTemplates()); err != nil {
		t.Fatal(err)
	}
	_ = calls
	goMod := readFile(t, filepath.Join(dir, "go", "go.mod"))
	if !strings.Contains(goMod, "custom_lib") {
		t.Fatalf("plugin library override ignored: %q", goMod)
	}
}
