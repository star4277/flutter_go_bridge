package generator

import (
	"strings"
	"testing"
)

func TestGenerateDartValueObjectMethods(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

type Child struct { Value int }
type Record struct {
	Name   string
	Tags   []string
	Counts map[string]int
	Data   []byte
	Child  *Child
}

func Echo(value Record) Record { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"bool operator ==(Object other) =>",
		"fgbInternalDeepEquals(tags, other.tags)",
		"int get hashCode => Object.hashAll([",
		"String toString() => fgbInternalJsonFor(this) ?? super.toString();",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart value object missing %q:\n%s", expected, apiDart)
		}
	}
	for _, expected := range []string{
		"bool fgbInternalDeepEquals(Object? left, Object? right)",
		"int fgbInternalDeepHash(Object? value)",
		"fgbInternalAttachJson",
		"Expando<String>",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("generated Dart runtime missing %q", expected)
		}
	}
	for _, expected := range []string{
		`"encoding/json"`,
		"encoded, err := json.Marshal(value)",
		`result["\x00fgb_json"] = fgbJSONText(value)`,
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go JSON transport missing %q:\n%s", expected, goSource)
		}
	}
	if strings.Contains(apiDart, "jsonEncode(") || strings.Contains(apiDart, "toJson()") {
		t.Fatalf("Dart must not rebuild Go JSON itself:\n%s", apiDart)
	}
}

func TestGeneratedOpaqueCarriesGoJSON(t *testing.T) {
	apiDart, central, goSource, _, err := generateFixture(t, `package api

type Proxy struct { state chan int }

func (p *Proxy) MarshalJSON() ([]byte, error) {
	return []byte("{\"type\":\"proxy\"}"), nil
}

func NewProxy() *Proxy { return &Proxy{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Proxy extends GoOpaque {") {
		t.Fatalf("proxy should fall back to GoOpaque:\n%s", apiDart)
	}
	for _, expected := range []string{
		"return []any{int64(handle), fgbJSONText(value)}, nil",
		"result := C.fgb_dco_array_new(2)",
		"jsonValue, err := fgbDcoJSONText(value)",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("opaque JSON envelope missing %q:\n%s", expected, goSource)
		}
	}
	for _, expected := range []string{
		"expected opaque handle and optional Go JSON",
		"fgbInternalAttachJson(Proxy.fgbInternal",
		"String toString() => fgbInternalJsonFor(this) ?? super.toString();",
	} {
		if !strings.Contains(apiDart+central, expected) {
			t.Fatalf("opaque Dart JSON attachment missing %q", expected)
		}
	}
}

func TestGeneratedGoOpaqueUsesHandleIdentity(t *testing.T) {
	_, central, _, _, err := generateFixture(t, `package api

//fgb:opaque
type Counter struct { value int }

func NewCounter() *Counter { return &Counter{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"other is GoOpaque",
		"identical(other.fgbBridge, fgbBridge)",
		"other.fgbHandle == fgbHandle",
		"Object.hash(runtimeType, identityHashCode(fgbBridge), fgbHandle)",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("GoOpaque identity implementation missing %q", expected)
		}
	}
}
