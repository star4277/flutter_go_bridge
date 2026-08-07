// Package integrate injects Gokit-based Go support into an existing Flutter
// project, mirroring `flutter_rust_bridge_codegen integrate`.
package integrate

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/star4277/flutter_go_bridge/internal/config"
	templatefs "github.com/star4277/flutter_go_bridge/template"
)

// Template selects which project layout the overlay targets.
type Template string

const (
	TemplateApp    Template = "app"
	TemplatePlugin Template = "plugin"
)

// ParseTemplate converts a CLI value into a Template.
func ParseTemplate(value string) (Template, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(TemplateApp):
		return TemplateApp, nil
	case string(TemplatePlugin):
		return TemplatePlugin, nil
	}
	return "", fmt.Errorf("unknown template %q (expected app or plugin)", value)
}

// Config mirrors flutter_rust_bridge's IntegrateConfig.
type Config struct {
	// WorkDir is where the search for the enclosing Dart package starts.
	WorkDir string
	// Template should match the kind of Flutter project being integrated.
	Template Template
	// LibraryName overrides the Go module / dynamic library name. Empty means
	// go_lib_<pubspec name> for apps and <pubspec name> for plugins.
	LibraryName string
	// GoModDir is the project-relative directory of the Go module ("go").
	GoModDir string
	// Platforms is a comma-separated platform list; empty auto-detects.
	Platforms string

	EnableWriteLib        bool
	EnableIntegrationTest bool
	EnableDartFix         bool
	EnableDartFormat      bool
}

// Run integrates Go into the Flutter project that encloses cfg.WorkDir.
func Run(cfg Config) error {
	return run(cfg, templatefs.FS)
}

func run(cfg Config, templates fs.FS) error {
	if cfg.Template == "" {
		cfg.Template = TemplateApp
	}
	dartRoot, err := findDartPackageDir(cfg.WorkDir)
	if err != nil {
		return err
	}
	dartPackageName, err := readDartPackageName(dartRoot)
	if err != nil {
		return err
	}
	libraryName := cfg.LibraryName
	if libraryName == "" {
		// Apps follow the go_lib_<project> convention shared with the generate
		// command; a plugin is itself the library, exactly like FRB names the
		// plugin crate after the Dart package.
		if cfg.Template == TemplatePlugin {
			libraryName = dartPackageName
		} else {
			libraryName = config.GoLibraryName(dartPackageName)
		}
	}
	goModDir, err := normalizeGoModDir(cfg.GoModDir)
	if err != nil {
		return err
	}
	if err := ensureGokitTemplatePresent(templates, cfg.Template); err != nil {
		return err
	}
	platforms := resolveFlutterPlatforms(cfg.Template, cfg.Platforms)
	includeOhos := platformListContainsOhos(platforms)

	log.Printf("overlay template onto %s", dartRoot)
	replacements := computeReplacements(dartPackageName, libraryName, goModDir, includeOhos)
	if err := executeOverlayTemplates(templates, replacements, dartRoot, &cfg, includeOhos, dartPackageName); err != nil {
		return err
	}
	if err := ensureWebWasmAssets(dartRoot, cfg.Template); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		log.Printf("modify file permissions")
		if err := modifyPermissions(dartRoot, cfg.Template); err != nil {
			return err
		}
	}

	log.Printf("add pub dependencies")
	if err := pubAddDependencies(dartRoot, libraryName, &cfg); err != nil {
		return err
	}

	log.Printf("setup gokit build_tool dependencies")
	if err := flutterPubGet(filepath.Join(gokitDir(dartRoot, cfg.Template), "build_tool")); err != nil {
		return err
	}
	if err := excludeGokitFromOuterAnalyzer(dartRoot, cfg.Template); err != nil {
		return err
	}

	if cfg.EnableDartFix {
		log.Printf("apply Dart fixes")
		if err := dartFix(dartRoot); err != nil {
			return err
		}
	} else {
		log.Printf("dart fix is disabled")
	}
	if cfg.EnableDartFormat {
		log.Printf("format Dart code")
		if err := dartFormat(dartRoot); err != nil {
			return err
		}
	} else {
		log.Printf("dart format is disabled")
	}
	return nil
}

// findDartPackageDir walks up from start until a pubspec.yaml is found.
func findDartPackageDir(start string) (string, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = cwd
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "pubspec.yaml")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("fail to detect a Dart package (no pubspec.yaml) from %s; run inside a Flutter project", start)
		}
		dir = parent
	}
}

func readDartPackageName(dartRoot string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dartRoot, "pubspec.yaml"))
	if err != nil {
		return "", err
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(raw, &metadata); err != nil {
		return "", fmt.Errorf("parse %s: %w", filepath.Join(dartRoot, "pubspec.yaml"), err)
	}
	name := strings.TrimSpace(metadata.Name)
	if name == "" {
		return "", fmt.Errorf("pubspec.yaml in %s has no name field", dartRoot)
	}
	return name, nil
}

func normalizeGoModDir(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "go", nil
	}
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("go-mod-dir %q must be a relative directory inside the project", value)
	}
	return cleaned, nil
}

// ensureGokitTemplatePresent guards against builds made from a checkout whose
// gokit submodules were never initialized: embed silently drops empty
// directories, which would otherwise surface as a broken generated project.
func ensureGokitTemplatePresent(templates fs.FS, template Template) error {
	sentinel := "app/go_builder/gokit/run_build_tool.sh"
	if template == TemplatePlugin {
		sentinel = "plugin/gokit/run_build_tool.sh"
	}
	if _, err := fs.Stat(templates, sentinel); err != nil {
		return errors.New("the embedded gokit template is missing; rebuild flutter_go_bridge_codegen from a checkout with `git submodule update --init --recursive`")
	}
	return nil
}

func gokitDir(dartRoot string, template Template) string {
	if template == TemplatePlugin {
		return filepath.Join(dartRoot, "gokit")
	}
	return filepath.Join(dartRoot, "go_builder", "gokit")
}

// modifyPermissions marks the gokit helper scripts executable (non-Windows).
func modifyPermissions(dartRoot string, template Template) error {
	dir := gokitDir(dartRoot, template)
	for _, name := range []string{"build_pod.sh", "run_build_tool.sh", "run_build_tool.cmd"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := os.Chmod(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}
