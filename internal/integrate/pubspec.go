package integrate

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// pubAddDependencies ports FRB's pub_add_dependencies: apps depend on the
// go_builder helper package, and integration_test is added where enabled.
// There is no runtime Dart package to add - the generated bridge only uses
// the Dart SDK.
func pubAddDependencies(dartRoot, libraryName string, cfg *Config) error {
	if cfg.Template == TemplateApp {
		if err := flutterPubAdd(dartRoot, libraryName, "--path=go_builder"); err != nil {
			return err
		}
	}

	if cfg.Template == TemplatePlugin {
		exampleDir := filepath.Join(dartRoot, "example")
		if _, err := os.Stat(filepath.Join(exampleDir, "pubspec.yaml")); err == nil {
			if err := flutterPubAdd(exampleDir, "integration_test", "--dev", "--sdk=flutter"); err != nil {
				return err
			}
		} else {
			log.Printf("skip integration_test in example: %s has no pubspec.yaml", exampleDir)
		}
	}

	if cfg.EnableIntegrationTest {
		if err := flutterPubAdd(dartRoot, "integration_test", "--dev", "--sdk=flutter"); err != nil {
			return err
		}
	}
	return nil
}

// excludeGokitFromOuterAnalyzer keeps the copied gokit build_tool sources out
// of the host project's Dart analyzer.
func excludeGokitFromOuterAnalyzer(dartRoot string, template Template) error {
	path := filepath.Join(dartRoot, "analysis_options.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	exclude := "go_builder/gokit/**"
	if template == TemplatePlugin {
		exclude = "gokit/**"
	}
	return os.WriteFile(path, []byte(addAnalyzerExclude(string(raw), exclude)), 0o644)
}

// addAnalyzerExclude inserts `exclude` into the analyzer block with targeted
// text edits so YAML comments, blank lines, and formatting stay unchanged.
func addAnalyzerExclude(text, exclude string) string {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "- "+exclude {
			return text
		}
	}
	excludeLine := "    - " + exclude

	hadTrailingNewline := strings.HasSuffix(text, "\n")
	body := strings.TrimSuffix(text, "\n")
	var lines []string
	if body != "" {
		lines = strings.Split(body, "\n")
	}

	analyzerIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "analyzer:" {
			analyzerIndex = i
			break
		}
	}
	if analyzerIndex < 0 {
		return "analyzer:\n  exclude:\n" + excludeLine + "\n\n" + text
	}

	blockEnd := len(lines)
	for i := analyzerIndex + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" && !strings.HasPrefix(lines[i], " ") && !strings.HasPrefix(lines[i], "#") {
			blockEnd = i
			break
		}
	}

	excludeIndex := -1
	for i := analyzerIndex + 1; i < blockEnd; i++ {
		if strings.TrimSpace(lines[i]) == "exclude:" {
			excludeIndex = i
			break
		}
	}
	if excludeIndex >= 0 {
		insertIndex := blockEnd
		for i := excludeIndex + 1; i < blockEnd; i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed != "" && !strings.HasPrefix(trimmed, "- ") {
				insertIndex = i
				break
			}
		}
		lines = insertLines(lines, insertIndex, excludeLine)
	} else {
		lines = insertLines(lines, analyzerIndex+1, "  exclude:", excludeLine)
	}

	output := strings.Join(lines, "\n")
	if hadTrailingNewline {
		output += "\n"
	}
	return output
}

func insertLines(lines []string, index int, inserted ...string) []string {
	result := make([]string, 0, len(lines)+len(inserted))
	result = append(result, lines[:index]...)
	result = append(result, inserted...)
	result = append(result, lines[index:]...)
	return result
}
