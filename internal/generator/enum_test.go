package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
)

func TestBuildExplicitIntegerEnum(t *testing.T) {
	unit, warnings, err := buildEnumFixture(t, `package api

//fgb:enum
type Status int

const (
	StatusUnknown Status = 0
	StatusReady Status = 1
)

func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(unit.Named) != 1 || !unit.Named[0].Enum {
		t.Fatalf("explicit enum was not represented in the generator IR: %#v", unit.Named)
	}
	for index, expected := range []struct{ name, literal string }{{"unknown", "0"}, {"ready", "1"}} {
		if unit.Named[0].Constants[index].DartName != expected.name || unit.Named[0].Constants[index].DartLiteral != expected.literal {
			t.Fatalf("generated enum case %d = %#v, want %#v", index, unit.Named[0].Constants[index], expected)
		}
	}
}

func TestBuildStringEnumPrefixAndRename(t *testing.T) {
	unit, _, err := buildEnumFixture(t, `package api

//fgb:enum
type Kind string

const (
	KindFoo Kind = "foo"
	//fgb:rename = "special"
	KindBar Kind = "bar"
)

func EchoKind(value Kind) Kind { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := unit.Named[0].Constants; len(got) != 2 || got[0].DartName != "foo" || got[0].DartLiteral != `"foo"` || got[1].DartName != "special" {
		t.Fatalf("unexpected string enum cases: %#v", got)
	}
}

func TestBuildEnumWithoutTypePrefixKeepsFullCaseName(t *testing.T) {
	unit, _, err := buildEnumFixture(t, `package api

//fgb:enum
type Status int

const ReadyState Status = 2

func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := unit.Named[0].Constants[0].DartName; got != "readyState" {
		t.Fatalf("unprefixed Go enum constant got Dart name %q, want readyState", got)
	}
}

func TestBuildEnumExplicitRenameWinsOverPrefixStripping(t *testing.T) {
	unit, _, err := buildEnumFixture(t, `package api

//fgb:enum
type Status int

const (
	//fgb:rename = "statusReady"
	StatusReady Status = 1
)

func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := unit.Named[0].Constants[0].DartName; got != "statusReady" {
		t.Fatalf("explicit rename was stripped as a type prefix: got %q", got)
	}
}

func TestBuildEnumRejectsInvalidClosedSet(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "unsupported underlying",
			source: `package api

//fgb:enum
type Ratio float64

const RatioOne Ratio = 1

func EchoRatio(value Ratio) Ratio { return value }
`,
			want: "enum Ratio requires a string or integer underlying type",
		},
		{
			name: "no constants",
			source: `package api

//fgb:enum
type Empty int

func EchoEmpty(value Empty) Empty { return value }
`,
			want: "enum Empty must declare at least one exported typed constant",
		},
		{
			name: "duplicate values",
			source: `package api

//fgb:enum
type State int

const (
	StateA State = 1
	StateB State = 1
)

func EchoState(value State) State { return value }
`,
			want: "enum State has duplicate underlying value",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := buildEnumFixture(t, test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestUnmarkedLikelyEnumStaysExtensionTypeWithWarning(t *testing.T) {
	unit, warnings, err := buildEnumFixture(t, `package api

type Status int

const (
	StatusUnknown Status = 0
	StatusReady Status = 1
)

func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit.Named) != 1 || unit.Named[0].Enum {
		t.Fatalf("unmarked named type changed representation: %#v", unit.Named)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "looks like an enum") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected heuristic enum warning, got %v", warnings)
	}
}

func TestBuildEnumDisambiguatesReservedCaseNames(t *testing.T) {
	unit, warnings, err := buildEnumFixture(t, `package api

//fgb:enum
type Flag int

const (
	FlagValues Flag = 0
	FlagValue Flag = 1
)

func EchoFlag(value Flag) Flag { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := unit.Named[0].Constants; len(got) != 2 || got[0].DartName != "values2" || got[1].DartName != "value2" {
		t.Fatalf("reserved enum case names were not disambiguated: %#v", got)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected reserved-name warnings, got %v", warnings)
	}
}

func TestBuildEnumDisambiguatesDuplicateRenamedCases(t *testing.T) {
	unit, warnings, err := buildEnumFixture(t, `package api

//fgb:enum
type Flag int

const (
	//fgb:rename = "same"
	FlagFirst Flag = 1
	//fgb:rename = "same"
	FlagSecond Flag = 2
)

func EchoFlag(value Flag) Flag { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := unit.Named[0].Constants; len(got) != 2 || got[0].DartName != "same" || got[1].DartName != "same2" {
		t.Fatalf("duplicate renamed cases were not disambiguated: %#v", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "duplicate Dart name") {
		t.Fatalf("expected one duplicate-case warning, got %v", warnings)
	}
}

func TestRenderEnumDeclarationAndWireCodecs(t *testing.T) {
	apiDart, central, goSource, warnings, err := generateFixture(t, `package api

//fgb:enum
type Status int

const (
	StatusUnknown Status = 0
	StatusReady Status = 1
)

func EchoStatus(value Status) Status { return value }
func MaybeStatus(value *Status) *Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	for _, expected := range []string{
		"enum Status {",
		"unknown(0),",
		"ready(1);",
		"const Status(this.value);",
		"final int value;",
		"Status? maybeStatus({Status? value})",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart enum missing %q:\n%s", expected, apiDart)
		}
	}
	for _, expected := range []string{
		"for (final item in Status.values)",
		"unknown Status value",
		"return fgbEncode",
		"value.value",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("generated Dart codec missing %q:\n%s", expected, central)
		}
	}
	if !strings.Contains(goSource, "fgbDcoEncode") || !strings.Contains(goSource, "int(value)") {
		t.Fatalf("Go named enum codec no longer transports the underlying value:\n%s", goSource)
	}
}

func TestRenderStringEnumStoresExactWireValue(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

//fgb:enum
type Mode string

const (
	ModeFast Mode = "fast-mode"
	ModeSafe Mode = "safe mode"
)

func EchoMode(value Mode) Mode { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`fast("fast-mode")`, `safe("safe mode")`, "final String value;"} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("string enum lost exact wire value %q:\n%s", expected, apiDart)
		}
	}
	if !strings.Contains(central, "unknown Mode value") {
		t.Fatalf("string enum decoder does not reject unknown values:\n%s", central)
	}
}

func TestRenderUint64EnumUsesConstWireLiteral(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

//fgb:enum
type Code uint64

const CodeMax Code = 18446744073709551615

func EchoCode(value Code) Code { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`max("18446744073709551615");`,
		"const Code(this._wireValue);",
		"final String _wireValue;",
		"BigInt get value => BigInt.parse(_wireValue);",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("uint64 enum missing const-safe BigInt representation %q:\n%s", expected, apiDart)
		}
	}
	if !strings.Contains(central, "value.value") || !strings.Contains(central, "unknown Code value") {
		t.Fatalf("uint64 enum codec does not preserve its BigInt value:\n%s", central)
	}
}

func TestRenderEnumMethodsOperatorsAndGoToString(t *testing.T) {
	apiDart, central, _, warnings, err := generateFixture(t, `package api

//fgb:enum
type Status int

const (
	StatusUnknown Status = 0
	StatusReady Status = 1
)

func (s Status) IsReady() bool { return s == StatusReady }
func (s Status) Add(other Status) Status { return s + other }
func (s Status) ToString() string { return "status" }
func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	for _, expected := range []string{
		"bool isReady()",
		"Status operator +(Status other)",
		"@override\n  String toString()",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("enum method behavior missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "String toString() => value.toString();") || !strings.Contains(central, "fgbInternalCall") {
		t.Fatalf("enum toString must use the Go method instead of a local fallback:\n%s", apiDart)
	}
}

func TestRenderEnumWithoutGoToStringKeepsDartDefault(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

//fgb:enum
type Status int

const StatusReady Status = 1

func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "String toString()") {
		t.Fatalf("enum without a Go-backed representation must keep Dart's normal toString:\n%s", apiDart)
	}
}

func TestRenderEnumDisambiguatesBuiltInMethodNames(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

//fgb:enum
type Status int

const StatusReady Status = 1

func (s Status) Values() int { return int(s) }
func (s Status) Value() int { return int(s) }
func (s Status) Name() string { return "status" }
func (s Status) Index() int { return int(s) }
func EchoStatus(value Status) Status { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"int values2()", "int value2()", "String name2()", "int index2()"} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("enum built-in method was not disambiguated as %q:\n%s", expected, apiDart)
		}
	}
	if len(warnings) < 4 {
		t.Fatalf("expected enum built-in collision warnings, got %v", warnings)
	}
}

func buildEnumFixture(t *testing.T, source string) (*unit, []error, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/enum-fixture\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved := config.Resolved{BaseDir: dir, GoInput: inputDir, GoOutput: filepath.Join(dir, "bridge_generated.go"), DartOutput: filepath.Join(dir, "dart", "bridge_generated.dart"), LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge", StopOnError: true}
	if _, err := WriteSupportPackage(resolved); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		return nil, nil, err
	}
	return buildUnit(api, resolved, false)
}
