package generator

import (
	"strings"
	"testing"
)

// TestRenderNamedWithMethods drives renderNamed's method loop.
func TestRenderNamedWithMethods(t *testing.T) {
	r := &splitDartRenderer{}
	method := &callModel{ID: 3, DartName: "greet", Results: []*resultModel{}}
	named := &namedModel{
		DartName:   "Pet",
		Underlying: &wireType{Kind: kindString, DartType: "String"},
		Constants:  []*constantModel{},
		Methods:    []*callModel{method},
	}
	r.renderNamed(named)
	if !strings.Contains(r.buffer.String(), "extension type const Pet(String value)") {
		t.Fatalf("named type extension was not rendered:\n%s", r.buffer.String())
	}
	if !strings.Contains(r.buffer.String(), "void greet(") {
		t.Fatalf("named type method was not rendered:\n%s", r.buffer.String())
	}
}
