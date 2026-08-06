package generator

import (
	"strings"
	"testing"
)

// TestGenerateNilInterfaceEncodesAsNull verifies R1: a nil Go interface field
// encodes as null instead of raising "cannot send a nil X to Dart", so a
// zero-value struct can cross the bridge.
func TestGenerateNilInterfaceEncodesAsNull(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

type Shape interface { Area() int }

type Square struct { Side int }

func (value Square) Area() int { return value.Side * value.Side }

type Slot struct {
	Label string
	Kind  Shape
}

func ZeroSlot() Slot { return Slot{Label: "zero"} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(goSource, "cannot send a nil") {
		t.Fatalf("nil interface encoder must not raise an error:\n%s", goSource)
	}
	// gofmt expands the single-line nil guard into a block; check both parts.
	if !strings.Contains(goSource, "if value == nil {") || !strings.Contains(goSource, "return nil, nil") {
		t.Fatalf("nil interface must encode as null:\n%s", goSource)
	}
}

// TestGenerateDartAbsentClassForNilInterface verifies the Dart side of R1: a
// nil interface arriving from Go materializes as a generated _XxxAbsent
// stand-in whose methods throw, and the encoder maps it back to null.
func TestGenerateDartAbsentClassForNilInterface(t *testing.T) {
	_, central, _, _, err := generateFixture(t, `package api

type Shape interface { Area() int }

type Square struct { Side int }

func (value Square) Area() int { return value.Side * value.Side }

type Slot struct {
	Label string
	Kind  Shape
}

func ZeroSlot() Slot { return Slot{Label: "zero"} }
`)
	if err != nil {
		t.Fatal(err)
	}
	// The absent class lives in the central file alongside the decoders.
	for _, expected := range []string{
		"final class _ShapeAbsent implements Shape, GoAbsent",
		"const _ShapeAbsent()",
		"throw StateError(",
		"Shape(absent)",
		"if (value == null) {",
		"if (value is GoAbsent) {",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("generated central Dart missing absent-class element %q:\n%s", expected, central)
		}
	}
}

// TestGenerateNullableInterfaceFieldKeepsRealNull verifies that a field marked
// fgb:"nullable" still receives a real Dart null (not the absent stand-in)
// when the Go value is nil.
func TestGenerateNullableInterfaceFieldKeepsRealNull(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

type Shape interface { Area() int }

type Square struct { Side int }

func (value Square) Area() int { return value.Side * value.Side }

type NullableSlot struct {
	Label string
	Kind  Shape `+"`fgb:\"nullable\"`"+`
}

func Echo(value NullableSlot) NullableSlot { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final Shape? kind;") {
		t.Fatalf("nullable interface field should be Shape?:\n%s", apiDart)
	}
	// dartFieldDecode must keep the null for nullable fields instead of letting
	// the shared decoder materialize the absent stand-in.
	if !strings.Contains(central, "== null ? null :") {
		t.Fatalf("nullable field decode must preserve real null:\n%s", central)
	}
}

// TestGenerateDependencyInterfaceAbsentClass verifies that a dependency
// interface (marker-only, no methods) also gets an absent class in the central
// file. The class has no method overrides, but the GoAbsent marker and
// toString are still emitted.
func TestGenerateDependencyInterfaceAbsentClass(t *testing.T) {
	_, central, _, _, err := generateFixture(t, `package api

import "fmt"

type Report struct {
	Printer fmt.Stringer
}

func ZeroReport() Report { return Report{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"final class _StringerAbsent implements Stringer, GoAbsent",
		"Stringer(absent)",
		"if (value == null) {",
		"if (value is GoAbsent) {",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("dependency interface absent class missing %q in central:\n%s", expected, central)
		}
	}
}

// TestGenerateInterfaceTagSchemeIsConsistent verifies R3: every implementor of
// the same interface uses the same tag scheme (all string content tags or all
// integer indices), decided once by the interface-level UsesContentTags field.
func TestGenerateInterfaceTagSchemeIsConsistent(t *testing.T) {
	// Input-package interface: integer tags.
	_, _, inputGo, _, err := generateFixture(t, `package api

type Shape interface { Area() int }

type Square struct { Side int }
func (value Square) Area() int { return value.Side * value.Side }

type Circle struct { Radius int }
func (value Circle) Area() int { return value.Radius * value.Radius * 3 }

func Echo(value Shape) Shape { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	// The Go encoder must use int64 tags, and the decoder must use integer
	// parsing for an input-package interface.
	if !strings.Contains(inputGo, "int64(0)") {
		t.Fatalf("input-package interface must use integer tags:\n%s", inputGo)
	}
	if strings.Contains(inputGo, "raw[0].(string)") {
		t.Fatalf("input-package interface must not use string tag parsing:\n%s", inputGo)
	}

	// Dependency interface: string content tags.
	_, _, depGo, _, err := generateFixture(t, `package api

import "fmt"

type Text struct{ Value string }
func (value Text) String() string { return value.Value }

type Box struct{ Value fmt.Stringer }

func Echo(value Box) Box { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	// The Go decoder must use string tag parsing for a dependency interface.
	if !strings.Contains(depGo, "raw[0].(string)") {
		t.Fatalf("dependency interface must use string tag parsing:\n%s", depGo)
	}
	if strings.Contains(depGo, "fgbAsInt64(raw[0]") {
		t.Fatalf("dependency interface must not use integer tag parsing:\n%s", depGo)
	}
}
