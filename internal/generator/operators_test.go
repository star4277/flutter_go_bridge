package generator

import (
	"path/filepath"
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
