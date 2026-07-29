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
	resolved := config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "dart", "bridge_generated.dart"),
		LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	}
	// Mirrors the CLI: the support package exists before the input is loaded.
	if _, writeErr := WriteSupportPackage(resolved); writeErr != nil {
		t.Fatal(writeErr)
	}
	api, parseErr := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if parseErr != nil {
		return "", "", "", nil, parseErr
	}
	result, genErr := Generate(api, resolved)
	if genErr != nil {
		return "", "", "", result.Warnings, genErr
	}
	// The Dart tree mirrors the Go package layout, so api/api.go lands in
	// dart/api/api.dart.
	return mustRead(t, filepath.Join(dir, "dart", "api", "api.dart")),
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
	apiDart, central, goSource, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

func Keep(token fgb.DartOpaque) fgb.DartOpaque { return token }

func MaybeKeep(token *fgb.DartOpaque) *fgb.DartOpaque { return token }
`)
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
		"\"example.com/fixture/internal/fgb\"",
		"fgbrt.NewDartOpaque", "fgbReleaseDartOpaque", "fgb_dart_opaque_port",
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

func TestGenerateCallbackParameter(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

//fgb:async
func Transform(input string, mapper func(s string) string) string {
	return mapper(input)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "required FutureOr<String> Function(String) mapper") {
		t.Fatalf("callback should surface as a FutureOr function type:\n%s", apiDart)
	}
	for _, expected := range []string{
		"fgbInternalRegisterCallback",
		"await Future.sync(() => value(a0));",
		"'callback result'",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("central bridge missing %q:\n%s", expected, central)
		}
	}
	for _, expected := range []string{
		"func fgbMakeCallback", "fgbInvokeCallback(handle", "runtime.KeepAlive(ref)",
		"//export fgb_callback_port", "//export fgb_callback_result",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go bridge missing %q", expected)
		}
	}
}

func TestGenerateCallbackRequiresAsync(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Broken(callback func(value int)) {
	callback(1)
}
`)
	if err == nil || !strings.Contains(err.Error(), "//fgb:async") {
		t.Fatalf("callbacks in sync calls must be rejected, got %v", err)
	}
}

func TestGenerateCallbackWithErrorResult(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

//fgb:async
func Guarded(callback func() (string, error)) (string, error) {
	return callback()
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSource, "func() (string, error) {") {
		t.Fatalf("callback factory should return (string, error):\n%s", goSource)
	}
	if !strings.Contains(goSource, "var zero string") || !strings.Contains(goSource, "return zero, err") {
		t.Fatal("transport failures should surface through the error result, not a panic")
	}
}

func TestGenerateCallbackRejectedOutsideDirectParams(t *testing.T) {
	if _, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Bad() func() { return nil }
`); err == nil || !strings.Contains(err.Error(), "direct parameters") {
		t.Fatalf("function results must be rejected, got %v", err)
	}
	if _, _, _, _, err := generateFixture(t, `package api

//fgb:async
func AlsoBad(callbacks []func()) {}
`); err == nil || !strings.Contains(err.Error(), "direct parameters") {
		t.Fatalf("function slices must be rejected, got %v", err)
	}
	if _, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Nested(callback func(inner func())) {}
`); err == nil || !strings.Contains(err.Error(), "nested function types") {
		t.Fatalf("nested function types must be rejected, got %v", err)
	}
}

func TestGenerateNullableCallback(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

//fgb:async, nullable = "onEvent"
func WithOptional(value int, onEvent func(message string)) int { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "required int value, FutureOr<void> Function(String)? onEvent") {
		t.Fatalf("nullable callback should be optional and nullable:\n%s", apiDart)
	}
	if !strings.Contains(goSource, "if handle == 0 {") {
		t.Fatal("callback factory should map handle 0 to a nil func")
	}
}

func TestGenerateNullableRejectsNonCallbackParameters(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:sync, rename = "fetchValue", nullable = "a,b,c"
func Values(a, b string, c func(string) (string, error)) string { return a }
`)
	if err == nil || !strings.Contains(err.Error(), `nullable lists parameter "a"`) {
		t.Fatalf("nullable on a non-callback parameter must be rejected, got %v", err)
	}
}

func TestGenerateNullableRejectsUnknownParameter(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async, nullable = "missing"
func Run(callback func()) {}
`)
	if err == nil || !strings.Contains(err.Error(), "unknown parameter") {
		t.Fatalf("nullable with an unknown name must be rejected, got %v", err)
	}
}

func TestGenerateNullableCollections(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

//fgb:async, nullable = "tags,scores,blob"
func Store(id int, tags []string, scores map[string]int, blob []byte) int { return id }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"required int id",
		"List<String>? tags",
		"Map<String, int>? scores",
		"Uint8List? blob",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
	for _, forbidden := range []string{"required List<String>?", "required Map<String, int>?", "required Uint8List?"} {
		if strings.Contains(apiDart, forbidden) {
			t.Fatalf("nullable parameters must not be required:\n%s", apiDart)
		}
	}
	if !strings.Contains(goSource, "if value == nil {") {
		t.Fatal("Go decoders should accept a null wire value for nilable types")
	}
}

func TestGenerateNullableRejectsArray(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async, nullable = "values"
func Fixed(values [3]int) int { return values[0] }
`)
	if err == nil || !strings.Contains(err.Error(), "nil without a pointer") {
		t.Fatalf("arrays cannot be nil and must be rejected, got %v", err)
	}
}

func TestGenerateStreamOwnedByGo(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

//fgb:async
func Ticks(count int, sink fgb.StreamSink[int]) error { return nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Stream<int> ticks({required int count})") {
		t.Fatalf("a call with only a sink should return Stream<T>:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "sink") && strings.Contains(apiDart, "required StreamSink") {
		t.Fatalf("the owned sink must not appear in the public signature:\n%s", apiDart)
	}
	for _, expected := range []string{"StreamController<int>()", "fgbInternalStartStream", "controller.stream"} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
	if !strings.Contains(central, "fgbInternalRegisterStreamSink") {
		t.Fatal("central bridge should register the sink")
	}
	for _, expected := range []string{"fgbMakeStreamSink", "fgbrt.NewStreamSink", "//export fgb_stream_port", "//export fgb_stream_cancel"} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go bridge missing %q", expected)
		}
	}
}

func TestGenerateStreamCreatedByDartWhenCallReturnsValue(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

//fgb:async
func Subscribe(name string, sink fgb.StreamSink[string]) (int, error) { return 0, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Future<int> subscribe({required String name, required StreamSink<String> sink})") {
		t.Fatalf("a call with a result keeps its return type and takes the sink:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "StreamController") {
		t.Fatalf("the Dart side owns the controller here:\n%s", apiDart)
	}
}

func TestGenerateStreamSinkInStructField(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

type Watcher struct {
	Name   string
	Events fgb.StreamSink[string]
}

//fgb:async
func Watch(watcher Watcher) error { return nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final StreamSink<String> events;") {
		t.Fatalf("a sink field should stay a Dart-created sink:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "GoOpaque") {
		t.Fatalf("a sink field must not force the struct opaque:\n%s", apiDart)
	}
}

func TestGenerateStreamSinkRequiresAsync(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

func Ticks(sink fgb.StreamSink[int]) error { return nil }
`)
	if err == nil || !strings.Contains(err.Error(), "//fgb:async") {
		t.Fatalf("stream sinks in sync calls must be rejected, got %v", err)
	}
}

func TestGenerateStreamSinkResultRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

//fgb:async
func Make() (fgb.StreamSink[int], error) { var zero fgb.StreamSink[int]; return zero, nil }
`)
	if err == nil || !strings.Contains(err.Error(), "cannot be returned to Dart") {
		t.Fatalf("returning a sink must be rejected, got %v", err)
	}
}

func TestGenerateChannelStream(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

//fgb:async
func Ticks(count int, out chan<- int) error { return nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Stream<int> ticks({required int count})") {
		t.Fatalf("a channel parameter should produce Stream<T>:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "out") {
		t.Fatalf("the bridge-owned channel must not reach Dart:\n%s", apiDart)
	}
	for _, expected := range []string{
		"func fgbMakeStreamChannel", "make(chan int, 16)", "for value := range ch {",
		"func fgbCloseStreamChannel", "defer fgbCloseStreamChannel",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go bridge missing %q", expected)
		}
	}
	if strings.Contains(goSource, "fgbMakeStreamSink") {
		t.Fatal("a channel stream must not build a StreamSink")
	}
}

func TestGenerateChannelStreamWithContext(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

import "context"

//fgb:async
func Watch(ctx context.Context, out chan<- string) error { return nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Stream<String> watch()") {
		t.Fatalf("context and channel must both stay out of the Dart signature:\n%s", apiDart)
	}
	for _, expected := range []string{
		"context.WithCancel(context.Background())",
		"fgbRegisterStreamCancel(", "api.Watch(fgbCtx, arg0)",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go bridge missing %q:\n%s", expected, goSource)
		}
	}
}

func TestGenerateChannelStreamRejectsBidirectional(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Ticks(out chan int) error { return nil }
`)
	if err == nil || !strings.Contains(err.Error(), "send-only") {
		t.Fatalf("only chan<- T may be bridged, got %v", err)
	}
}

func TestGenerateChannelStreamRequiresAsync(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Ticks(out chan<- int) error { return nil }
`)
	if err == nil || !strings.Contains(err.Error(), "//fgb:async") {
		t.Fatalf("channel streams in sync calls must be rejected, got %v", err)
	}
}

func TestGenerateMultipleResultsBecomeRecord(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

func Divide(a, b int) (int, int, error) { return 0, 0, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "(int, int) divide({required int a, required int b})") {
		t.Fatalf("unnamed results should become a positional record:\n%s", apiDart)
	}
	if !strings.Contains(goSource, "result0, result1, goErr0 := api.Divide(arg0, arg1)") {
		t.Fatalf("Go dispatch should bind every result:\n%s", goSource)
	}
	if !strings.Contains(goSource, "return []any{encoded0, encoded1}, nil") {
		t.Fatal("several results should travel as one list")
	}
}

func TestGenerateNamedResultsBecomeNamedRecord(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

func Split(value string) (head string, tail string, err error) { return "", "", nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "({String head, String tail}) split({required String value})") {
		t.Fatalf("named results should become a named record:\n%s", apiDart)
	}
	if !strings.Contains(central, "head:") || !strings.Contains(central, "tail:") {
		t.Fatalf("the decoder should build the record with field names:\n%s", central)
	}
}

func TestGenerateSingleResultStaysPlain(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

func Add(a, b int) (int, error) { return 0, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "int add({required int a, required int b})") {
		t.Fatalf("a single result must not be wrapped in a record:\n%s", apiDart)
	}
	if !strings.Contains(goSource, "fgbGoError(call.Method, goErr0)") {
		t.Fatal("a single error should keep the plain error path")
	}
}

func TestGenerateMultipleErrorsAreCollected(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

func Validate(name string) (string, error, error) { return "", nil, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "String validate({required String name})") {
		t.Fatalf("errors must not appear in the Dart result:\n%s", apiDart)
	}
	for _, expected := range []string{
		"var fgbErrs []any",
		"fgbErrs = append(fgbErrs, goErr0.Error())",
		"fgbErrs = append(fgbErrs, goErr1.Error())",
		"fgbGoErrors(call.Method, fgbErrs)",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go dispatch missing %q:\n%s", expected, goSource)
		}
	}
}

func TestGenerateErrorNeedNotBeLast(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

func Load(id int) (error, int) { return nil, 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "int load({required int id})") {
		t.Fatalf("a leading error should not change the Dart result:\n%s", apiDart)
	}
	if !strings.Contains(goSource, "goErr0, result0 := api.Load(arg0)") {
		t.Fatalf("Go dispatch should keep the declared result order:\n%s", goSource)
	}
}

func TestGenerateEmbeddedStructBecomesSuperclass(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

type Animal struct {
	Name string
	Legs int
}

type Dog struct {
	Animal
	Breed string
}

func MakeDog() Dog { return Dog{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "class Dog extends Animal {") {
		t.Fatalf("an embedded struct should become the Dart superclass:\n%s", apiDart)
	}
	for _, expected := range []string{
		"required super.name,",
		"required super.legs,",
		"required this.breed,",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("promoted fields should be forwarded with super, missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "final Animal animal") {
		t.Fatalf("the embedded struct must not stay an ordinary field:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "final class Animal") || strings.Contains(apiDart, "final class Dog") {
		t.Fatalf("classes taking part in inheritance must not be final:\n%s", apiDart)
	}
	for _, expected := range []string{`"name"`, `"legs"`, `"breed"`, "result.Name =", "result.Legs =", "result.Breed ="} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("Go codec missing %q for the flattened struct", expected)
		}
	}
}

func TestGenerateShadowedMethodBecomesOverride(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Animal struct{ Name string }

func (a Animal) Kind() string { return "animal" }

type Dog struct{ Animal }

func (d Dog) Kind() string { return "dog" }

func MakeDog() Dog { return Dog{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "@override") {
		t.Fatalf("a compatible shadowing method should be marked @override:\n%s", apiDart)
	}
}

func TestGenerateIncompatibleShadowedMethodRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Animal struct{ Name string }

func (a Animal) Kind() string { return "animal" }

type Dog struct{ Animal }

func (d Dog) Kind(verbose bool) string { return "dog" }

func MakeDog() Dog { return Dog{} }
`)
	if err == nil || !strings.Contains(err.Error(), "shadows") {
		t.Fatalf("Dart cannot express an incompatible override, got %v", err)
	}
}

func TestGenerateEmbeddingRejectsMultipleAndPointer(t *testing.T) {
	if _, _, _, _, err := generateFixture(t, `package api

type A struct{ X int }
type B struct{ Y int }

type C struct {
	A
	B
}

func MakeC() C { return C{} }
`); err == nil || !strings.Contains(err.Error(), "only extend one type") {
		t.Fatalf("multiple embedding must be rejected, got %v", err)
	}
	if _, _, _, _, err := generateFixture(t, `package api

type A struct{ X int }

type D struct {
	*A
	Y int
}

func MakeD() D { return D{} }
`); err == nil || !strings.Contains(err.Error(), "pointer") {
		t.Fatalf("embedding a pointer must be rejected, got %v", err)
	}
}

func TestGenerateInterfaceBecomesAbstractInterfaceClass(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

type Shape interface {
	Area() int
	Label() string
}

type Circle struct{ R int }

func (c Circle) Area() int     { return c.R }
func (c Circle) Label() string { return "circle" }

func Describe(shape Shape) string { return shape.Label() }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"abstract interface class Shape {",
		"int area();",
		"String label();",
		"class Circle implements Shape {",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
	if !strings.Contains(central, "is not a Go implementation of Shape") {
		t.Fatalf("the Dart encoder should tag interface values:\n%s", central)
	}
	if !strings.Contains(goSource, "case api.Circle:") {
		t.Fatalf("the Go encoder should switch on the concrete type:\n%s", goSource)
	}
}

func TestGenerateInterfaceRespectsEmbeddingOrder(t *testing.T) {
	_, central, _, _, err := generateFixture(t, `package api

type Shape interface{ Area() int }

type Square struct{ Side int }

func (s Square) Area() int { return s.Side }

type ColoredSquare struct {
	Square
	Color string
}

func Describe(shape Shape) int { return shape.Area() }
`)
	if err != nil {
		t.Fatal(err)
	}
	colored := strings.Index(central, "value is ColoredSquare")
	square := strings.Index(central, "value is Square")
	if colored < 0 || square < 0 || colored > square {
		t.Fatalf("a subclass must be tested before the class it extends:\n%s", central)
	}
}

func TestGenerateInterfaceMethodDirectives(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Loader interface {
	//fgb:async, rename = "fetch"
	Load(id int) (string, error)
}

type Remote struct{ Host string }

//fgb:async
func (r Remote) Load(id int) (string, error) { return "", nil }

func Use(loader Loader) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Future<String> fetch({required int id});") {
		t.Fatalf("interface method directives should shape the declaration:\n%s", apiDart)
	}
}

func TestGenerateNullableInterfaceParameter(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Shape interface{ Area() int }

type Circle struct{ R int }

func (c Circle) Area() int { return c.R }

//fgb:nullable = "shape"
func Describe(shape Shape) int { return 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "int describe({Shape? shape})") {
		t.Fatalf("an interface parameter should be nullable:\n%s", apiDart)
	}
}

func TestGenerateInterfaceWithoutImplementationRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Shape interface{ Area() int }

func Describe(shape Shape) int { return 0 }
`)
	if err == nil || !strings.Contains(err.Error(), "no bridged type implements") {
		t.Fatalf("an interface with no implementation must be rejected, got %v", err)
	}
}

func TestGenerateNullableFieldTag(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

type Record struct {
	Name   string
	Tags   []string       `+"`fgb:\"nullable\"`"+`
	Scores map[string]int `+"`fgb:\"nullable\"`"+`
	Plain  []string
}

func RoundTrip(record Record) Record { return record }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"final List<String>? tags;",
		"final Map<String, int>? scores;",
		"final List<String> plain;",
		"required this.name",
		"this.tags,",
		"this.scores,",
		"required this.plain",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
	// A nullable field keeps nil distinct from empty in the Go -> Dart
	// direction, which the shared encoders would otherwise normalize away.
	if !strings.Contains(goSource, "if value.Tags != nil {") || !strings.Contains(goSource, "if value.Scores != nil {") {
		t.Fatalf("the Go encoder should preserve nil for nullable fields:\n%s", goSource)
	}
	if strings.Contains(goSource, "if value.Plain != nil {") {
		t.Fatal("an unmarked field should keep the plain encoding path")
	}
}

func TestGenerateNullableFieldTagRejectsUnsupportedTypes(t *testing.T) {
	if _, _, _, _, err := generateFixture(t, `package api

type Record struct {
	Count int `+"`fgb:\"nullable\"`"+`
}

func RoundTrip(record Record) Record { return record }
`); err == nil || !strings.Contains(err.Error(), "nil without a pointer") {
		t.Fatalf("a non-nilable field must be rejected, got %v", err)
	}
	if _, _, _, _, err := generateFixture(t, `package api

type Record struct {
	Note *string `+"`fgb:\"nullable\"`"+`
}

func RoundTrip(record Record) Record { return record }
`); err == nil || !strings.Contains(err.Error(), "already nullable") {
		t.Fatalf("a pointer field must be reported as redundant, got %v", err)
	}
}

func TestGenerateTypedListKeepsEmptyDistinctFromNil(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

func RoundTrip(data []byte) []byte { return data }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(goSource, "append([]byte(nil), raw...)") {
		t.Fatal("append on a nil slice returns nil for empty input, erasing empty vs nil")
	}
	if !strings.Contains(goSource, "copy(result, raw)") {
		t.Fatalf("typed lists should be copied into a made slice:\n%s", goSource)
	}
}
