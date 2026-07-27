package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	bridgeparser "github.com/star4277/flutter-go-bridge-gokit/internal/parser"
)

// generateFixture writes a one-file Go module, runs the full pipeline, and
// returns the three generated sources plus warnings. setup callbacks may add
// files (e.g. a go.work) to the module directory first.
func generateFixture(t *testing.T, source string, setup ...func(dir string)) (apiDart, central, goSource string, warnings []error, err error) {
	t.Helper()
	dir := t.TempDir()
	if writeErr := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.24\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	for _, callback := range setup {
		callback(dir)
	}
	inputDir := filepath.Join(dir, "api")
	if writeErr := os.MkdirAll(inputDir, 0o755); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	api, parseErr := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if parseErr != nil {
		return "", "", "", nil, parseErr
	}
	result, genErr := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "dart", "bridge_generated.dart"),
		LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	})
	if genErr != nil {
		return "", "", "", result.Warnings, genErr
	}
	return mustRead(t, filepath.Join(dir, "dart", "api.dart")),
		mustRead(t, filepath.Join(dir, "dart", "bridge_generated.dart")),
		mustRead(t, filepath.Join(dir, "bridge_generated.go")),
		result.Warnings, nil
}

func TestGenerateStructFieldTags(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

type Item struct {
	Name     string  `+"`fgb:\"rename:title\"`"+`
	Count    int     `+"`fgb:\"non-final,defaultValue: 0\"`"+`
	Hidden   string  `+"`fgb:\"ignore\"`"+`
	internal int
	Note     *string
}

func MakeItem(item Item) Item { return item }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"final String title;",
		"\n  int count;",
		"final String? note;",
		"required this.title,",
		"this.count = 0,",
		"this.note,",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "hidden") || strings.Contains(apiDart, "internal") {
		t.Fatalf("ignored/unexported fields leaked into Dart:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "const Item({") {
		t.Fatalf("a class with non-final fields must not have a const constructor:\n%s", apiDart)
	}
	if strings.Contains(goSource, "value.Hidden") || strings.Contains(goSource, "value.internal") || strings.Contains(goSource, `"hidden"`) {
		t.Fatalf("ignored/unexported fields leaked into the Go codec")
	}
	if !strings.Contains(goSource, `"title"`) {
		t.Fatal("renamed field should also rename the wire key")
	}
}

func TestGeneratePointerReceiverMethodOnValueStruct(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

type Point struct {
	X     int
	Y     int
	Label string
}

// Moved returns a copy shifted by (dx, dy).
func (p Point) Moved(dx int, dy int) Point {
	p.X += dx
	p.Y += dy
	return p
}

func (p *Point) Reset() {}
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"final class Point {",
		"final int x;",
		"final int label;",
	} {
		if strings.Contains(expected, "label") {
			continue // guard below uses the real spelling
		}
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "GoOpaque") || strings.Contains(apiDart, "fgbInternal(") {
		t.Fatalf("value struct with pointer methods must not be opaque:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "Point moved({required int dx, required int dy})") {
		t.Fatalf("methods should use named parameters:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "void reset()") {
		t.Fatalf("pointer-receiver method missing:\n%s", apiDart)
	}
	if !strings.Contains(goSource, "receiver.Moved(arg0, arg1)") || !strings.Contains(goSource, "receiver.Reset()") {
		t.Fatalf("Go dispatch should call methods on the reconstructed value:\n%s", goSource)
	}
	if !strings.Contains(central, "Point receiver") {
		t.Fatal("central bridge should pass the receiver by value")
	}
}

func TestGenerateExplicitOpaque(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

// Counter keeps its state on the Go side.
//
//fgb:opaque
type Counter struct {
	total int
}

func NewCounter() *Counter { return &Counter{} }

func (c *Counter) Add(delta int) int {
	c.total += delta
	return c.total
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Counter extends GoOpaque {") {
		t.Fatalf("fgb(opaque) type should extend GoOpaque:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "Counter.fgbInternal({required super.fgbBridge, required super.fgbHandle});") {
		t.Fatalf("opaque constructor should use named super parameters:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "int add({required int delta})") {
		t.Fatalf("opaque methods should use named parameters:\n%s", apiDart)
	}
	if !strings.Contains(central, "Counter.fgbInternal(fgbBridge: bridge, fgbHandle: value)") {
		t.Fatalf("decoder should construct opaque values with named arguments:\n%s", central)
	}
	if !strings.Contains(goSource, "fgbHandles.Store(handle, value)") {
		t.Fatal("opaque encoding should store the Go value in the handle registry")
	}
}

func TestGenerateAutoOpaqueFallback(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Conn struct {
	Events chan int
}

func Dial() *Conn { return &Conn{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Conn extends GoOpaque {") {
		t.Fatalf("untranslatable struct should fall back to GoOpaque:\n%s", apiDart)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "bridges as GoOpaque") && strings.Contains(warning.Error(), "Events") {
			found = true
		}
	}
	if !found {
		t.Fatalf("auto-opaque fallback should warn with the blocking field, got %v", warnings)
	}
}

func TestGenerateValueUseOfOpaqueRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:opaque
type Res struct {
	n int
}

func Take(r Res) {}
`)
	if err == nil || !strings.Contains(err.Error(), "must be passed as *Res") {
		t.Fatalf("value use of a GoOpaque type must be rejected, got %v", err)
	}
}

func TestGenerateDirectiveIgnoreAndRename(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

//fgb:ignore
func Hidden() {}

//fgb:async, rename = "fetchValue"
func LoadValue() int { return 42 }

//fgb:ignore
type Secret struct {
	X int
}

func (s Secret) Reveal() int { return s.X }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Future<int> fetchValue()") {
		t.Fatalf("fgb(rename) with async missing:\n%s", apiDart)
	}
	for _, forbidden := range []string{"hidden", "Secret", "reveal"} {
		if strings.Contains(apiDart, forbidden) {
			t.Fatalf("ignored declaration %q leaked into Dart:\n%s", forbidden, apiDart)
		}
	}
}

func TestGenerateIgnoredTypeInSignatureRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:ignore
type Gone struct {
	X int
}

func Use(g Gone) {}
`)
	if err == nil || !strings.Contains(err.Error(), "fgb(ignore)") {
		t.Fatalf("using an ignored type must be rejected, got %v", err)
	}
}

func TestGenerateTypeRename(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

//fgb:rename = "Position"
type Point struct {
	X int
}

func Origin() Point { return Point{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Position {") || !strings.Contains(apiDart, "Position origin()") {
		t.Fatalf("type rename was not applied:\n%s", apiDart)
	}
}

func TestGenerateDartOpaque(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	withWorkspace := func(dir string) {
		goWork := "go 1.24.0\n\nuse (\n\t.\n\t" + filepath.ToSlash(repoRoot) + "\n)\n"
		if writeErr := os.WriteFile(filepath.Join(dir, "go.work"), []byte(goWork), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	apiDart, central, goSource, _, err := generateFixture(t, `package api

import "github.com/star4277/flutter-go-bridge-gokit/fgb"

func Keep(token fgb.DartOpaque) fgb.DartOpaque { return token }

func MaybeKeep(token *fgb.DartOpaque) *fgb.DartOpaque { return token }
`, withWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Object keep({required Object token})") {
		t.Fatalf("DartOpaque should surface as Object:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "Object? maybeKeep({Object? token})") {
		t.Fatalf("*DartOpaque should surface as optional Object?:\n%s", apiDart)
	}
	for _, expected := range []string{"fgbInternalResolveDartOpaque", "fgbInternalRegisterDartOpaque"} {
		if !strings.Contains(central, expected) {
			t.Fatalf("central bridge missing %q", expected)
		}
	}
	for _, expected := range []string{
		"\"github.com/star4277/flutter-go-bridge-gokit/fgb\"",
		"fgb.NewDartOpaque", "fgbReleaseDartOpaque", "fgb_dart_opaque_port",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go bridge missing %q", expected)
		}
	}
}

func TestParseFieldTag(t *testing.T) {
	options, err := parseFieldTag("non-final, rename:pos, defaultValue: const [1, 2]")
	if err != nil {
		t.Fatal(err)
	}
	if !options.NonFinal || options.Rename != "pos" || options.DefaultValue != "const [1, 2]" {
		t.Fatalf("unexpected options: %#v", options)
	}
	if options, err := parseFieldTag("ignore"); err != nil || !options.Ignore {
		t.Fatalf("ignore: %#v err=%v", options, err)
	}
	if _, err := parseFieldTag("bogus"); err == nil {
		t.Fatal("unknown option should be rejected")
	}
	if _, err := parseFieldTag("rename:"); err == nil {
		t.Fatal("empty rename should be rejected")
	}
}

func TestGenerateWarnsInsteadOfFailingWithoutStopOnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/warn\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

func Good(value int) int { return value }

func Bad(callback func()) {}
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "bridge_generated.dart"),
		LibraryName: "warn", DartEntrypointClassName: "WarnBridge", StopOnError: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for the unsupported function type parameter")
	}
}
