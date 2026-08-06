package generator

import (
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
	result := Result{Warnings: warnings, DartDependencies: dartDependencies}
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
