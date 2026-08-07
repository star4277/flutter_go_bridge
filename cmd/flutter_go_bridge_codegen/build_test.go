package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/platformbuild"
)

type recordingPlatformBuilder struct {
	request platformbuild.Request
	result  platformbuild.Result
	err     error
}

func (builder *recordingPlatformBuilder) Build(_ context.Context, request platformbuild.Request) (platformbuild.Result, error) {
	builder.request = request
	return builder.result, builder.err
}

func TestBuildCommandGeneratesAndSelectsPlatform(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		platform platformbuild.Platform
	}{
		{name: "Web", target: "web", platform: platformbuild.PlatformWeb},
		{name: "Native", target: "windows", platform: platformbuild.PlatformNative},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseDir, configPath := writeBuildFixture(t)
			builder := &recordingPlatformBuilder{
				result: platformbuild.Result{Artifacts: []string{filepath.Join(baseDir, "artifact")}},
			}
			original := newPlatformBuilder
			newPlatformBuilder = func(platform platformbuild.Platform) platformbuild.PlatformBuilder {
				if platform != test.platform {
					t.Fatalf("platform = %q, want %q", platform, test.platform)
				}
				return builder
			}
			t.Cleanup(func() { newPlatformBuilder = original })

			command := newRootCommand()
			command.SetArgs([]string{
				"build", test.target,
				"--config-file", configPath,
				"--no-dart-format",
				"--", "--release",
			})
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}

			if builder.request.Platform != test.platform {
				t.Fatalf("request platform = %q, want %q", builder.request.Platform, test.platform)
			}
			if builder.request.Target != test.target {
				t.Fatalf("request target = %q, want %q", builder.request.Target, test.target)
			}
			if !reflect.DeepEqual(builder.request.FlutterArgs, []string{test.target, "--release"}) {
				t.Fatalf("Flutter args = %#v", builder.request.FlutterArgs)
			}
			if builder.request.BaseDir != baseDir || builder.request.ProjectDir != baseDir {
				t.Fatalf("directories = base %q, project %q, want %q", builder.request.BaseDir, builder.request.ProjectDir, baseDir)
			}
			if builder.request.ManifestDir != filepath.Join(baseDir, "go") {
				t.Fatalf("manifest dir = %q", builder.request.ManifestDir)
			}
			for _, generated := range []string{
				filepath.Join(baseDir, "go", "bridge_generated.go"),
				filepath.Join(baseDir, "go", "bridge_generated_web.go"),
				filepath.Join(baseDir, "lib", "src", "bridge_generated.dart"),
			} {
				if _, err := os.Stat(generated); err != nil {
					t.Fatalf("generated file %s: %v", generated, err)
				}
			}
		})
	}
}

func TestBuildCommandValidatesPlatformPosition(t *testing.T) {
	for _, args := range [][]string{
		{"build"},
		{"build", "web", "extra"},
		{"build", "--", "--release"},
	} {
		command := newRootCommand()
		command.SetArgs(args)
		err := command.Execute()
		if err == nil || !strings.Contains(err.Error(), "exactly one Flutter platform before --") {
			t.Fatalf("args %#v error = %v", args, err)
		}
	}
}

func TestIsWebFlutterBuild(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{args: []string{"web"}, want: true},
		{args: []string{" WEB ", "--wasm"}, want: true},
		{args: []string{"windows"}, want: false},
		{args: nil, want: false},
	} {
		if got := isWebFlutterBuild(test.args); got != test.want {
			t.Fatalf("isWebFlutterBuild(%#v) = %v, want %v", test.args, got, test.want)
		}
	}
}

func TestRootCommandIncludesBuild(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() == "build" {
			return
		}
	}
	t.Fatal("root command does not include build")
}

func writeBuildFixture(t *testing.T) (string, string) {
	t.Helper()
	baseDir := t.TempDir()
	goDir := filepath.Join(baseDir, "go")
	if err := os.MkdirAll(filepath.Join(baseDir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(goDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(baseDir, "pubspec.yaml"):     "name: build_fixture\nversion: 1.0.0\nenvironment:\n  sdk: '>=3.0.0 <4.0.0'\n",
		filepath.Join(baseDir, "lib", "main.dart"): "void main() {}\n",
		filepath.Join(goDir, "go.mod"):             "module example.com/build_fixture\n\ngo 1.25.0\n",
		filepath.Join(goDir, "api.go"):             "package main\n\nfunc Add(left, right int64) int64 { return left + right }\n\nfunc main() {}\n",
	}
	for path, contents := range files {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(baseDir, "flutter_go_bridge.yaml")
	config := "go_input: go\ngo_output: go/bridge_generated.go\ndart_output: lib/src/bridge_generated.dart\ndart_format: false\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return baseDir, configPath
}
