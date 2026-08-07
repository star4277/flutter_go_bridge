package integrate

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// computeReplacements mirrors FRB's compute_replacements: placeholder values
// plus filename renames for files that must not look like real Go sources
// inside this repository (embed skips nested modules).
func computeReplacements(dartPackageName, libraryName, goModDir string, includeOhos bool) map[string]string {
	ohosText := ""
	if includeOhos {
		ohosText = "\n      ohos:\n        ffiPlugin: true"
	}
	return map[string]string{
		"REPLACE_ME_DART_PACKAGE_NAME":         dartPackageName,
		"REPLACE_ME_GO_MOD_NAME":               libraryName,
		"REPLACE_ME_GO_MOD_DIR":                goModDir,
		"REPLACE_ME_OHOS_PLUGIN_PLATFORM_TEXT": ohosText,
		"go.mod.template":                      "go.mod",
		"bridge_generated.go.template":         "bridge_generated.go",
		"lib.go.template":                      "lib.go",
	}
}

// replaceStringContent applies every replacement. Keys are ordered longest
// first so the result does not depend on map iteration order.
func replaceStringContent(content string, replacements map[string]string) string {
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		content = strings.ReplaceAll(content, key, replacements[key])
	}
	return content
}

// executeOverlayTemplates copies the template trees onto dartRoot in the same
// order FRB uses: common files, then the app/plugin shared files (which may
// comment out an existing entrypoint), then the backend-specific tree.
func executeOverlayTemplates(templates fs.FS, replacements map[string]string, dartRoot string, cfg *Config, includeOhos bool, dartPackageName string) error {
	if err := overlayDir(templates, "shared/common", replacements, dartRoot, cfg, nil, includeOhos); err != nil {
		return err
	}

	sharedTemplateDir := "shared/app"
	commentOutFiles := []string{"main.dart"}
	if cfg.Template == TemplatePlugin {
		sharedTemplateDir = "shared/plugin"
		commentOutFiles = []string{dartPackageName + ".dart"}
	}
	if err := overlayDir(templates, sharedTemplateDir, replacements, dartRoot, cfg, commentOutFiles, includeOhos); err != nil {
		return err
	}

	backendTemplateDir := "app"
	if cfg.Template == TemplatePlugin {
		backendTemplateDir = "plugin"
	}
	return overlayDir(templates, backendTemplateDir, replacements, dartRoot, cfg, nil, includeOhos)
}

// overlayDir walks one template root and copies its entries into dartRoot,
// substituting placeholders in both paths and file contents. Existing files
// are never overwritten: entrypoints listed in commentOutFiles keep their old
// content commented out above the template, everything else is skipped with a
// warning.
func overlayDir(templates fs.FS, root string, replacements map[string]string, dartRoot string, cfg *Config, commentOutFiles []string, includeOhos bool) error {
	sub, err := fs.Sub(templates, root)
	if err != nil {
		return err
	}
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := fs.ReadDir(sub, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			rel := entry.Name()
			if dir != "." {
				rel = path.Join(dir, entry.Name())
			}
			if !filterFile(rel, cfg.EnableWriteLib, cfg.EnableIntegrationTest, includeOhos) {
				continue
			}
			target := filepath.Join(dartRoot, filepath.FromSlash(replaceStringContent(rel, replacements)))
			if entry.IsDir() {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
				if err := walk(rel); err != nil {
					return err
				}
				continue
			}
			reference, err := fs.ReadFile(sub, rel)
			if err != nil {
				return err
			}
			existing, readErr := os.ReadFile(target)
			if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
				return fmt.Errorf("read existing %s: %w", target, readErr)
			}
			data, write := modifyFile(rel, target, reference, existing, readErr == nil, replacements, commentOutFiles)
			if !write {
				continue
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", target, err)
			}
		}
		return nil
	}
	return walk(".")
}

func modifyFile(rel, target string, reference, existing []byte, exists bool, replacements map[string]string, commentOutFiles []string) ([]byte, bool) {
	src := []byte(replaceStringContent(string(reference), replacements))

	if exists {
		if slices.Contains(commentOutFiles, filepath.Base(target)) {
			return commentOutExistingAndAppendTemplate(existing, src), true
		}
		// Upgrade the generated bridge when an earlier integration still has
		// the synchronous initializer signature. The bridge is generated output,
		// so this narrow signature check does not overwrite user-authored files.
		if strings.Contains(string(existing), "static void initialize({String? libraryPath})") &&
			strings.Contains(string(src), "static Future<void> initialize") &&
			strings.HasSuffix(filepath.ToSlash(rel), "lib/src/bridge_generated.dart") {
			return src, true
		}
		log.Printf("warning: skip writing to %s because the file already exists; remove it and rerun to apply the full template", target)
		return nil, false
	}

	if hasPathComponent(rel, "gokit") {
		if comments, ok := computeGokitComments(rel); ok {
			return append([]byte(comments), src...), true
		}
	}
	return src, true
}

func commentOutExistingAndAppendTemplate(existing, src []byte) []byte {
	lines := strings.Split(string(existing), "\n")
	for i, line := range lines {
		lines[i] = "// " + line
	}
	commented := "// The original content is temporarily commented out to allow generating a self-contained demo - feel free to uncomment later.\n\n" +
		strings.Join(lines, "\n") + "\n\n"
	return append([]byte(commented), src...)
}

// filterFile ports FRB's filter_file: prune ohos when unsupported, gokit
// metadata always, and lib/platform/Go-module files when --no-write-lib.
func filterFile(rel string, enableWriteLib, enableIntegrationTest, includeOhos bool) bool {
	if !includeOhos && hasPathComponent(rel, "ohos") {
		return false
	}

	if hasPathComponent(rel, "gokit") {
		switch path.Base(rel) {
		case ".git", ".github", "docs", "test":
			return false
		}
		return true
	}

	if !enableWriteLib {
		if hasPathComponent(rel, "go_builder") {
			return true
		}
		for _, component := range []string{
			"android", "ios", "windows", "macos", "linux", "ohos",
			"lib", "REPLACE_ME_GO_MOD_DIR", "flutter_go_bridge.yaml",
		} {
			if hasPathComponent(rel, component) {
				return false
			}
		}
	}

	if !enableIntegrationTest &&
		(hasPathComponent(rel, "integration_test") || hasPathComponent(rel, "test_driver")) {
		return false
	}
	return true
}

func hasPathComponent(rel, name string) bool {
	return slices.Contains(strings.Split(rel, "/"), name)
}

var gokitPrelude = []string{
	"This is copied from Gokit (which is the official way to use it currently)",
	"Details: https://github.com/star4277/gokit",
}

// computeGokitComments prepends a provenance note to copied gokit files where
// a comment syntax is known and safe (e.g. not shell scripts with shebangs).
func computeGokitComments(rel string) (string, bool) {
	name := path.Base(rel)
	if name == ".gitignore" {
		return "", false
	}
	leading := ""
	switch strings.TrimPrefix(path.Ext(name), ".") {
	case "dart", "md", "gradle", "":
		leading = "///"
	case "yaml", "toml":
		leading = "#"
	default:
		// sh/cmd/ps1/cmake/lock and anything unexpected stay untouched.
		return "", false
	}
	var builder strings.Builder
	for _, line := range gokitPrelude {
		builder.WriteString(leading + " " + line + "\n")
	}
	builder.WriteString("\n")
	return builder.String(), true
}
