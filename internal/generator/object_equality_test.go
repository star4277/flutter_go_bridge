package generator

import (
	"strings"
	"testing"
)

// Two structs decoded from the same wire bytes are the same Go value, so the
// generated class needs structural equality rather than Dart's identity default.
func TestGenerateStructEqualityForFieldedTypes(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

type Point struct {
	X int
	Y int
}

func Echo(p Point) Point { return p }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"  bool operator ==(Object other) =>",
		"      other is Point &&",
		"          other.runtimeType == runtimeType &&",
		"          fgbInternalDeepEquals(x, other.x) &&",
		"          fgbInternalDeepEquals(y, other.y);",
		"  int get hashCode {",
		"    var result = runtimeType.hashCode;",
		"    result = result * 31 + fgbInternalDeepHash(x);",
		"    result = result * 31 + fgbInternalDeepHash(y);",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing %q:\n%s", expected, apiDart)
		}
	}
	// The helpers the generated code calls have to exist in the runtime.
	for _, expected := range []string{
		"bool fgbInternalDeepEquals(Object? left, Object? right) {",
		"int fgbInternalDeepHash(Object? value) {",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("runtime missing %q", expected)
		}
	}
}

// A struct with no bridged fields has nothing to compare, so every instance
// would be equal to every other. Identity equality is more useful there. A
// GoOpaque handle is left alone for a different reason: its identity is the
// handle, and the same Go object can cross the bridge under several of them.
func TestGenerateStructEqualityOmittedWithoutFields(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Marker struct{}

type Hidden struct{ secret int }

func EchoMarker(m Marker) Marker { return m }

func EchoHidden(h *Hidden) *Hidden { return h }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "operator ==") || strings.Contains(apiDart, "get hashCode") {
		t.Fatalf("neither an empty struct nor a GoOpaque handle should gain equality:\n%s", apiDart)
	}
}

// An embedded struct becomes a Dart superclass. The subclass must compare its
// promoted fields too, and must never equal an instance of the parent.
func TestGenerateStructEqualitySpansInheritance(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Base struct{ ID int }

type Derived struct {
	Base
	Label string
}

func Echo(d Derived) Derived { return d }
`)
	if err != nil {
		t.Fatal(err)
	}
	derived := apiDart[strings.Index(apiDart, "class Derived"):]
	for _, expected := range []string{
		"          fgbInternalDeepEquals(id, other.id) &&",
		"          fgbInternalDeepEquals(label, other.label);",
		"    result = result * 31 + fgbInternalDeepHash(id);",
		"    result = result * 31 + fgbInternalDeepHash(label);",
	} {
		if !strings.Contains(derived, expected) {
			t.Fatalf("subclass equality missing %q:\n%s", expected, derived)
		}
	}
	// runtimeType keeps a parent holding the same values from comparing equal.
	if strings.Count(apiDart, "other.runtimeType == runtimeType") != 2 {
		t.Fatalf("both classes need the runtimeType guard:\n%s", apiDart)
	}
}

// Collection fields must compare by content, and a map must not depend on
// insertion order.
func TestGenerateStructEqualityUsesDeepHelpersForCollections(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

type Tags struct {
	Names  []string
	Counts map[string]int
}

func Echo(t Tags) Tags { return t }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"          fgbInternalDeepEquals(names, other.names) &&",
		"          fgbInternalDeepEquals(counts, other.counts);",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing %q:\n%s", expected, apiDart)
		}
	}
	// A scalar key must use the linear lookup rather than pairwise matching.
	if !strings.Contains(central, "if (left.keys.every(_fgbLooksUpByValue)) {") {
		t.Fatalf("map equality should keep a scalar-key fast path:\n%s", central)
	}
	// Map hashing has to stay commutative so entry order cannot change it.
	if !strings.Contains(central, "result ^= fgbInternalDeepHash(entry.key) * 31 + fgbInternalDeepHash(entry.value);") {
		t.Fatalf("map hashing should combine entries commutatively:\n%s", central)
	}
}
