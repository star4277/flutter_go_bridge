package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/devrun"
	"github.com/star4277/flutter_go_bridge/internal/generator"
	"github.com/star4277/flutter_go_bridge/internal/integrate"
	"github.com/star4277/flutter_go_bridge/internal/parser"
	"github.com/star4277/flutter_go_bridge/internal/platformbuild"
	"github.com/star4277/flutter_go_bridge/internal/watcher"
)

const (
	versionEnvironment = "FLUTTER_GO_BRIDGE_VERSION"
	defaultVersion     = "development"
)

// version is replaced by the release build's ldflags. Runtime environment
// configuration still wins so local development can override it without a
// rebuild.
var version = defaultVersion

func main() {
	if err := newRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type generateFlags struct {
	configFile              string
	target                  string
	goInput                 string
	goOutput                string
	dartOutput              string
	libraryName             string
	dartEntrypointClassName string
	dartFormatLineLength    int
	dartPreamble            string
	goPreamble              string
	noDartFormat            bool
	stopOnError             bool
	printAST                bool
	watch                   bool
}

func newRootCommand() *cobra.Command {
	flags := &generateFlags{}
	root := &cobra.Command{
		Use:     "flutter_go_bridge_codegen",
		Version: resolvedVersion(),
		Short:   "Generate pure-Dart bindings for a Go library built by Gokit",
	}
	root.SetVersionTemplate("{{.DisplayName}} version {{.Version}}\nBuild with " + runtime.Version() + "\n")
	root.AddCommand(newGenerateCommand(flags))
	root.AddCommand(newRunCommand(&generateFlags{}))
	root.AddCommand(newBuildCommand(&generateFlags{}))
	root.AddCommand(newCreateCommand())
	root.AddCommand(newIntegrateCommand())
	return root
}

func resolvedVersion() string {
	if value := strings.TrimSpace(os.Getenv(versionEnvironment)); value != "" {
		return value
	}
	if value := strings.TrimSpace(version); value != "" {
		return value
	}
	return defaultVersion
}

type runFlags struct {
	deviceID   string
	projectDir string
	dartInput  []string
	interval   time.Duration
}

type buildFlags struct {
	projectDir string
}

var (
	newWebPlatformBuilder = func() *platformbuild.WebBuilder {
		return platformbuild.NewWebBuilder()
	}
	newPlatformBuilder = func(platform platformbuild.Platform) platformbuild.PlatformBuilder {
		if platform == platformbuild.PlatformWeb {
			return newWebPlatformBuilder()
		}
		return platformbuild.NewNativeBuilder()
	}
)

func newBuildCommand(flags *generateFlags) *cobra.Command {
	options := &buildFlags{}
	command := &cobra.Command{
		Use:   "build <platform> [flags] [-- flutter build args]",
		Short: "Generate the bridge and build a Flutter platform",
		Long: `Generate the shared Native/Web bridge, build the selected platform artifact,
and pass the result through the platform signing boundary.

Web builds run Gokit build-web before flutter build web. Other targets use the
Native implementation. Arguments after -- are forwarded to flutter build:

  flutter_go_bridge_codegen build web -- --release
  flutter_go_bridge_codegen build windows -- --release`,
		Args: cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			flutterArgs, err := flutterBuildArgs(command, args)
			if err != nil {
				return err
			}
			return buildFlutter(command, flags, options, flutterArgs)
		},
	}
	registerGenerateFlags(command, flags, false)
	command.Flags().StringVar(&options.projectDir, "project-dir", "", "Flutter project to build (default the config base dir, or its example/ for plugin projects)")
	return command
}

func flutterBuildArgs(command *cobra.Command, args []string) ([]string, error) {
	beforeDash := args
	var forwarded []string
	if index := command.ArgsLenAtDash(); index >= 0 {
		beforeDash = args[:index]
		forwarded = args[index:]
	}
	if len(beforeDash) != 1 || strings.TrimSpace(beforeDash[0]) == "" {
		return nil, errors.New("build requires exactly one Flutter platform before --")
	}
	return append([]string{beforeDash[0]}, forwarded...), nil
}

func buildFlutter(command *cobra.Command, flags *generateFlags, options *buildFlags, flutterArgs []string) error {
	resolved, err := resolveGenerateConfig(command, flags)
	if err != nil {
		return err
	}
	projectDir, err := resolveProjectDir(resolved.BaseDir, options.projectDir)
	if err != nil {
		return err
	}
	if _, err := runGenerateFiles(command, flags); err != nil {
		return err
	}

	request := platformbuild.Request{
		BaseDir:     resolved.BaseDir,
		ProjectDir:  projectDir,
		ManifestDir: filepath.Dir(resolved.GoOutput),
		FlutterArgs: flutterArgs,
		Target:      flutterArgs[0],
	}
	platform := platformbuild.PlatformNative
	if isWebFlutterBuild(flutterArgs) {
		platform = platformbuild.PlatformWeb
	}
	request.Platform = platform
	builder := newPlatformBuilder(platform)
	result, err := builder.Build(command.Context(), request)
	if err != nil {
		return err
	}
	for _, artifact := range result.Artifacts {
		log.Printf("built %s", artifact)
	}
	return nil
}

func isWebFlutterBuild(flutterArgs []string) bool {
	return len(flutterArgs) > 0 && strings.EqualFold(strings.TrimSpace(flutterArgs[0]), "web")
}

func newRunCommand(flags *generateFlags) *cobra.Command {
	options := &runFlags{}
	command := &cobra.Command{
		Use:   "run [flags] [-- flutter run args]",
		Short: "Run the Flutter app and restart it whenever the Go API changes",
		Long: `Run the Flutter app, regenerating the bridge and restarting the app whenever
the watched Go sources change.

A rebuilt Go dynamic library cannot be swapped into a live process: hot reload
and hot restart only rebuild the Dart isolate, the already-loaded library stays
resident, and on Android the new .so has not even been pushed to the device.
Only relaunching the process re-links the app against the regenerated code, so
that is what a Go change triggers here. Dart changes still use a hot reload.

Arguments after -- are forwarded to flutter run:

  flutter_go_bridge_codegen run -d emulator-5554 -- --flavor dev

Note that "-d all" is rejected: the underlying flutter daemon protocol runs one
device per session.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			var flutterArgs []string
			if index := command.ArgsLenAtDash(); index >= 0 {
				flutterArgs = args[index:]
			}
			return runFlutter(command, flags, options, flutterArgs)
		},
	}
	registerGenerateFlags(command, flags, false)
	command.Flags().StringVarP(&options.deviceID, "device-id", "d", "", "Target device id, forwarded to flutter run")
	command.Flags().StringVar(&options.projectDir, "project-dir", "", "Flutter project to launch (default the config base dir, or its example/ for plugin projects)")
	command.Flags().StringSliceVar(&options.dartInput, "dart-input", nil, "Dart directories watched for hot reload (default the project's lib/)")
	command.Flags().DurationVar(&options.interval, "watch-interval", 0, "File polling interval (default 400ms)")
	return command
}

func runFlutter(command *cobra.Command, flags *generateFlags, options *runFlags, flutterArgs []string) error {
	resolved, err := resolveGenerateConfig(command, flags)
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved.GoInput); err != nil {
		return fmt.Errorf("run requires go_input to be a local file or directory, got %q", resolved.GoInput)
	}
	projectDir, err := resolveProjectDir(resolved.BaseDir, options.projectDir)
	if err != nil {
		return err
	}
	if options.deviceID != "" {
		flutterArgs = append([]string{"-d", options.deviceID}, flutterArgs...)
	}
	webRun := isWebFlutterRun(options.deviceID, flutterArgs)

	// Ctrl+C must reach the loop rather than the process, so the app can be
	// stopped in order instead of leaving gradle and adb behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return devrun.Run(ctx, devrun.Options{
		WorkDir:     projectDir,
		FlutterArgs: flutterArgs,
		GoRoots:     []string{resolved.GoInput},
		DartRoots:   resolveDartRoots(resolved.BaseDir, projectDir, options.dartInput),
		Exclude:     []string{resolved.GoOutput, resolved.DartOutput},
		Interval:    options.interval,
		Generate: func() ([]string, error) {
			files, err := runGenerateFiles(command, flags)
			if err != nil || !webRun {
				return files, err
			}
			current, err := resolveGenerateConfig(command, flags)
			if err != nil {
				return files, err
			}
			wasmFiles, err := buildWebWasm(ctx, current)
			return append(files, wasmFiles...), err
		},
	})
}

// isWebFlutterRun recognizes the explicit Flutter Web device forms accepted by
// `flutter run`. Native runs deliberately do not build Wasm: a pure-Go Web
// build can be a stricter target than the native cgo package and must not make
// Windows or Android startup depend on it.
func isWebFlutterRun(deviceID string, flutterArgs []string) bool {
	if isWebDeviceID(deviceID) {
		return true
	}
	for index, arg := range flutterArgs {
		if arg != "-d" && arg != "--device-id" {
			if arg == "--wasm" || arg == "--web-renderer" {
				return true
			}
			continue
		}
		if index+1 < len(flutterArgs) && isWebDeviceID(flutterArgs[index+1]) {
			return true
		}
	}
	return false
}

func isWebDeviceID(deviceID string) bool {
	switch strings.ToLower(strings.TrimSpace(deviceID)) {
	case "chrome", "edge", "web-server", "web-javascript", "web-wasm":
		return true
	default:
		return false
	}
}

// buildWebWasm reuses the same Web platform implementation as the synchronous
// build command, but intentionally stops before `flutter build`: `run` owns a
// live Flutter daemon instead.
func buildWebWasm(ctx context.Context, resolved config.Resolved) ([]string, error) {
	log.Printf("building Web Wasm with Gokit")
	result, err := newWebPlatformBuilder().BuildWasm(ctx, platformbuild.Request{
		Platform:    platformbuild.PlatformWeb,
		BaseDir:     resolved.BaseDir,
		ManifestDir: filepath.Dir(resolved.GoOutput),
	})
	if err != nil {
		return nil, err
	}
	return result.Artifacts, nil
}

// resolveProjectDir picks the Flutter project to launch. Plugin projects keep
// their runnable app under example/, so fall back to it when the base
// directory has no entrypoint of its own.
func resolveProjectDir(baseDir, override string) (string, error) {
	if override != "" {
		dir := override
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "pubspec.yaml")); err != nil {
			return "", fmt.Errorf("--project-dir %q is not a Flutter project: %w", override, err)
		}
		return dir, nil
	}
	if _, err := os.Stat(filepath.Join(baseDir, "lib", "main.dart")); err == nil {
		return baseDir, nil
	}
	example := filepath.Join(baseDir, "example")
	if _, err := os.Stat(filepath.Join(example, "lib", "main.dart")); err == nil {
		log.Printf("no lib/main.dart in %s, running the example app in %s", baseDir, example)
		return example, nil
	}
	return baseDir, nil
}

// resolveDartRoots returns the trees watched for hot reload. A plugin project
// has two of them: the plugin's own lib/ and the example app's.
func resolveDartRoots(baseDir, projectDir string, override []string) []string {
	if len(override) > 0 {
		roots := make([]string, 0, len(override))
		for _, root := range override {
			if !filepath.IsAbs(root) {
				root = filepath.Join(baseDir, root)
			}
			roots = append(roots, root)
		}
		return roots
	}
	roots := []string{filepath.Join(baseDir, "lib")}
	if projectDir != baseDir {
		roots = append(roots, filepath.Join(projectDir, "lib"))
	}
	return roots
}

type createFlags struct {
	org         string
	template    string
	libraryName string
	goModDir    string
	platforms   string
}

func newCreateCommand() *cobra.Command {
	flags := &createFlags{}
	command := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new Flutter + Go (Gokit) project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			template, err := integrate.ParseTemplate(flags.template)
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return integrate.Create(integrate.CreateConfig{
				Name:        args[0],
				Org:         flags.org,
				WorkDir:     cwd,
				Template:    template,
				LibraryName: flags.libraryName,
				GoModDir:    flags.goModDir,
				Platforms:   flags.platforms,
			})
		},
	}
	command.Flags().StringVar(&flags.org, "org", "", "The organization responsible for the new project, in reverse domain name notation")
	command.Flags().StringVarP(&flags.template, "template", "t", "app", "The template type for the new project (app or plugin)")
	command.Flags().StringVar(&flags.libraryName, "library-name", "", "Go module/dynamic library name (default go_lib_<name> for app, <name> for plugin)")
	command.Flags().StringVar(&flags.goModDir, "go-mod-dir", "", "Directory of the Go module, relative to the project path (default \"go\")")
	command.Flags().StringVar(&flags.platforms, "platforms", "", "Comma-separated platforms to support (default auto-detected)")
	return command
}

type integrateFlags struct {
	template          string
	libraryName       string
	goModDir          string
	platforms         string
	noWriteLib        bool
	noIntegrationTest bool
	noDartFix         bool
	noDartFormat      bool
}

func newIntegrateCommand() *cobra.Command {
	flags := &integrateFlags{}
	command := &cobra.Command{
		Use:   "integrate",
		Short: "Integrate Go via Gokit into an existing Flutter project",
		RunE: func(*cobra.Command, []string) error {
			template, err := integrate.ParseTemplate(flags.template)
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return integrate.Run(integrate.Config{
				WorkDir:               cwd,
				Template:              template,
				LibraryName:           flags.libraryName,
				GoModDir:              flags.goModDir,
				Platforms:             flags.platforms,
				EnableWriteLib:        !flags.noWriteLib,
				EnableIntegrationTest: !flags.noIntegrationTest,
				EnableDartFix:         !flags.noDartFix,
				EnableDartFormat:      !flags.noDartFormat,
			})
		},
	}
	command.Flags().StringVarP(&flags.template, "template", "t", "app", "Template matching the Flutter project type (app or plugin)")
	command.Flags().StringVar(&flags.libraryName, "library-name", "", "Go module/dynamic library name (default go_lib_<pubspec name> for app, <pubspec name> for plugin)")
	command.Flags().StringVar(&flags.goModDir, "go-mod-dir", "", "Directory of the Go module, relative to the project path (default \"go\")")
	command.Flags().StringVar(&flags.platforms, "platforms", "", "Comma-separated platforms to support (default auto-detected)")
	command.Flags().BoolVar(&flags.noWriteLib, "no-write-lib", false, "Do not generate code related to lib/example etc.")
	command.Flags().BoolVar(&flags.noIntegrationTest, "no-integration-test", false, "Do not generate code related to integration test")
	command.Flags().BoolVar(&flags.noDartFix, "no-dart-fix", false, "Do not apply dart fix after generating code")
	command.Flags().BoolVar(&flags.noDartFormat, "no-dart-format", false, "Do not format dart code after generating code")
	return command
}

func newGenerateCommand(flags *generateFlags) *cobra.Command {
	command := &cobra.Command{
		Use:   "generate",
		Short: "Generate a Go cgo bridge and pure-Dart API",
		RunE: func(command *cobra.Command, _ []string) error {
			if flags.watch {
				return runGenerateWatch(command, flags)
			}
			return runGenerate(command, flags)
		},
	}
	registerGenerateFlags(command, flags, true)
	command.Flags().BoolVar(&flags.watch, "watch", false, "Automatically re-generate whenever the input files change")
	return command
}

// registerGenerateFlags declares the configuration flags shared by `generate`
// and `run`. The single-letter forms are reserved for `generate`: `run` needs
// -d for the target device, matching `flutter run`.
func registerGenerateFlags(command *cobra.Command, flags *generateFlags, shorthands bool) {
	goInput, goOutput, dartOutput := "", "", ""
	if shorthands {
		goInput, goOutput, dartOutput = "i", "g", "d"
	}
	command.Flags().StringVar(&flags.configFile, "config-file", "", "Path to a flutter_go_bridge YAML/JSON config file")
	command.Flags().StringVar(&flags.target, "target", "", "Deprecated compatibility option; generate now emits Native and Web together")
	command.Flags().StringVarP(&flags.goInput, "go-input", goInput, "", "Go package directory, .go file, or package pattern")
	command.Flags().StringVarP(&flags.goOutput, "go-output", goOutput, "", "Generated Go bridge file (default bridge_generated.go beside the nearest go.mod)")
	command.Flags().StringVarP(&flags.dartOutput, "dart-output", dartOutput, "", "Generated Dart bridge file (default lib/src/bridge_generated.dart)")
	command.Flags().StringVar(&flags.libraryName, "library-name", "", "Dynamic library base name (defaults to go_lib_<pubspec.yaml name>, then Go module name)")
	command.Flags().StringVar(&flags.dartEntrypointClassName, "dart-entrypoint-class-name", "", "Generated Dart bridge class name")
	command.Flags().IntVar(&flags.dartFormatLineLength, "dart-format-line-length", 0, "Dart formatter line length")
	command.Flags().StringVar(&flags.dartPreamble, "dart-preamble", "", "Raw preamble inserted before generated Dart code")
	command.Flags().StringVar(&flags.goPreamble, "go-preamble", "", "Raw preamble inserted before generated Go code")
	command.Flags().BoolVar(&flags.noDartFormat, "no-dart-format", false, "Do not run dart format")
	command.Flags().BoolVar(&flags.stopOnError, "stop-on-error", true, "Stop on the first unsupported exported declaration")
	command.Flags().BoolVar(&flags.printAST, "print-ast", false, "Print official Go AST nodes while parsing")
}

// resolveGenerateConfig loads and merges the CLI, file, and default
// configuration exactly like a plain `generate` run.
func resolveGenerateConfig(command *cobra.Command, flags *generateFlags) (config.Resolved, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return config.Resolved{}, err
	}

	var fileConfig config.Config
	if flags.configFile != "" {
		fileConfig, err = config.LoadExplicit(flags.configFile)
		if err != nil {
			return config.Resolved{}, err
		}
	} else {
		fileConfig, err = config.LoadAuto(cwd)
		if err != nil && !errors.Is(err, config.ErrNotFound) {
			return config.Resolved{}, err
		}
	}
	merged := config.Merge(flags.toConfig(command), fileConfig)
	return config.Resolve(merged, cwd)
}

// runGenerateWatch keeps re-running the generator whenever the Go input tree
// changes. The configuration is reloaded on every cycle, so edits to the
// config file take effect on the next run; only the watched paths are fixed
// at startup.
func runGenerateWatch(command *cobra.Command, flags *generateFlags) error {
	resolved, err := resolveGenerateConfig(command, flags)
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved.GoInput); err != nil {
		return fmt.Errorf("--watch requires go_input to be a local file or directory, got %q", resolved.GoInput)
	}
	return watcher.RunContext(command.Context(), watcher.Options{
		Roots:   []string{resolved.GoInput},
		Exclude: []string{resolved.GoOutput, resolved.DartOutput},
	}, func() error {
		return runGenerate(command, flags)
	})
}

func runGenerate(command *cobra.Command, flags *generateFlags) error {
	_, err := runGenerateFiles(command, flags)
	return err
}

// runGenerateFiles performs one generation and reports every file it wrote,
// including the support package. `run` needs that list to keep generated
// output from registering as a hand edit on the next watch tick.
func runGenerateFiles(command *cobra.Command, flags *generateFlags) ([]string, error) {
	resolved, err := resolveGenerateConfig(command, flags)
	if err != nil {
		return nil, err
	}
	// The support package must exist before the Go input is loaded: an API
	// that references it cannot be type-checked until the package is on disk.
	supportPath, err := generator.WriteSupportPackage(resolved)
	if err != nil {
		return nil, err
	}
	log.Printf("generated %s", supportPath)
	log.Printf("loading Go package %s", resolved.GoInput)
	api, err := parser.Parse(parser.Options{
		Input: resolved.GoInput, BaseDir: resolved.BaseDir,
		Target:   config.TargetWeb,
		PrintAST: flags.printAST, ASTOut: os.Stdout,
	})
	if err != nil {
		return nil, err
	}
	result, err := generator.GenerateAll(api, resolved)
	for _, warning := range result.Warnings {
		log.Printf("warning: %v", warning)
	}
	if err != nil {
		return nil, err
	}
	for _, path := range result.Files {
		log.Printf("generated %s", path)
	}
	if err := ensureDartDependencies(resolved.BaseDir, result.DartDependencies); err != nil {
		return nil, err
	}
	if resolved.DartFormat {
		if err := formatDart(result.Files, resolved.DartFormatLineLength); err != nil {
			log.Printf("warning: dart format skipped: %v", err)
		}
	}
	return append([]string{supportPath}, result.Files...), nil
}

func ensureDartDependencies(projectDir string, dependencies []string) error {
	if len(dependencies) == 0 {
		return nil
	}
	pubspecPath := filepath.Join(projectDir, "pubspec.yaml")
	raw, err := os.ReadFile(pubspecPath)
	if err != nil {
		return fmt.Errorf("read pubspec for generated Dart dependencies: %w", err)
	}
	var pubspec struct {
		Dependencies map[string]any `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(raw, &pubspec); err != nil {
		return fmt.Errorf("parse %s: %w", pubspecPath, err)
	}
	for _, dependency := range dependencies {
		if _, exists := pubspec.Dependencies[dependency]; exists {
			continue
		}
		name := "flutter"
		args := []string{"pub", "add", dependency}
		if _, lookErr := exec.LookPath(name); lookErr != nil {
			if _, fvmErr := exec.LookPath("fvm"); fvmErr != nil {
				return fmt.Errorf("generated Dart code requires package %q; neither flutter nor fvm was found to run pub add", dependency)
			}
			name = "fvm"
			args = append([]string{"flutter"}, args...)
		}
		log.Printf("execute `%s %s` in %s", name, strings.Join(args, " "), projectDir)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		command := exec.CommandContext(ctx, name, args...)
		command.Dir = projectDir
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		command.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true", "CI=true")
		runErr := command.Run()
		cancel()
		if runErr != nil {
			// Pub may update pubspec before dependency resolution finishes. Treat
			// that partial success as installed for generation purposes; the user
			// can run pub get later if the package download itself was interrupted.
			if declared, checkErr := dartDependencyDeclared(pubspecPath, dependency); checkErr == nil && declared {
				log.Printf("warning: `%s %s` returned %v after adding %s to pubspec.yaml", name, strings.Join(args, " "), runErr, dependency)
				continue
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("`%s %s` timed out", name, strings.Join(args, " "))
			}
			return fmt.Errorf("`%s %s` failed: %w", name, strings.Join(args, " "), runErr)
		}
		// Keep the in-memory view current when several dependencies are added.
		if pubspec.Dependencies == nil {
			pubspec.Dependencies = map[string]any{}
		}
		pubspec.Dependencies[dependency] = true
	}
	return nil
}

func dartDependencyDeclared(pubspecPath, dependency string) (bool, error) {
	raw, err := os.ReadFile(pubspecPath)
	if err != nil {
		return false, err
	}
	var pubspec struct {
		Dependencies map[string]any `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(raw, &pubspec); err != nil {
		return false, err
	}
	_, exists := pubspec.Dependencies[dependency]
	return exists, nil
}

func (flags *generateFlags) toConfig(command *cobra.Command) config.Config {
	result := config.Config{}
	setString := func(value string) *string {
		if value == "" {
			return nil
		}
		return &value
	}
	result.Target = setString(flags.target)
	result.GoInput = setString(flags.goInput)
	result.GoOutput = setString(flags.goOutput)
	result.DartOutput = setString(flags.dartOutput)
	result.LibraryName = setString(flags.libraryName)
	result.DartEntrypointClassName = setString(flags.dartEntrypointClassName)
	result.DartPreamble = setString(flags.dartPreamble)
	result.GoPreamble = setString(flags.goPreamble)
	if command.Flags().Changed("dart-format-line-length") {
		result.DartFormatLineLength = &flags.dartFormatLineLength
	}
	if command.Flags().Changed("no-dart-format") {
		value := !flags.noDartFormat
		result.DartFormat = &value
	}
	if command.Flags().Changed("stop-on-error") {
		value := flags.stopOnError
		result.StopOnError = &value
	}
	return result
}

func formatDart(paths []string, lineLength int) error {
	dart := integrate.FindDartExecutable()
	args := []string{"format"}
	if lineLength > 0 {
		args = append(args, "--line-length", strconv.Itoa(lineLength))
	}
	for _, path := range paths {
		if filepath.Ext(path) == ".dart" {
			args = append(args, filepath.Clean(path))
		}
	}
	if len(args) == 1 || (len(args) == 3 && lineLength > 0) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, dart, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true", "CI=true")
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("dart format timed out after 30 seconds")
		}
		return err
	}
	return nil
}
