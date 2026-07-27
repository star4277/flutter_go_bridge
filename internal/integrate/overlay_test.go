package integrate

import (
	"strings"
	"testing"
)

func TestFilterFileExcludesOhosWhenNotEnabled(t *testing.T) {
	if filterFile("go_builder/ohos/src/main/module.json5", true, true, false) {
		t.Fatal("ohos files should be excluded without ohos support")
	}
	if filterFile("ohos/src/main/module.json5", true, true, false) {
		t.Fatal("top-level ohos files should be excluded without ohos support")
	}
	if !filterFile("go_builder/ohos/src/main/module.json5", true, true, true) {
		t.Fatal("ohos files should be included with ohos support")
	}
}

func TestFilterFileNoWriteLibExcludesEnabledOhosPlatformShell(t *testing.T) {
	if filterFile("ohos/src/main/module.json5", false, true, true) {
		t.Fatal("--no-write-lib should exclude the ohos platform shell even when supported")
	}
}

func TestFilterFileExcludesGokitMetadata(t *testing.T) {
	for _, excluded := range []string{
		"go_builder/gokit/.git",
		"gokit/.github",
		"gokit/docs",
		"gokit/test",
	} {
		if filterFile(excluded, true, true, true) {
			t.Fatalf("%s should be excluded", excluded)
		}
	}
	if !filterFile("gokit/build_tool/pubspec.yaml", true, true, true) {
		t.Fatal("gokit build_tool files should be included")
	}
	if !filterFile("gokit/.gitignore", true, true, true) {
		t.Fatal("gokit .gitignore should be included")
	}
}

func TestFilterFileNoWriteLib(t *testing.T) {
	for _, excluded := range []string{
		"lib/main.dart",
		"REPLACE_ME_GO_MOD_DIR/go.mod.template",
		"flutter_go_bridge.yaml",
		"windows/CMakeLists.txt",
	} {
		if filterFile(excluded, false, true, true) {
			t.Fatalf("%s should be excluded with --no-write-lib", excluded)
		}
	}
	if !filterFile("go_builder/windows/CMakeLists.txt", false, true, true) {
		t.Fatal("go_builder must survive --no-write-lib")
	}
}

func TestFilterFileNoIntegrationTest(t *testing.T) {
	if filterFile("test_driver/integration_test.dart", true, false, true) {
		t.Fatal("test_driver should be excluded with --no-integration-test")
	}
	if filterFile("integration_test/simple_test.dart", true, false, true) {
		t.Fatal("integration_test should be excluded with --no-integration-test")
	}
}

func TestReplaceStringContentIsDeterministic(t *testing.T) {
	replacements := computeReplacements("myapp", "go_lib_myapp", "go", false)
	got := replaceStringContent(
		"REPLACE_ME_GO_MOD_DIR/go.mod.template REPLACE_ME_GO_MOD_NAME REPLACE_ME_DART_PACKAGE_NAMEREPLACE_ME_OHOS_PLUGIN_PLATFORM_TEXT",
		replacements,
	)
	if got != "go/go.mod go_lib_myapp myapp" {
		t.Fatalf("unexpected replacement result %q", got)
	}
}

func TestComputeGokitComments(t *testing.T) {
	cases := map[string]struct {
		leading string
		ok      bool
	}{
		"gokit/build_tool/lib/build_tool.dart": {"///", true},
		"gokit/README":                         {"///", true},
		"gokit/build_tool/pubspec.yaml":        {"#", true},
		"gokit/run_build_tool.sh":              {"", false},
		"gokit/run_build_tool.cmd":             {"", false},
		"gokit/cmake/gokit.cmake":              {"", false},
		"gokit/build_tool/pubspec.lock":        {"", false},
		"gokit/.gitignore":                     {"", false},
	}
	for rel, want := range cases {
		comments, ok := computeGokitComments(rel)
		if ok != want.ok {
			t.Fatalf("%s: ok=%v, want %v", rel, ok, want.ok)
		}
		if ok && !strings.HasPrefix(comments, want.leading+" This is copied from Gokit") {
			t.Fatalf("%s: unexpected prelude %q", rel, comments)
		}
	}
}

func TestCommentOutExistingAndAppendTemplate(t *testing.T) {
	got := string(commentOutExistingAndAppendTemplate([]byte("void main() {}\n"), []byte("void generated() {}\n")))
	want := "// The original content is temporarily commented out to allow generating a self-contained demo - feel free to uncomment later.\n\n" +
		"// void main() {}\n// \n\nvoid generated() {}\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAddAnalyzerExcludePrependsAnalyzerBlock(t *testing.T) {
	got := addAnalyzerExclude("include: package:flutter_lints/flutter.yaml\n", "gokit/**")
	want := "analyzer:\n  exclude:\n    - gokit/**\n\ninclude: package:flutter_lints/flutter.yaml\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddAnalyzerExcludePreservesExistingAnalyzerOptions(t *testing.T) {
	got := addAnalyzerExclude(
		"analyzer:\n  errors:\n    prefer_const_constructors: ignore\ninclude: package:flutter_lints/flutter.yaml\n",
		"go_builder/gokit/**",
	)
	want := "analyzer:\n  exclude:\n    - go_builder/gokit/**\n  errors:\n    prefer_const_constructors: ignore\ninclude: package:flutter_lints/flutter.yaml\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddAnalyzerExcludeIsIdempotent(t *testing.T) {
	text := "analyzer:\n  exclude:\n    - go_builder/gokit/**\n"
	if got := addAnalyzerExclude(text, "go_builder/gokit/**"); got != text {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestAddAnalyzerExcludeAppendsToExistingExcludeBlock(t *testing.T) {
	got := addAnalyzerExclude(
		"analyzer:\n  exclude:\n    - build/**\n  errors:\n    avoid_print: ignore\n",
		"go_builder/gokit/**",
	)
	want := "analyzer:\n  exclude:\n    - build/**\n    - go_builder/gokit/**\n  errors:\n    avoid_print: ignore\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddAnalyzerExcludeAppendsToTerminalExcludeBlock(t *testing.T) {
	got := addAnalyzerExclude("analyzer:\n  exclude:\n    - build/**\n", "go_builder/gokit/**")
	want := "analyzer:\n  exclude:\n    - build/**\n    - go_builder/gokit/**\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddAnalyzerExcludePreservesMissingTrailingNewline(t *testing.T) {
	got := addAnalyzerExclude("analyzer:\n  exclude:\n    - build/**", "gokit/**")
	want := "analyzer:\n  exclude:\n    - build/**\n    - gokit/**"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDefaultFlutterPlatforms(t *testing.T) {
	if got := defaultFlutterPlatforms(TemplateApp, false); got != "android,ios,linux,macos,windows,web" {
		t.Fatalf("app: %q", got)
	}
	if got := defaultFlutterPlatforms(TemplateApp, true); got != "android,ios,linux,macos,windows,ohos,web" {
		t.Fatalf("app+ohos: %q", got)
	}
	if got := defaultFlutterPlatforms(TemplatePlugin, false); got != "android,ios,linux,macos,windows" {
		t.Fatalf("plugin: %q", got)
	}
}

func TestPlatformListContainsOhos(t *testing.T) {
	if !platformListContainsOhos("android, ohos") {
		t.Fatal("whitespace should be tolerated")
	}
	if !platformListContainsOhos("android,OHOS") {
		t.Fatal("matching should be case-insensitive")
	}
	if platformListContainsOhos("android,ios,linux,macos,windows") {
		t.Fatal("other platforms should not match")
	}
}

func TestFlutterCreateHelpSupportsPlatform(t *testing.T) {
	withOhos := `
Usage: flutter create <output directory>

    --platforms    The platforms supported by this project.
                   [android, ios, linux, macos, ohos, web, windows]
    --org          The organization responsible for your new Flutter project.
`
	if !flutterCreateHelpSupportsPlatform(withOhos, "ohos") {
		t.Fatal("ohos listed under --platforms should be detected")
	}

	ohosElsewhere := `
Usage: flutter create <output directory>

    --description  Create an app for the ohos store.
    --platforms    The platforms supported by this project.
                   [android, ios, linux, macos, web, windows]
    --org          The organization responsible for your new Flutter project.
`
	if flutterCreateHelpSupportsPlatform(ohosElsewhere, "ohos") {
		t.Fatal("ohos outside the --platforms option must not match")
	}

	afterNextOption := `
    --platforms    The platforms supported by this project.
                   [android, ios, linux, macos, web, windows]
    --org          The organization responsible for your ohos project.
`
	if flutterCreateHelpSupportsPlatform(afterNextOption, "ohos") {
		t.Fatal("scanning must stop at the next option")
	}
}
