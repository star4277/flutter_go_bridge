package generator

import (
	"go/types"
	"strings"
	"testing"

	bridgemodel "github.com/star4277/flutter_go_bridge/internal/model"
)

func TestGenerateToStringMethodPrecedenceAndStringAlias(t *testing.T) {
	apiDart, central, _, warnings, err := generateFixture(t, `package api

type Value struct { Name string }

func (v Value) MarshalJSON() ([]byte, error) { return []byte("json"), nil }
func (v Value) String() string { return "string" }
func (v Value) ToString() string { return "custom" }
func Use(v Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !strings.Contains(apiDart, "String toString()") || !strings.Contains(central, "fgbInternalCall") {
		t.Fatalf("expected Go-backed toString override:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "asString()") {
		t.Fatal("String should remain available as asString when ToString wins")
	}
}

func TestGenerateToStringFallsBackForRequiredArguments(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Value struct { Name string }

func (v Value) ToString(prefix string) string { return prefix + v.Name }
func (v Value) String() string { return v.Name }
func Use(v Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for required ToString argument")
	}
	if !strings.Contains(apiDart, "String toString()") || strings.Contains(apiDart, "asString()") {
		t.Fatalf("expected fallback to String-backed toString without duplicate alias:\n%s", apiDart)
	}
}

func TestGenerateToStringUsesLocalFieldsWithoutGoMethod(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Value struct { Name string; Count int }

func Use(v Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "String toString() => 'Value(name: $name, count: $count)';") {
		t.Fatalf("expected local field representation:\n%s", apiDart)
	}
}

func TestGenerateToStringMarshalJSONDecodesUTF8(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Value struct { Name string }

func Use(v Value) {}

func (v Value) MarshalJSON() ([]byte, error) { return []byte("json"), nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "import 'dart:convert';") || !strings.Contains(apiDart, "utf8.decode") {
		t.Fatalf("expected MarshalJSON UTF-8 decode:\n%s", apiDart)
	}
}

func TestGenerateToStringOptionalGoArguments(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Value struct { Name string }

func (v Value) ToString(prefix *string) string {
	if prefix == nil { return v.Name }
	return *prefix + v.Name
}
func Use(v Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || !strings.Contains(apiDart, "String toString({String? prefix})") {
		t.Fatalf("optional ToString parameters should remain a valid override, warnings=%v:\n%s", warnings, apiDart)
	}
}

func TestGenerateToStringWarnsForIneligibleStringAndMarshalJSON(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Value struct { Name string }

func (v Value) String(prefix string) string { return prefix + v.Name }
func (v Value) MarshalJSON(prefix string) ([]byte, error) { return []byte(prefix + v.Name), nil }
func Use(v Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) < 2 || !strings.Contains(apiDart, "String toString() => 'Value(name: $name)';") {
		t.Fatalf("ineligible methods should warn and use local fallback, warnings=%v:\n%s", warnings, apiDart)
	}
}

func TestGenerateToStringNamedAndEmptyStructFallbacks(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type ID string
type Empty struct{}

func UseID(value ID) ID { return value }
func UseEmpty(value Empty) Empty { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "String toString() => value.toString();") {
		t.Fatalf("named value type should have a local toString:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "String toString() => 'Empty()';") {
		t.Fatalf("empty structs should still have a deterministic local toString:\n%s", apiDart)
	}
}

func TestGenerateToStringInterfaceStringContract(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Printer interface { String() string }
type Value struct { Name string }

func (v Value) String() string { return v.Name }
func Use(value Printer) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(apiDart, "String toString()") < 2 || strings.Contains(apiDart, "String string()") {
		t.Fatalf("interface and implementation should share the toString contract:\n%s", apiDart)
	}
}

func TestGenerateToStringIncludesPromotedMethodsInPrecedence(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Parent struct { Name string }
func (v Parent) ToString() string { return "parent:" + v.Name }

type Child struct { Parent; Extra string }
func (v Child) String() string { return "child:" + v.Extra }

func Use(value Child) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(apiDart, "class Child")
	if start < 0 {
		t.Fatalf("missing Child class:\n%s", apiDart)
	}
	child := apiDart[start:]
	if !strings.Contains(child, "String asString()") || strings.Contains(child, "String toString() => 'Child(") {
		t.Fatalf("promoted ToString should outrank Child.String:\n%s", child)
	}
}

func TestGenerateToStringDisambiguatesReservedNames(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Value struct { Name string }

func (v Value) ToString() string { return v.Name }
func (v Value) String() string { return v.Name }
func (v Value) AsString() string { return v.Name }
func Use(value Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 || !strings.Contains(apiDart, "String asString()") || !strings.Contains(apiDart, "String asString2()") {
		t.Fatalf("reserved asString name should be disambiguated, warnings=%v:\n%s", warnings, apiDart)
	}
}

func TestGenerateToStringRejectsWrongMarshalJSONResultOrder(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Value struct { Name string }

func (v Value) MarshalJSON() (error, []byte) { return nil, []byte(v.Name) }
func Use(value Value) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 || !strings.Contains(apiDart, "String toString() => 'Value(name: $name)';") {
		t.Fatalf("non-json.Marshaler result order should use local fallback, warnings=%v:\n%s", warnings, apiDart)
	}
}

func TestToStringEligibilityBranches(t *testing.T) {
	b := &builder{}
	stringResult := &resultModel{Type: &wireType{Kind: kindString}}
	required := &paramModel{Type: &wireType{Original: types.Typ[types.String]}}
	optional := &paramModel{Type: &wireType{Original: types.NewPointer(types.Typ[types.String])}}
	for name, test := range map[string]struct {
		call *callModel
		want bool
	}{
		"nil":          {call: nil},
		"async":        {call: &callModel{Mode: bridgemodel.CallModeAsync, Results: []*resultModel{stringResult}}},
		"error":        {call: &callModel{Mode: bridgemodel.CallModeSync, ErrorCount: 1, Results: []*resultModel{stringResult}}},
		"no result":    {call: &callModel{Mode: bridgemodel.CallModeSync}},
		"wrong result": {call: &callModel{Mode: bridgemodel.CallModeSync, Results: []*resultModel{{Type: &wireType{Kind: kindSigned}}}}},
		"required":     {call: &callModel{Mode: bridgemodel.CallModeSync, Params: []*paramModel{required}, Results: []*resultModel{stringResult}}},
		"optional":     {call: &callModel{Mode: bridgemodel.CallModeSync, Params: []*paramModel{optional}, Results: []*resultModel{stringResult}}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := b.eligibleToString(test.call); got != test.want {
				t.Fatalf("eligibleToString() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGenerateToStringNamedMarshalJSONImportsConvert(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Code string

func (value Code) MarshalJSON() ([]byte, error) { return []byte(value), nil }
func Use(value Code) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "import 'dart:convert';") || !strings.Contains(apiDart, "utf8.decode") {
		t.Fatalf("named MarshalJSON toString should import dart:convert:\n%s", apiDart)
	}
}
