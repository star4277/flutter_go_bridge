package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	"github.com/star4277/flutter-go-bridge-gokit/internal/generator"
	"github.com/star4277/flutter-go-bridge-gokit/internal/parser"
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
	root.AddCommand(deferredCommand("create", "Project creation is intentionally deferred; run flutter_go_bridge_codegen generate."))
	root.AddCommand(deferredCommand("integrate", "Project integration is intentionally deferred; configure gokit and run generate."))
	return root
}

func newGenerateCommand(flags *generateFlags) *cobra.Command {
	command := &cobra.Command{
		Use:   "generate",
		Short: "Generate a Go cgo bridge and pure-Dart API",
		RunE: func(command *cobra.Command, _ []string) error {
			if flags.watch {
				return errors.New("--watch is reserved for a future release")
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
	command.Flags().BoolVar(&flags.watch, "watch", false, "Reserved for a future release")
	return command
}

func deferredCommand(name, message string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: message,
		RunE:  func(*cobra.Command, []string) error { return errors.New(message) },
	}
}

func runGenerate(command *cobra.Command, flags *generateFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	var fileConfig config.Config
	if flags.configFile != "" {
		fileConfig, err = config.LoadExplicit(flags.configFile)
		if err != nil {
			return err
		}
	} else {
		fileConfig, err = config.LoadAuto(cwd)
		if err != nil && !errors.Is(err, config.ErrNotFound) {
			return err
		}
	}
	cliConfig := flags.toConfig(command)
	merged := config.Merge(cliConfig, fileConfig)
	resolved, err := config.Resolve(merged, cwd)
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
	dart, err := findDartExecutable()
	if err != nil {
		return err
	}
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

func findDartExecutable() (string, error) {
	dart, err := exec.LookPath("dart")
	if err != nil {
		return "", err
	}
	// Flutter's Windows `dart.bat` wrapper performs SDK/bootstrap checks before
	// launching dart.exe. Those checks are unnecessary for formatting generated
	// files and can hang in non-interactive codegen runs. Prefer the SDK binary
	// living beside the wrapper when it exists.
	if ext := strings.ToLower(filepath.Ext(dart)); runtime.GOOS == "windows" && (ext == ".bat" || ext == ".cmd") {
		candidate := filepath.Join(filepath.Dir(dart), "cache", "dart-sdk", "bin", "dart.exe")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return dart, nil
}
