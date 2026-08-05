package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/model"
)

// callableByName indexes the parsed methods of a fixture by their Go name.
func callableByName(api *model.API) map[string]*model.Callable {
	result := map[string]*model.Callable{}
	for _, callable := range api.Callables {
		result[callable.Func.Name()] = callable
	}
	return result
}

// Every operator in the table has to be reachable from Go, so the fixture
// declares one qualifying method per symbol and expects all of them back.
func TestDartOperatorTableCoversEveryOperator(t *testing.T) {
	dir := t.TempDir()
	writeFixtureModule(t, dir, `package api

type Point struct{ X int }

func (p Point) Add(other Point) Point                 { return p }
func (p Point) Subtract(other Point) Point            { return p }
func (p Point) Multiply(other Point) Point            { return p }
func (p Point) Divide(other Point) Point              { return p }
func (p Point) TruncateDivide(other Point) Point      { return p }
func (p Point) Modulo(other Point) Point              { return p }
func (p Point) BitwiseAnd(other Point) Point          { return p }
func (p Point) BitwiseOr(other Point) Point           { return p }
func (p Point) BitwiseXor(other Point) Point          { return p }
func (p Point) ShiftLeft(other Point) Point           { return p }
func (p Point) ShiftRight(other Point) Point          { return p }
func (p Point) LessThan(other Point) bool             { return false }
func (p Point) GreaterThan(other Point) bool          { return false }
func (p Point) LessThanOrEqualTo(other Point) bool    { return false }
func (p Point) GreaterThanOrEqualTo(other Point) bool { return false }
func (p Point) BitwiseNot() Point                     { return p }

func Echo(p Point) Point { return p }
`)
	methods := callableByName(parseFixtureAPI(t, dir, filepath.Join(dir, "api")))

	expected := map[string]string{
		"Add": "+", "Subtract": "-", "Multiply": "*", "Divide": "/",
		"TruncateDivide": "~/", "Modulo": "%",
		"BitwiseAnd": "&", "BitwiseOr": "|", "BitwiseXor": "^",
		"ShiftLeft": "<<", "ShiftRight": ">>",
		"LessThan": "<", "GreaterThan": ">",
		"LessThanOrEqualTo": "<=", "GreaterThanOrEqualTo": ">=",
		"BitwiseNot": "~",
	}
	if len(expected) != len(dartOperators) {
		t.Fatalf("the fixture covers %d operators but the table holds %d", len(expected), len(dartOperators))
	}
	for name, symbol := range expected {
		method, found := methods[name]
		if !found {
			t.Fatalf("fixture is missing method %s", name)
		}
		if got := dartOperatorFor(method); got != symbol {
			t.Fatalf("%s maps to %q, want %q", name, got, symbol)
		}
	}
	// A plain function is never an operator, whatever it is called.
	if got := dartOperatorFor(methods["Echo"]); got != "" {
		t.Fatalf("a top-level function became operator %q", got)
	}
}

// Sharing a name with an operator is not enough: every condition has to hold,
// and a method that fails one stays an ordinary method rather than being
// reshaped into something Dart would reject.
func TestDartOperatorSignatureRules(t *testing.T) {
	dir := t.TempDir()
	writeFixtureModule(t, dir, `package api

import "context"

type Value struct{ N int }

// Accepted: a pointer operand is still the receiver's own type.
func (v Value) Add(other *Value) Value { return v }

// Accepted: a failing operator may report one error, thrown on the Dart side.
func (v Value) Subtract(other Value) (Value, error) { return v, nil }

// Accepted: the bridge supplies the context, so it is not an operand.
func (v Value) Multiply(ctx context.Context, other Value) Value { return v }

// Rejected: a pointer result would let a division be null.
func (v Value) Divide(other Value) *Value { return &v }

// Rejected: the operand is not the receiver's type.
func (v Value) Modulo(scale int) Value { return v }

// Rejected: a binary operator takes exactly one operand.
func (v Value) ShiftLeft(first Value, second Value) Value { return v }

// Rejected: a relational operator has to return bool.
func (v Value) LessThan(other Value) Value { return v }

// Rejected: ~ takes no operand.
func (v Value) BitwiseNot(other Value) Value { return v }

// Rejected: two values leave Dart no single result to hand back.
func (v Value) BitwiseAnd(other Value) (Value, Value) { return v, v }

// Rejected: the name is not one of the operators.
func (v Value) Scale(other Value) Value { return v }

// Rejected: an operator cannot return a Future.
//fgb:async
func (v Value) BitwiseOr(other Value) Value { return v }

func Echo(v Value) Value { return v }
`)
	methods := callableByName(parseFixtureAPI(t, dir, filepath.Join(dir, "api")))

	for name, symbol := range map[string]string{"Add": "+", "Subtract": "-", "Multiply": "*"} {
		if got := dartOperatorFor(methods[name]); got != symbol {
			t.Fatalf("%s maps to %q, want %q", name, got, symbol)
		}
	}
	for _, name := range []string{
		"Divide", "Modulo", "ShiftLeft", "LessThan", "BitwiseNot", "BitwiseAnd", "Scale", "BitwiseOr",
	} {
		method, found := methods[name]
		if !found {
			t.Fatalf("fixture is missing method %s", name)
		}
		if got := dartOperatorFor(method); got != "" {
			t.Fatalf("%s should stay an ordinary method, got operator %q", name, got)
		}
	}
	// Guard the nil paths the builder never reaches but callers could.
	if got := dartOperatorFor(nil); got != "" {
		t.Fatalf("nil callable produced operator %q", got)
	}
}

// A qualifying method is rendered as the operator itself, with the positional
// operand Dart requires, and is not additionally exposed under its ordinary
// name.
func TestRenderDartOperators(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Point struct {
	X int
	Y int
}

func (p Point) Add(other Point) Point       { return p }
func (p Point) Subtract(other *Point) Point { return p }
func (p Point) LessThan(other Point) bool   { return p.X < other.X }
func (p Point) BitwiseNot() Point           { return p }
func (p Point) Scale(factor int) Point      { return p }

func Echo(p Point) Point { return p }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"  Point operator +(Point other) {",
		// A pointer operand keeps its nullable Dart spelling.
		"  Point operator -(Point? other) {",
		"  bool operator <(Point other) {",
		"  Point operator ~() {",
		// A method that qualifies for nothing keeps the ordinary named-parameter
		// shape.
		"  Point scale({required int factor}) {",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing %q:\n%s", expected, apiDart)
		}
	}
	// The operator replaces the ordinary method rather than doubling it.
	for _, unwanted := range []string{
		"Point add(", "Point subtract(", "bool lessThan(", "Point bitwiseNot(",
	} {
		if strings.Contains(apiDart, unwanted) {
			t.Fatalf("operator method also exposed as %q:\n%s", unwanted, apiDart)
		}
	}
}

// An operator holds no ordinary member name. A subclass method that merely
// shares the promoted operator's Go name overrides nothing, and rejecting it as
// an incompatible shadow would refuse a perfectly valid pair of members.
func TestRenderDartOperatorDoesNotShadowOrdinaryMethod(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Base struct{ ID int }

func (b Base) Add(other Base) Base { return b }

type Derived struct {
	Base
	Label string
}

// Not an operator: the operand is not the receiver's type, so this stays an
// ordinary method next to the inherited operator +.
func (d Derived) Add(scale int) Derived { return d }

func Echo(d Derived) Derived { return d }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	for _, expected := range []string{
		"  Base operator +(Base other) {",
		"  Derived add({required int scale}) {",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing %q:\n%s", expected, apiDart)
		}
	}
	// The ordinary method overrides nothing, so it must not claim to.
	if strings.Contains(apiDart, "@override\n  Derived add(") {
		t.Fatalf("add() is not an override of operator +:\n%s", apiDart)
	}
}

// A method renamed onto the Dart spelling an operator's Go name would have used
// keeps that name: the operator is not occupying it.
func TestRenderDartOperatorLeavesOrdinaryNameFree(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Point struct{ X int }

func (p Point) Add(other Point) Point { return p }

//fgb:rename = "add"
func (p Point) Combine(other Point) Point { return p }

func Echo(p Point) Point { return p }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected rename warnings: %v", warnings)
	}
	for _, expected := range []string{
		"  Point operator +(Point other) {",
		"  Point add({required Point other}) {",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "add2") {
		t.Fatalf("the ordinary method should not have been renamed:\n%s", apiDart)
	}
}

// A subclass cannot own its own version of a promoted operator: its operand and
// result would have to be its own type, which is not the signature Dart accepts
// as an override of the parent's. The generator says so instead of emitting Dart
// that fails to analyze.
func TestRenderDartOperatorSubclassRedeclarationIsRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Base struct{ ID int }

func (b Base) Add(other Base) Base { return b }

type Derived struct {
	Base
	Label string
}

func (d Derived) Add(other Derived) Derived { return d }

func Echo(d Derived) Derived { return d }
`)
	if err == nil || !strings.Contains(err.Error(), "shadows") {
		t.Fatalf("expected a shadowing error, got %v", err)
	}
}

// An interface owns the Dart shape of the methods it declares. Turning an
// implementation into an operator would leave the `implements` clause without
// the named member the interface promises, so the operator is given up and the
// reason is reported.
func TestRenderDartOperatorYieldsToInterfaceContract(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

type Adder interface {
	Add(other Point) Point
}

type Point struct{ X int }

func (p Point) Add(other Point) Point { return p }

func Combine(a Adder) Point { return a.Add(Point{}) }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "  Point add({required Point other}) {") {
		t.Fatalf("the implementation must keep the interface's named member:\n%s", apiDart)
	}
	if strings.Contains(apiDart, "operator +") {
		t.Fatalf("an interface-claimed method must not become an operator:\n%s", apiDart)
	}
	// The interface declaration itself is never an operator either.
	if !strings.Contains(apiDart, "abstract interface class Adder") {
		t.Fatalf("missing interface declaration:\n%s", apiDart)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "matches Dart operator +") {
			found = true
		}
	}
	if !found {
		t.Fatalf("giving up an operator has to be reported: %v", warnings)
	}
}

// An extension type carries operators too: a named Go type with a qualifying
// method is the same rule as a struct.

func TestRenderDartOperatorsOnNamedType(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Meters float64

func (m Meters) Add(other Meters) Meters      { return m + other }
func (m Meters) GreaterThan(other Meters) bool { return m > other }

func Echo(m Meters) Meters { return m }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"extension type const Meters(double value) {",
		"  Meters operator +(Meters other) {",
		"  bool operator >(Meters other) {",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("missing %q:\n%s", expected, apiDart)
		}
	}
}
