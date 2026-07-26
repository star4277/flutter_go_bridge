package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrNotFound = errors.New("flutter_go_bridge configuration not found")

var autoConfigNames = []string{
	".flutter_go_bridge.yml",
	".flutter_go_bridge.yaml",
	".flutter_go_bridge.json",
	"flutter_go_bridge.yml",
	"flutter_go_bridge.yaml",
	"flutter_go_bridge.json",
}

type Config struct {
	BaseDir                 *string `yaml:"base_dir" json:"base_dir"`
	GoInput                 *string `yaml:"go_input" json:"go_input"`
	GoOutput                *string `yaml:"go_output" json:"go_output"`
	DartOutput              *string `yaml:"dart_output" json:"dart_output"`
	LibraryName             *string `yaml:"library_name" json:"library_name"`
	DartEntrypointClassName *string `yaml:"dart_entrypoint_class_name" json:"dart_entrypoint_class_name"`
	DartFormatLineLength    *int    `yaml:"dart_format_line_length" json:"dart_format_line_length"`
	DartPreamble            *string `yaml:"dart_preamble" json:"dart_preamble"`
	GoPreamble              *string `yaml:"go_preamble" json:"go_preamble"`
	DartFormat              *bool   `yaml:"dart_format" json:"dart_format"`
	StopOnError             *bool   `yaml:"stop_on_error" json:"stop_on_error"`
}

type Resolved struct {
	BaseDir                 string
	GoInput                 string
	GoOutput                string
	DartOutput              string
	LibraryName             string
	DartEntrypointClassName string
	DartFormatLineLength    int
	DartPreamble            string
	GoPreamble              string
	DartFormat              bool
	StopOnError             bool
}

func Merge(high, low Config) Config {
	return Config{
		BaseDir:                 first(high.BaseDir, low.BaseDir),
		GoInput:                 first(high.GoInput, low.GoInput),
		GoOutput:                first(high.GoOutput, low.GoOutput),
		DartOutput:              first(high.DartOutput, low.DartOutput),
		LibraryName:             first(high.LibraryName, low.LibraryName),
		DartEntrypointClassName: first(high.DartEntrypointClassName, low.DartEntrypointClassName),
		DartFormatLineLength:    first(high.DartFormatLineLength, low.DartFormatLineLength),
		DartPreamble:            first(high.DartPreamble, low.DartPreamble),
		GoPreamble:              first(high.GoPreamble, low.GoPreamble),
		DartFormat:              first(high.DartFormat, low.DartFormat),
		StopOnError:             first(high.StopOnError, low.StopOnError),
	}
}

func first[T any](values ...*T) *T {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func LoadExplicit(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	config, err := decodeStrict(raw)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, err
	}
	config.BaseDir = &dir
	return config, nil
}

func LoadAuto(cwd string) (Config, error) {
	for _, name := range autoConfigNames {
		path := filepath.Join(cwd, name)
		if _, err := os.Stat(path); err == nil {
			return LoadExplicit(path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	pubspecPath := filepath.Join(cwd, "pubspec.yaml")
	raw, err := os.ReadFile(pubspecPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, ErrNotFound
		}
		return Config{}, err
	}

	var root map[string]yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", pubspecPath, err)
	}
	section, found := root["flutter_go_bridge"]
	if !found {
		return Config{}, ErrNotFound
	}
	sectionRaw, err := yaml.Marshal(&section)
	if err != nil {
		return Config{}, err
	}
	result, err := decodeStrict(sectionRaw)
	if err != nil {
		return Config{}, fmt.Errorf("parse flutter_go_bridge in %s: %w", pubspecPath, err)
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Config{}, err
	}
	result.BaseDir = &abs
	return result, nil
}

func decodeStrict(raw []byte) (Config, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var result Config
	if err := decoder.Decode(&result); err != nil {
		return Config{}, err
	}
	return result, nil
}

func Resolve(raw Config, cwd string) (Resolved, error) {
	baseDir := cwd
	if raw.BaseDir != nil && *raw.BaseDir != "" {
		baseDir = *raw.BaseDir
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return Resolved{}, err
	}

	if raw.GoInput == nil || *raw.GoInput == "" {
		return Resolved{}, errors.New("go_input is required")
	}
	// The bridge is a complete package-main cgo entrypoint. Keep it beside
	// go.mod instead of creating a secondary bridge package.
	goOutput := ""
	if raw.GoOutput != nil && *raw.GoOutput != "" {
		goOutput = *raw.GoOutput
	} else {
		goOutput = filepath.Join(defaultGoModuleRoot(absBase, *raw.GoInput), "bridge_generated.go")
	}
	// Keep the generated implementation under lib/src.  This matches the
	// layout used by real Flutter applications while leaving lib/ as the
	// public-facing package surface.
	dartOutput := filepath.Join("lib", "src", "bridge_generated.dart")
	if raw.DartOutput != nil && *raw.DartOutput != "" {
		dartOutput = *raw.DartOutput
	}

	result := Resolved{
		BaseDir:                 absBase,
		GoInput:                 resolvePathIfLocal(absBase, *raw.GoInput),
		GoOutput:                resolvePath(absBase, goOutput),
		DartOutput:              resolvePath(absBase, dartOutput),
		DartEntrypointClassName: "FlutterGoBridge",
		DartFormatLineLength:    100,
		DartFormat:              true,
		StopOnError:             true,
	}
	if raw.LibraryName != nil && strings.TrimSpace(*raw.LibraryName) != "" {
		result.LibraryName = strings.TrimSpace(*raw.LibraryName)
	} else {
		// A Flutter application's pubspec name is the authoritative project
		// name; CGO libraries use the go_lib_<project> convention. Keep the
		// empty value when no pubspec is present so generator-level Go module
		// fallback remains available for
		// standalone Go projects.
		if name := pubspecName(absBase); name != "" {
			result.LibraryName = goLibraryName(name)
		}
	}
	if raw.DartEntrypointClassName != nil && *raw.DartEntrypointClassName != "" {
		result.DartEntrypointClassName = *raw.DartEntrypointClassName
	}
	if raw.DartFormatLineLength != nil {
		if *raw.DartFormatLineLength < 40 {
			return Resolved{}, errors.New("dart_format_line_length must be at least 40")
		}
		result.DartFormatLineLength = *raw.DartFormatLineLength
	}
	if raw.DartPreamble != nil {
		result.DartPreamble = *raw.DartPreamble
	}
	if raw.GoPreamble != nil {
		result.GoPreamble = *raw.GoPreamble
	}
	if raw.DartFormat != nil {
		result.DartFormat = *raw.DartFormat
	}
	if raw.StopOnError != nil {
		result.StopOnError = *raw.StopOnError
	}
	return result, nil
}

func goLibraryName(projectName string) string {
	projectName = strings.TrimSpace(projectName)
	if strings.HasPrefix(projectName, "rust_lib_") {
		projectName = strings.TrimPrefix(projectName, "rust_lib_")
	}
	if strings.HasPrefix(projectName, "go_lib_") {
		return projectName
	}
	return "go_lib_" + projectName
}

// pubspecName reads the package name without requiring the caller to load the
// whole Flutter configuration section.  Missing or malformed pubspec files
// deliberately return an empty string: a standalone Go invocation can still
// derive a stable name from go.mod in the generator layer.
func pubspecName(baseDir string) string {
	raw, err := os.ReadFile(filepath.Join(baseDir, "pubspec.yaml"))
	if err != nil {
		return ""
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	return strings.TrimSpace(metadata.Name)
}

// defaultGoModuleRoot finds the module containing a local Go input. Flutter
// projects commonly keep Go under a nested directory (for example go/api),
// while the bridge must live beside that directory's go.mod. Package patterns
// and non-existent paths fall back to the configuration base directory.
func defaultGoModuleRoot(baseDir, goInput string) string {
	candidate := goInput
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	info, err := os.Stat(candidate)
	if err == nil && !info.IsDir() {
		candidate = filepath.Dir(candidate)
	}
	if err != nil {
		candidate = filepath.Dir(candidate)
	}
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		candidate = filepath.Dir(candidate)
	}
	for {
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		candidate = parent
	}
	return baseDir
}

func resolvePath(base, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(base, value)
}

func resolvePathIfLocal(base, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	candidate := filepath.Join(base, value)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return value
}
