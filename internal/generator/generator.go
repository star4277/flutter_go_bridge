package generator

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	"github.com/star4277/flutter-go-bridge-gokit/internal/model"
	"github.com/star4277/flutter-go-bridge-gokit/internal/names"
)

type Result struct {
	Files    []string
	Warnings []error
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

	goSource, err := renderGo(unit)
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
	for path, content := range dartFiles {
		files[path] = content
	}
	result := Result{Warnings: warnings}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return result, err
		}
		result.Files = append(result.Files, path)
	}
	sort.Strings(result.Files)
	return result, nil
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func numberedSource(source []byte) string {
	lines := strings.Split(string(source), "\n")
	var output strings.Builder
	for index, line := range lines {
		fmt.Fprintf(&output, "%4d | %s\n", index+1, line)
	}
	return output.String()
}
