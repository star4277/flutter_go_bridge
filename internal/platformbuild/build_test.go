package platformbuild

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type commandCall struct {
	name string
	args []string
	dir  string
	env  []string
}

type recordingRunner struct {
	calls []commandCall
	errs  []error
}

func (runner *recordingRunner) Run(_ context.Context, name string, args []string, dir string, env []string) error {
	runner.calls = append(runner.calls, commandCall{
		name: name,
		args: append([]string{}, args...),
		dir:  dir,
		env:  append([]string{}, env...),
	})
	index := len(runner.calls) - 1
	if index < len(runner.errs) {
		return runner.errs[index]
	}
	return nil
}

type recordingSigner struct {
	called bool
	err    error
}

func (signer *recordingSigner) Sign(_ context.Context, _ Request, result Result) (Result, error) {
	signer.called = true
	result.Artifacts = append(result.Artifacts, "signed")
	return result, signer.err
}

func TestNativeBuilderBuild(t *testing.T) {
	runner := &recordingRunner{}
	signer := &recordingSigner{}
	builder := &NativeBuilder{Runner: runner, Signer: signer, HostOS: "windows"}
	request := Request{
		ProjectDir:     `C:\project`,
		FlutterArgs:    []string{"windows", "--release"},
		FlutterCommand: `C:\flutter\bin\flutter.bat`,
		AdditionalEnv:  []string{"FGB_TEST=1"},
	}

	result, err := builder.Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !signer.called || !reflect.DeepEqual(result.Artifacts, []string{"signed"}) {
		t.Fatalf("signing result = %#v, called = %v", result, signer.called)
	}
	want := commandCall{
		name: "cmd",
		args: []string{"/c", request.FlutterCommand, "build", "windows", "--release"},
		dir:  request.ProjectDir,
		env:  request.AdditionalEnv,
	}
	if !reflect.DeepEqual(runner.calls, []commandCall{want}) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, []commandCall{want})
	}
}

func TestNativeBuilderFailuresAndDefaults(t *testing.T) {
	t.Run("Flutter build failure", func(t *testing.T) {
		runner := &recordingRunner{errs: []error{errors.New("flutter failed")}}
		signer := &recordingSigner{}
		_, err := (&NativeBuilder{Runner: runner, Signer: signer, HostOS: "linux"}).Build(
			context.Background(),
			Request{ProjectDir: t.TempDir(), FlutterArgs: []string{"linux"}, FlutterCommand: "/flutter"},
		)
		if err == nil || !strings.Contains(err.Error(), "Flutter build failed") {
			t.Fatalf("error = %v", err)
		}
		if signer.called {
			t.Fatal("signer called after Flutter build failure")
		}
	})

	t.Run("signing failure", func(t *testing.T) {
		signer := &recordingSigner{err: errors.New("sign failed")}
		_, err := (&NativeBuilder{Runner: &recordingRunner{}, Signer: signer, HostOS: "linux"}).Build(
			context.Background(),
			Request{ProjectDir: t.TempDir(), FlutterArgs: []string{"linux"}, FlutterCommand: "/flutter"},
		)
		if err == nil || !strings.Contains(err.Error(), "sign Native artifacts") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("zero value dependencies", func(t *testing.T) {
		builder := &NativeBuilder{}
		if _, ok := builder.runner().(ExecRunner); !ok {
			t.Fatalf("runner = %T", builder.runner())
		}
		if _, ok := builder.signer().(NativeSigner); !ok {
			t.Fatalf("signer = %T", builder.signer())
		}
		if builder.hostOS() != runtime.GOOS {
			t.Fatalf("hostOS = %q", builder.hostOS())
		}
		created := NewNativeBuilder()
		if created.HostOS != runtime.GOOS {
			t.Fatalf("constructor hostOS = %q", created.HostOS)
		}
	})
}

func TestWebBuilderBuild(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "example")
	manifestDir := filepath.Join(baseDir, "go")
	outputDir := filepath.Join(baseDir, "assets", "wasm")
	mustMkdirAll(t, filepath.Join(baseDir, "gokit", "build_tool"))
	mustMkdirAll(t, projectDir)
	mustMkdirAll(t, manifestDir)
	mustMkdirAll(t, outputDir)
	mustWriteFile(t, filepath.Join(outputDir, "bridge.wasm"))
	mustWriteFile(t, filepath.Join(outputDir, "fgb_wasm_manifest.json"))

	runner := &recordingRunner{}
	signer := &recordingSigner{}
	builder := &WebBuilder{Runner: runner, Signer: signer, HostOS: "linux"}
	request := Request{
		BaseDir:        baseDir,
		ProjectDir:     projectDir,
		ManifestDir:    manifestDir,
		FlutterArgs:    []string{"web", "--release"},
		FlutterCommand: "/sdk/flutter/bin/flutter",
		FlutterRoot:    "/sdk/flutter",
		AdditionalEnv:  []string{"FGB_TEST=1"},
	}

	result, err := builder.Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantArtifacts := []string{
		filepath.Join(outputDir, "bridge.wasm"),
		filepath.Join(outputDir, "fgb_wasm_manifest.json"),
		"signed",
	}
	if !reflect.DeepEqual(result.Artifacts, wantArtifacts) {
		t.Fatalf("artifacts = %#v, want %#v", result.Artifacts, wantArtifacts)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	wasmCall := runner.calls[0]
	if wasmCall.name != filepath.Join(baseDir, "gokit", "run_build_tool.sh") {
		t.Fatalf("Wasm command = %q", wasmCall.name)
	}
	wantWasmArgs := []string{
		"build-web",
		"--manifest-dir", manifestDir,
		"--output-dir", outputDir,
		"--root-project-dir", baseDir,
	}
	if !reflect.DeepEqual(wasmCall.args, wantWasmArgs) {
		t.Fatalf("Wasm args = %#v, want %#v", wasmCall.args, wantWasmArgs)
	}
	for _, entry := range []string{
		"FGB_TEST=1",
		"GOKIT_TOOL_TEMP_DIR=" + filepath.Join(baseDir, "build", ".gokit_tool"),
		"GOKIT_ROOT_PROJECT_DIR=" + baseDir,
		"FLUTTER_ROOT=/sdk/flutter",
	} {
		if !contains(wasmCall.env, entry) {
			t.Fatalf("Wasm env %#v does not contain %q", wasmCall.env, entry)
		}
	}
	flutterCall := runner.calls[1]
	if flutterCall.name != request.FlutterCommand || !reflect.DeepEqual(flutterCall.args, []string{"build", "web", "--release"}) {
		t.Fatalf("Flutter call = %#v", flutterCall)
	}
}

func TestWebBuilderAppLayoutAndWindowsWrapper(t *testing.T) {
	baseDir := t.TempDir()
	outputDir := filepath.Join(baseDir, "go_builder", "assets", "wasm")
	mustMkdirAll(t, filepath.Join(baseDir, "go_builder", "gokit", "build_tool"))
	mustMkdirAll(t, filepath.Join(outputDir, "ignored"))
	mustWriteFile(t, filepath.Join(outputDir, "bridge.wasm"))

	runner := &recordingRunner{}
	builder := &WebBuilder{Runner: runner, HostOS: "windows"}
	result, err := builder.BuildWasm(context.Background(), Request{
		BaseDir:     baseDir,
		ManifestDir: filepath.Join(baseDir, "go"),
		FlutterRoot: `C:\flutter`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Artifacts, []string{filepath.Join(outputDir, "bridge.wasm")}) {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
	call := runner.calls[0]
	wantScript := filepath.Join(baseDir, "go_builder", "gokit", "run_build_tool.cmd")
	if call.name != "cmd" || !reflect.DeepEqual(call.args[:2], []string{"/c", wantScript}) {
		t.Fatalf("Windows wrapper call = %#v", call)
	}
}

func TestWebBuilderFailuresAndDefaults(t *testing.T) {
	t.Run("missing Gokit", func(t *testing.T) {
		_, err := (&WebBuilder{Runner: &recordingRunner{}, HostOS: "linux"}).BuildWasm(
			context.Background(),
			Request{BaseDir: t.TempDir()},
		)
		if err == nil || !strings.Contains(err.Error(), "integrated Gokit build_tool") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("Gokit failure", func(t *testing.T) {
		baseDir := t.TempDir()
		mustMkdirAll(t, filepath.Join(baseDir, "gokit", "build_tool"))
		runner := &recordingRunner{errs: []error{errors.New("gokit failed")}}
		_, err := (&WebBuilder{Runner: runner, HostOS: "linux"}).BuildWasm(
			context.Background(),
			Request{BaseDir: baseDir, ManifestDir: filepath.Join(baseDir, "go")},
		)
		if err == nil || !strings.Contains(err.Error(), "Gokit Web Wasm build failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing output", func(t *testing.T) {
		baseDir := t.TempDir()
		mustMkdirAll(t, filepath.Join(baseDir, "gokit", "build_tool"))
		_, err := (&WebBuilder{Runner: &recordingRunner{}, HostOS: "linux"}).BuildWasm(
			context.Background(),
			Request{BaseDir: baseDir, ManifestDir: filepath.Join(baseDir, "go")},
		)
		if err == nil || !strings.Contains(err.Error(), "read Web Wasm output directory") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("Flutter failure", func(t *testing.T) {
		baseDir := t.TempDir()
		mustMkdirAll(t, filepath.Join(baseDir, "gokit", "build_tool"))
		mustMkdirAll(t, filepath.Join(baseDir, "assets", "wasm"))
		runner := &recordingRunner{errs: []error{nil, errors.New("flutter failed")}}
		_, err := (&WebBuilder{Runner: runner, HostOS: "linux"}).Build(
			context.Background(),
			Request{BaseDir: baseDir, ProjectDir: baseDir, ManifestDir: baseDir, FlutterArgs: []string{"web"}, FlutterCommand: "/flutter"},
		)
		if err == nil || !strings.Contains(err.Error(), "Flutter build failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("signing failure", func(t *testing.T) {
		baseDir := t.TempDir()
		mustMkdirAll(t, filepath.Join(baseDir, "gokit", "build_tool"))
		mustMkdirAll(t, filepath.Join(baseDir, "assets", "wasm"))
		signer := &recordingSigner{err: errors.New("sign failed")}
		_, err := (&WebBuilder{Runner: &recordingRunner{}, Signer: signer, HostOS: "linux"}).Build(
			context.Background(),
			Request{BaseDir: baseDir, ProjectDir: baseDir, ManifestDir: baseDir, FlutterArgs: []string{"web"}, FlutterCommand: "/flutter"},
		)
		if err == nil || !strings.Contains(err.Error(), "sign Web artifacts") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("zero value dependencies", func(t *testing.T) {
		builder := &WebBuilder{}
		if _, ok := builder.runner().(ExecRunner); !ok {
			t.Fatalf("runner = %T", builder.runner())
		}
		if _, ok := builder.signer().(WebSigner); !ok {
			t.Fatalf("signer = %T", builder.signer())
		}
		if builder.hostOS() != runtime.GOOS {
			t.Fatalf("hostOS = %q", builder.hostOS())
		}
		created := NewWebBuilder()
		if created.HostOS != runtime.GOOS {
			t.Fatalf("constructor hostOS = %q", created.HostOS)
		}
	})
}

func TestCommandPathAndFlutterRoot(t *testing.T) {
	t.Run("configured Unix command", func(t *testing.T) {
		name, prefix, err := commandPath("/sdk/flutter", "flutter", "linux")
		if err != nil || name != "/sdk/flutter" || prefix != nil {
			t.Fatalf("commandPath = %q, %#v, %v", name, prefix, err)
		}
	})

	t.Run("missing fallback", func(t *testing.T) {
		_, _, err := commandPath("", "fgb-command-that-does-not-exist", runtime.GOOS)
		if err == nil || !strings.Contains(err.Error(), "find fgb-command-that-does-not-exist executable") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("explicit Flutter root", func(t *testing.T) {
		if got := resolveFlutterRoot(Request{FlutterRoot: "/flutter"}); got != "/flutter" {
			t.Fatalf("root = %q", got)
		}
	})

	t.Run("resolved Flutter command", func(t *testing.T) {
		goCommand, err := exec.LookPath("go")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Dir(filepath.Dir(goCommand))
		if got := resolveFlutterRoot(Request{FlutterCommand: "go"}); got != want {
			t.Fatalf("root = %q, want %q", got, want)
		}
	})

	t.Run("missing Flutter command", func(t *testing.T) {
		if got := resolveFlutterRoot(Request{FlutterCommand: "fgb-command-that-does-not-exist"}); got != "" {
			t.Fatalf("root = %q", got)
		}
	})
}

func TestPlatformSignersAndExecRunner(t *testing.T) {
	input := Result{Artifacts: []string{"artifact"}}
	for name, signer := range map[string]ArtifactSigner{"native": NativeSigner{}, "web": WebSigner{}} {
		t.Run(name, func(t *testing.T) {
			result, err := signer.Sign(context.Background(), Request{}, input)
			if err != nil || !reflect.DeepEqual(result, input) {
				t.Fatalf("result = %#v, error = %v", result, err)
			}
		})
	}

	goCommand, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	if err := (ExecRunner{}).Run(context.Background(), goCommand, []string{"version"}, t.TempDir(), []string{"FGB_TEST=1"}); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
