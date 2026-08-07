package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/model"
	"github.com/star4277/flutter_go_bridge/internal/names"
)

type Result struct {
	Files            []string
	Warnings         []error
	DartDependencies []string
}

// GenerateAll emits both platform implementations and the conditional Dart
// facades that select Native or Web at compile time. The existing Generate
// function remains the single-target renderer used by focused generator tests
// and by callers that intentionally need one platform only.
func GenerateAll(api *model.API, resolved config.Resolved) (Result, error) {
	if filepath.Ext(resolved.GoOutput) != ".go" {
		return Result{}, fmt.Errorf("go_output must be a .go file: %s", resolved.GoOutput)
	}
	if resolved.LibraryName == "" {
		modulePath := api.Package.PkgPath
		if api.Package.Module != nil && api.Package.Module.Path != "" {
			modulePath = api.Package.Module.Path
		}
		resolved.LibraryName = config.GoLibraryName(names.LibraryBase(modulePath))
	}
	outputDir := filepath.Dir(resolved.GoOutput)
	direct := samePath(outputDir, api.InputDir)
	if direct && api.Package.Name != "main" {
		return Result{}, fmt.Errorf("go_output is inside input package %q; keep the API in a subpackage and place bridge_generated.go beside go.mod, or make the input package main", api.Package.Name)
	}
	if !direct && api.Package.Name == "main" {
		return Result{}, fmt.Errorf("package main cannot be imported by a separate bridge package; place go_output in %s", api.InputDir)
	}

	native := resolved
	native.Target = config.TargetNative
	web := resolved
	web.Target = config.TargetWeb
	web.GoOutput = webGoOutputPath(resolved.GoOutput)

	nativeUnit, warnings, err := buildUnit(api, native, direct)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	webUnit, _, err := buildUnit(api, web, direct)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	available := *api
	available.Callables = nil
	for _, call := range webUnit.Calls {
		if call.TargetAvailable {
			available.Callables = append(available.Callables, call.Source)
		} else {
			warnings = append(warnings, fmt.Errorf("%s is unavailable on Web: %s", call.GoName, call.TargetReason))
		}
	}
	webGoUnit, _, err := buildUnit(&available, web, direct)
	if err != nil {
		return Result{Warnings: warnings}, err
	}

	nativeGo, err := renderGo(nativeUnit)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	nativeGo, err = format.Source(append([]byte("//go:build !(js && wasm)\n\n"), nativeGo...))
	if err != nil {
		return Result{Warnings: warnings}, fmt.Errorf("format generated Native Go code: %w\n%s", err, numberedSource(nativeGo))
	}
	webGo, err := renderGoWeb(webGoUnit)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	webGo, err = format.Source(append([]byte("//go:build js && wasm\n\n"), webGo...))
	if err != nil {
		return Result{Warnings: warnings}, fmt.Errorf("format generated Web Go code: %w\n%s", err, numberedSource(webGo))
	}
	dartFiles, err := renderDartDual(nativeUnit, webUnit, resolved.DartOutput)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	metadata, err := webBridgeMetadata(api, web)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	files := map[string][]byte{
		resolved.GoOutput: nativeGo,
		web.GoOutput:      webGo,
		filepath.Join(filepath.Dir(resolved.GoOutput), "fgb_web_build.json"): metadata,
	}
	for path, content := range dartFiles {
		files[path] = content
	}
	dependencies := []string{}
	if nativeUnit.UsesUUID {
		dependencies = append(dependencies, "uuid")
	}
	if nativeUnit.UsesDecimal {
		dependencies = append(dependencies, "decimal")
	}
	return writeGeneratedFiles(files, Result{Warnings: warnings, DartDependencies: dependencies})
}

func webGoOutputPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "_web" + ext
}

func Generate(api *model.API, resolved config.Resolved) (Result, error) {
	if filepath.Ext(resolved.GoOutput) != ".go" {
		return Result{}, fmt.Errorf("go_output must be a .go file: %s", resolved.GoOutput)
	}
	if resolved.LibraryName == "" {
		modulePath := api.Package.PkgPath
		if api.Package.Module != nil && api.Package.Module.Path != "" {
			modulePath = api.Package.Module.Path
		}
		baseName := names.LibraryBase(modulePath)
		resolved.LibraryName = config.GoLibraryName(baseName)
	}

	outputDir := filepath.Dir(resolved.GoOutput)
	direct := samePath(outputDir, api.InputDir)
	if direct && api.Package.Name != "main" {
		return Result{}, fmt.Errorf("go_output is inside input package %q; keep the API in a subpackage and place bridge_generated.go beside go.mod, or make the input package main", api.Package.Name)
	}
	if !direct && api.Package.Name == "main" {
		return Result{}, fmt.Errorf("package main cannot be imported by a separate bridge package; place go_output in %s", api.InputDir)
	}

	unit, warnings, err := buildUnit(api, resolved, direct)
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	dartDependencies := []string{}
	if unit.UsesUUID {
		dartDependencies = append(dartDependencies, "uuid")
	}
	if unit.UsesDecimal {
		dartDependencies = append(dartDependencies, "decimal")
	}

	goUnit := unit
	if resolved.Target == config.TargetWeb {
		available := *api
		available.Callables = nil
		for _, call := range unit.Calls {
			if call.TargetAvailable {
				available.Callables = append(available.Callables, call.Source)
			} else {
				warnings = append(warnings, fmt.Errorf("%s is unavailable on Web: %s", call.GoName, call.TargetReason))
			}
		}
		goUnit, _, err = buildUnit(&available, resolved, direct)
		if err != nil {
			return Result{Warnings: warnings}, err
		}
	}

	var goSource []byte
	if resolved.Target == config.TargetWeb {
		goSource, err = renderGoWeb(goUnit)
	} else {
		goSource, err = renderGo(goUnit)
	}
	if err != nil {
		return Result{Warnings: warnings}, err
	}
	formattedGo, err := format.Source(goSource)
	if err != nil {
		return Result{Warnings: warnings}, fmt.Errorf("format generated Go code: %w\n%s", err, numberedSource(goSource))
	}
	dartFiles, err := renderDartSplit(unit, resolved.DartOutput)
	if err != nil {
		return Result{Warnings: warnings}, err
	}

	files := map[string][]byte{
		resolved.GoOutput: formattedGo,
	}
	if resolved.Target == config.TargetWeb {
		metadata, metadataErr := webBridgeMetadata(api, resolved)
		if metadataErr != nil {
			return Result{Warnings: warnings}, metadataErr
		}
		files[filepath.Join(filepath.Dir(resolved.GoOutput), "fgb_web_build.json")] = metadata
	}
	for path, content := range dartFiles {
		files[path] = content
	}
	result := Result{Warnings: warnings, DartDependencies: dartDependencies}
	return writeGeneratedFiles(files, result)
}

func writeGeneratedFiles(files map[string][]byte, result Result) (Result, error) {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	temporary := make(map[string]string, len(paths))
	defer func() {
		for _, path := range temporary {
			_ = os.Remove(path)
		}
	}()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return result, err
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".fgb-*")
		if err != nil {
			return result, err
		}
		tempPath := file.Name()
		if _, err := file.Write(files[path]); err != nil {
			file.Close()
			return result, err
		}
		if err := file.Chmod(0o644); err != nil {
			file.Close()
			return result, err
		}
		if err := file.Close(); err != nil {
			return result, err
		}
		temporary[path] = tempPath
	}
	for _, path := range paths {
		if err := os.Rename(temporary[path], path); err != nil {
			return result, err
		}
		delete(temporary, path)
		result.Files = append(result.Files, path)
	}
	return result, nil
}

func webBridgeMetadata(api *model.API, resolved config.Resolved) ([]byte, error) {
	hash := sha256.New()
	for _, source := range api.SourceFiles {
		content, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read Web bridge source %s: %w", source, err)
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(source)))
		_, _ = hash.Write(content)
	}
	payload := map[string]any{
		"protocol_version":  1,
		"generator_version": "flutter_go_bridge_codegen",
		"target":            string(resolved.Target),
		"library_name":      resolved.LibraryName,
		"api_hash":          hex.EncodeToString(hash.Sum(nil)),
	}
	return json.MarshalIndent(payload, "", "  ")
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	leftClean, rightClean := filepath.Clean(leftAbs), filepath.Clean(rightAbs)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(leftClean, rightClean)
	}
	return leftClean == rightClean
}

func numberedSource(source []byte) string {
	lines := strings.Split(string(source), "\n")
	var output strings.Builder
	for index, line := range lines {
		fmt.Fprintf(&output, "%4d | %s\n", index+1, line)
	}
	return output.String()
}
