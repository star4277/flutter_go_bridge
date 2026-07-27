package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	"github.com/star4277/flutter-go-bridge-gokit/internal/generator"
	"github.com/star4277/flutter-go-bridge-gokit/internal/integrate"
	"github.com/star4277/flutter-go-bridge-gokit/internal/parser"
	"github.com/star4277/flutter-go-bridge-gokit/internal/watcher"
)

var version = "0.1.0"

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
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
	root.AddCommand(newCreateCommand())
	root.AddCommand(newIntegrateCommand())
	return root
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
	command.Flags().StringVar(&flags.configFile, "config-file", "", "Path to a flutter_go_bridge YAML/JSON config file")
	command.Flags().StringVarP(&flags.goInput, "go-input", "i", "", "Go package directory, .go file, or package pattern")
	command.Flags().StringVarP(&flags.goOutput, "go-output", "g", "", "Generated Go bridge file (default bridge_generated.go beside the nearest go.mod)")
	command.Flags().StringVarP(&flags.dartOutput, "dart-output", "d", "", "Generated Dart bridge file (default lib/src/bridge_generated.dart)")
	command.Flags().StringVar(&flags.libraryName, "library-name", "", "Dynamic library base name (defaults to go_lib_<pubspec.yaml name>, then Go module name)")
	command.Flags().StringVar(&flags.dartEntrypointClassName, "dart-entrypoint-class-name", "", "Generated Dart bridge class name")
	command.Flags().IntVar(&flags.dartFormatLineLength, "dart-format-line-length", 0, "Dart formatter line length")
	command.Flags().StringVar(&flags.dartPreamble, "dart-preamble", "", "Raw preamble inserted before generated Dart code")
	command.Flags().StringVar(&flags.goPreamble, "go-preamble", "", "Raw preamble inserted before generated Go code")
	command.Flags().BoolVar(&flags.noDartFormat, "no-dart-format", false, "Do not run dart format")
	command.Flags().BoolVar(&flags.stopOnError, "stop-on-error", true, "Stop on the first unsupported exported declaration")
	command.Flags().BoolVar(&flags.printAST, "print-ast", false, "Print official Go AST nodes while parsing")
	command.Flags().BoolVar(&flags.watch, "watch", false, "Automatically re-generate whenever the input files change")
	return command
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
	resolved, err := resolveGenerateConfig(command, flags)
	if err != nil {
		return err
	}
	log.Printf("loading Go package %s", resolved.GoInput)
	api, err := parser.Parse(parser.Options{
		Input: resolved.GoInput, BaseDir: resolved.BaseDir,
		PrintAST: flags.printAST, ASTOut: os.Stdout,
	})
	if err != nil {
		return err
	}
	result, err := generator.Generate(api, resolved)
	for _, warning := range result.Warnings {
		log.Printf("warning: %v", warning)
	}
	if err != nil {
		return err
	}
	for _, path := range result.Files {
		log.Printf("generated %s", path)
	}
	if resolved.DartFormat {
		if err := formatDart(result.Files, resolved.DartFormatLineLength); err != nil {
			log.Printf("warning: dart format skipped: %v", err)
		}
	}
	return nil
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
