package integrate

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	templatefs "github.com/star4277/flutter_go_bridge/template"
)

// CreateConfig mirrors flutter_rust_bridge's CreateConfig.
type CreateConfig struct {
	// Name of the new project; must be a valid Dart package name.
	Name string
	// Org is passed to `flutter create --org` when non-empty.
	Org string
	// WorkDir is the parent directory; the project is created at WorkDir/Name.
	// Empty means the current working directory.
	WorkDir string
	// Template selects between a Flutter app and an FFI plugin.
	Template Template
	// LibraryName and GoModDir have the same meaning as in Config.
	LibraryName string
	GoModDir    string
	// Platforms is a comma-separated platform list; empty auto-detects.
	Platforms string
}

// Create builds a new Flutter + Go project: `flutter create`, removal of the
// scaffold files our templates replace, then the regular integrate flow.
func Create(cfg CreateConfig) error {
	return create(cfg, templatefs.FS)
}

func create(cfg CreateConfig, templates fs.FS) error {
	if cfg.Template == "" {
		cfg.Template = TemplateApp
	}
	workDir := cfg.WorkDir
	if workDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		workDir = cwd
	}
	dartRoot := filepath.Join(workDir, cfg.Name)
	if _, err := os.Stat(dartRoot); err == nil {
		return fmt.Errorf("the target folder %s already exists; please use the `integrate` command in this case", dartRoot)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	platforms := resolveFlutterPlatforms(cfg.Template, cfg.Platforms)
	if err := flutterCreate(workDir, cfg.Name, cfg.Org, cfg.Template, platforms); err != nil {
		return err
	}

	if cfg.Template == TemplatePlugin {
		if err := removeUnnecessaryPluginFiles(dartRoot); err != nil {
			return err
		}
	} else {
		if err := removeUnnecessaryAppFiles(dartRoot); err != nil {
			return err
		}
	}

	log.Printf("inject flutter_go_bridge related code")
	return run(Config{
		WorkDir:               dartRoot,
		Template:              cfg.Template,
		LibraryName:           cfg.LibraryName,
		GoModDir:              cfg.GoModDir,
		Platforms:             platforms,
		EnableWriteLib:        true,
		EnableIntegrationTest: true,
		EnableDartFix:         true,
		EnableDartFormat:      true,
	}, templates)
}

// removeUnnecessaryAppFiles clears the `flutter create` demo sources so the
// template entrypoint is written fresh instead of commented in.
func removeUnnecessaryAppFiles(dartRoot string) error {
	if err := removeFilesInDir(filepath.Join(dartRoot, "lib")); err != nil {
		return err
	}
	return removeFilesInDir(filepath.Join(dartRoot, "test"))
}

// removeUnnecessaryPluginFiles clears the plugin_ffi scaffold pieces that the
// gokit templates replace: the C sources, ffigen config, and per-platform
// build files.
func removeUnnecessaryPluginFiles(dartRoot string) error {
	if err := removeFilesInDir(filepath.Join(dartRoot, "lib")); err != nil {
		return err
	}

	srcDir := filepath.Join(dartRoot, "src")
	if dirExists(srcDir) {
		if err := removeFilesInDir(srcDir); err != nil {
			return err
		}
		if err := os.Remove(srcDir); err != nil {
			return err
		}
	}
	ffigenFile := filepath.Join(dartRoot, "ffigen.yaml")
	if _, err := os.Stat(ffigenFile); err == nil {
		if err := os.Remove(ffigenFile); err != nil {
			return err
		}
	}

	if err := removeFilesInDir(filepath.Join(dartRoot, "example", "lib")); err != nil {
		return err
	}

	androidDir := filepath.Join(dartRoot, "android")
	if dirExists(androidDir) {
		androidSrcDir := filepath.Join(androidDir, "src")
		androidSrcMainDir := filepath.Join(androidSrcDir, "main")
		if err := removeFilesInDir(androidSrcMainDir); err != nil {
			return err
		}
		if err := os.Remove(androidSrcMainDir); err != nil {
			return err
		}
		if err := os.Remove(androidSrcDir); err != nil {
			return err
		}
		if err := removeFilesInDir(androidDir); err != nil {
			return err
		}
	}

	iosDir := filepath.Join(dartRoot, "ios")
	if dirExists(iosDir) {
		if err := removeClassesDir(iosDir); err != nil {
			return err
		}
	}

	linuxDir := filepath.Join(dartRoot, "linux")
	if dirExists(linuxDir) {
		if err := removeFilesInDir(linuxDir); err != nil {
			return err
		}
	}

	macosDir := filepath.Join(dartRoot, "macos")
	if dirExists(macosDir) {
		if err := removeClassesDir(macosDir); err != nil {
			return err
		}
	}

	windowsDir := filepath.Join(dartRoot, "windows")
	if dirExists(windowsDir) {
		if err := removeFilesInDir(windowsDir); err != nil {
			return err
		}
	}

	ohosDir := filepath.Join(dartRoot, "ohos")
	if dirExists(ohosDir) {
		if err := os.RemoveAll(ohosDir); err != nil {
			return err
		}
	}
	return nil
}

// removeClassesDir clears an ios/macos platform dir: its Classes/ subtree
// first, then the top-level files (podspec etc.).
func removeClassesDir(platformDir string) error {
	classesDir := filepath.Join(platformDir, "Classes")
	if err := removeFilesInDir(classesDir); err != nil {
		return err
	}
	if err := os.Remove(classesDir); err != nil {
		return err
	}
	return removeFilesInDir(platformDir)
}

// removeFilesInDir deletes every file in dir and refuses to touch nested
// directories, mirroring FRB's defensive remove_files_in_dir.
func removeFilesInDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			return fmt.Errorf("directory %s was expected to contain only files but directory %s was encountered", dir, path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
