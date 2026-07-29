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
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/devrun"
	"github.com/star4277/flutter_go_bridge/internal/generator"
	"github.com/star4277/flutter_go_bridge/internal/integrate"
	"github.com/star4277/flutter_go_bridge/internal/parser"
	"github.com/star4277/flutter_go_bridge/internal/watcher"
)

var version = "0.1.0"

func main() {
	if err := newRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type generateFlags struct {
	configFile              string
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
		Version: version,
		Short:   "Generate pure-Dart bindings for a Go library built by Gokit",
	}
	root.AddCommand(newGenerateCommand(flags))
	root.AddCommand(newRunCommand(&generateFlags{}))
	root.AddCommand(newCreateCommand())
	root.AddCommand(newIntegrateCommand())
	return root
}

type runFlags struct {
	deviceID   string
	projectDir string
	dartInput  []string
	interval   time.Duration
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
			return runGenerateFiles(command, flags)
		},
	})
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
	return watcher.Run(watcher.Options{
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
		PrintAST: flags.printAST, ASTOut: os.Stdout,
	})
	if err != nil {
		return nil, err
	}
	result, err := generator.Generate(api, resolved)
	for _, warning := range result.Warnings {
		log.Printf("warning: %v", warning)
	}
	if err != nil {
		return nil, err
	}
	for _, path := range result.Files {
		log.Printf("generated %s", path)
	}
	if resolved.DartFormat {
		if err := formatDart(result.Files, resolved.DartFormatLineLength); err != nil {
			log.Printf("warning: dart format skipped: %v", err)
		}
	}
	return append([]string{supportPath}, result.Files...), nil
}

func (flags *generateFlags) toConfig(command *cobra.Command) config.Config {
	result := config.Config{}
	setString := func(value string) *string {
		if value == "" {
			return nil
		}
		return &value
	}
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
