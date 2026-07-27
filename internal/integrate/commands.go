package integrate

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runCommand executes an external tool, streaming its output to the console.
// It is a variable so tests can stub out flutter/dart invocations.
var runCommand = func(dir string, name string, args ...string) error {
	log.Printf("execute `%s %s` in %s (this may take a while)", name, strings.Join(args, " "), dir)
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true", "CI=true")
	if err := command.Run(); err != nil {
		return fmt.Errorf("`%s %s` failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// captureCommand runs a tool and returns its stdout; also stubbed in tests.
var captureCommand = func(dir string, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true", "CI=true")
	out, err := command.Output()
	return string(out), err
}

func flutterPubAdd(dir string, items ...string) error {
	return runCommand(dir, "flutter", append([]string{"pub", "add"}, items...)...)
}

func flutterPubGet(dir string) error {
	return runCommand(dir, "flutter", "pub", "get")
}

func dartFix(dir string) error {
	return runCommand(dir, FindDartExecutable(), "fix", "--apply", ".")
}

func dartFormat(dir string) error {
	return runCommand(dir, FindDartExecutable(), "format", "--line-length", "80", ".")
}

// FindDartExecutable locates dart, preferring the SDK binary over Flutter's
// Windows `dart.bat` wrapper: the wrapper performs SDK/bootstrap checks that
// are unnecessary here and can hang in non-interactive runs.
func FindDartExecutable() string {
	dart, err := exec.LookPath("dart")
	if err != nil {
		return "dart"
	}
	if ext := strings.ToLower(filepath.Ext(dart)); runtime.GOOS == "windows" && (ext == ".bat" || ext == ".cmd") {
		candidate := filepath.Join(filepath.Dir(dart), "cache", "dart-sdk", "bin", "dart.exe")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	return dart
}

// resolveFlutterPlatforms returns the explicit platform list or the default
// one, probing `flutter create --help` for OpenHarmony-enabled Flutter forks.
func resolveFlutterPlatforms(template Template, platforms string) string {
	if strings.TrimSpace(platforms) != "" {
		return platforms
	}
	return defaultFlutterPlatforms(template, isOhosFlutter())
}

func isOhosFlutter() bool {
	out, err := captureCommand("", "flutter", "create", "--help")
	if err != nil {
		return false
	}
	supported := flutterCreateHelpSupportsPlatform(out, "ohos")
	if supported {
		log.Printf("current flutter supports the ohos platform")
	}
	return supported
}

func defaultFlutterPlatforms(template Template, includeOhos bool) string {
	platforms := "android,ios,linux,macos,windows"
	if includeOhos {
		platforms += ",ohos"
	}
	if template != TemplatePlugin {
		platforms += ",web"
	}
	return platforms
}

func platformListContainsOhos(platforms string) bool {
	for platform := range strings.SplitSeq(platforms, ",") {
		if strings.EqualFold(strings.TrimSpace(platform), "ohos") {
			return true
		}
	}
	return false
}

// flutterCreateHelpSupportsPlatform checks whether the `--platforms` option in
// the given `flutter create --help` output lists the platform.
func flutterCreateHelpSupportsPlatform(help, platform string) bool {
	inPlatformsOption := false
	for line := range strings.SplitSeq(help, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		startsNewOption := strings.HasPrefix(trimmed, "--") ||
			(strings.HasPrefix(trimmed, "-") && strings.Contains(trimmed, "--"))
		if inPlatformsOption && startsNewOption && !strings.Contains(trimmed, "--platforms") {
			return false
		}
		if strings.Contains(trimmed, "--platforms") {
			inPlatformsOption = true
		}
		if inPlatformsOption && textContainsPlatformToken(line, platform) {
			return true
		}
	}
	return false
}

func textContainsPlatformToken(text, platform string) bool {
	isTokenRune := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_'
	}
	for _, token := range strings.FieldsFunc(text, func(r rune) bool { return !isTokenRune(r) }) {
		if strings.EqualFold(token, platform) {
			return true
		}
	}
	return false
}
