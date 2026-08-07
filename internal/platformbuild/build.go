// Package platformbuild owns the synchronous platform build and signing
// boundary used by the codegen CLI.
package platformbuild

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Platform string

const (
	PlatformNative Platform = "native"
	PlatformWeb    Platform = "web"
)

// Request is the stable input shared by platform builders and signers.
// FlutterArgs contains the target and all arguments after `flutter build`.
type Request struct {
	Platform       Platform
	Target         string
	BaseDir        string
	ProjectDir     string
	ManifestDir    string
	FlutterArgs    []string
	FlutterCommand string
	FlutterRoot    string
	AdditionalEnv  []string
}

// Result lists the artifacts made available to the signing boundary.
type Result struct {
	Artifacts []string
}

// PlatformBuilder builds one Flutter platform. Platform-specific signing can
// evolve behind this interface without changing the CLI command contract.
type PlatformBuilder interface {
	Build(context.Context, Request) (Result, error)
}

// ArtifactSigner is the stable signing integration point. The initial
// platform implementations are intentional no-ops until signing is added.
type ArtifactSigner interface {
	Sign(context.Context, Request, Result) (Result, error)
}

// NativeSigner preserves a distinct Native signing entrypoint.
type NativeSigner struct{}

func (NativeSigner) Sign(_ context.Context, _ Request, result Result) (Result, error) {
	return result, nil
}

// WebSigner preserves a distinct Web signing entrypoint.
type WebSigner struct{}

func (WebSigner) Sign(_ context.Context, _ Request, result Result) (Result, error) {
	return result, nil
}

// CommandRunner keeps process execution injectable for platform tests.
type CommandRunner interface {
	Run(context.Context, string, []string, string, []string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args []string, dir string, env []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), env...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

type NativeBuilder struct {
	Runner CommandRunner
	Signer ArtifactSigner
	HostOS string
}

func NewNativeBuilder() *NativeBuilder {
	return &NativeBuilder{Runner: ExecRunner{}, Signer: NativeSigner{}, HostOS: runtime.GOOS}
}

func (builder *NativeBuilder) Build(ctx context.Context, request Request) (Result, error) {
	result, err := runFlutterBuild(ctx, builder.runner(), builder.hostOS(), request)
	if err != nil {
		return Result{}, err
	}
	result, err = builder.signer().Sign(ctx, request, result)
	if err != nil {
		return Result{}, fmt.Errorf("sign Native artifacts: %w", err)
	}
	return result, nil
}

func (builder *NativeBuilder) runner() CommandRunner {
	if builder.Runner != nil {
		return builder.Runner
	}
	return ExecRunner{}
}

func (builder *NativeBuilder) signer() ArtifactSigner {
	if builder.Signer != nil {
		return builder.Signer
	}
	return NativeSigner{}
}

func (builder *NativeBuilder) hostOS() string {
	if builder.HostOS != "" {
		return builder.HostOS
	}
	return runtime.GOOS
}

type WebBuilder struct {
	Runner CommandRunner
	Signer ArtifactSigner
	HostOS string
}

func NewWebBuilder() *WebBuilder {
	return &WebBuilder{Runner: ExecRunner{}, Signer: WebSigner{}, HostOS: runtime.GOOS}
}

func (builder *WebBuilder) Build(ctx context.Context, request Request) (Result, error) {
	result, err := builder.BuildWasm(ctx, request)
	if err != nil {
		return Result{}, err
	}
	if _, err := runFlutterBuild(ctx, builder.runner(), builder.hostOS(), request); err != nil {
		return Result{}, err
	}
	result, err = builder.signer().Sign(ctx, request, result)
	if err != nil {
		return Result{}, fmt.Errorf("sign Web artifacts: %w", err)
	}
	return result, nil
}

// BuildWasm builds and installs only the Go WebAssembly assets. The `run`
// command uses this narrower operation before launching Flutter's daemon.
func (builder *WebBuilder) BuildWasm(ctx context.Context, request Request) (Result, error) {
	layout, err := findGokitLayout(request.BaseDir)
	if err != nil {
		return Result{}, err
	}

	tool := "run_build_tool.sh"
	if builder.hostOS() == "windows" {
		tool = "run_build_tool.cmd"
	}
	scriptPath := filepath.Join(layout.gokitDir, tool)
	name, prefix, err := commandPath(scriptPath, scriptPath, builder.hostOS())
	if err != nil {
		return Result{}, err
	}
	args := append(prefix,
		"build-web",
		"--manifest-dir", request.ManifestDir,
		"--output-dir", layout.outputDir,
		"--root-project-dir", request.BaseDir,
	)
	env := append([]string{}, request.AdditionalEnv...)
	env = append(env,
		"GOKIT_TOOL_TEMP_DIR="+filepath.Join(request.BaseDir, "build", ".gokit_tool"),
		"GOKIT_ROOT_PROJECT_DIR="+request.BaseDir,
	)
	if flutterRoot := resolveFlutterRoot(request); flutterRoot != "" {
		env = append(env, "FLUTTER_ROOT="+flutterRoot)
	}
	if err := builder.runner().Run(ctx, name, args, request.BaseDir, env); err != nil {
		return Result{}, fmt.Errorf("Gokit Web Wasm build failed: %w", err)
	}

	entries, err := os.ReadDir(layout.outputDir)
	if err != nil {
		return Result{}, fmt.Errorf("read Web Wasm output directory: %w", err)
	}
	result := Result{Artifacts: make([]string, 0, len(entries))}
	for _, entry := range entries {
		if !entry.IsDir() {
			result.Artifacts = append(result.Artifacts, filepath.Join(layout.outputDir, entry.Name()))
		}
	}
	return result, nil
}

func (builder *WebBuilder) runner() CommandRunner {
	if builder.Runner != nil {
		return builder.Runner
	}
	return ExecRunner{}
}

func (builder *WebBuilder) signer() ArtifactSigner {
	if builder.Signer != nil {
		return builder.Signer
	}
	return WebSigner{}
}

func (builder *WebBuilder) hostOS() string {
	if builder.HostOS != "" {
		return builder.HostOS
	}
	return runtime.GOOS
}

type gokitLayout struct {
	gokitDir  string
	outputDir string
}

func findGokitLayout(baseDir string) (gokitLayout, error) {
	layouts := []gokitLayout{
		{gokitDir: filepath.Join(baseDir, "gokit"), outputDir: filepath.Join(baseDir, "assets", "wasm")},
		{gokitDir: filepath.Join(baseDir, "go_builder", "gokit"), outputDir: filepath.Join(baseDir, "go_builder", "assets", "wasm")},
	}
	for _, layout := range layouts {
		if _, err := os.Stat(filepath.Join(layout.gokitDir, "build_tool")); err == nil {
			return layout, nil
		}
	}
	return gokitLayout{}, fmt.Errorf("Web Wasm build requires an integrated Gokit build_tool under %s or %s", layouts[0].gokitDir, layouts[1].gokitDir)
}

func runFlutterBuild(ctx context.Context, runner CommandRunner, hostOS string, request Request) (Result, error) {
	name, prefix, err := commandPath(request.FlutterCommand, "flutter", hostOS)
	if err != nil {
		return Result{}, err
	}
	args := append(prefix, "build")
	args = append(args, request.FlutterArgs...)
	if err := runner.Run(ctx, name, args, request.ProjectDir, request.AdditionalEnv); err != nil {
		return Result{}, fmt.Errorf("Flutter build failed: %w", err)
	}
	return Result{}, nil
}

func commandPath(configured, fallback, hostOS string) (string, []string, error) {
	path := configured
	if path == "" {
		resolved, err := exec.LookPath(fallback)
		if err != nil {
			return "", nil, fmt.Errorf("find %s executable: %w", fallback, err)
		}
		path = resolved
	}
	if hostOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".bat" || ext == ".cmd" {
			return "cmd", []string{"/c", path}, nil
		}
	}
	return path, nil, nil
}

func resolveFlutterRoot(request Request) string {
	if request.FlutterRoot != "" {
		return request.FlutterRoot
	}
	flutter := request.FlutterCommand
	if flutter == "" {
		resolved, err := exec.LookPath("flutter")
		if err != nil {
			return ""
		}
		flutter = resolved
	} else {
		resolved, err := exec.LookPath(flutter)
		if err != nil {
			return ""
		}
		flutter = resolved
	}
	return filepath.Dir(filepath.Dir(flutter))
}
