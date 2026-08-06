package generator

import (
	"strings"
	"testing"
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
