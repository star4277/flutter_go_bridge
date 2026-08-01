package generator

import (
	"go/types"
	"strings"
	"testing"
)

// TestGoRenderDecoderUnknownKind drives the default branch of the Go decoder.
func TestGoRenderDecoderUnknownKind(t *testing.T) {
	r := &goRenderer{unit: &unit{}}
	err := r.renderDecoder(&wireType{Kind: "bogus", Original: types.Typ[types.Int]})
	if err == nil || !strings.Contains(err.Error(), "no Go decoder") {
		t.Fatalf("unknown decoder kind should error, got %v", err)
	}
}

// TestGoRenderEncoderUnknownKind drives the default branch of the Go encoder.
func TestGoRenderEncoderUnknownKind(t *testing.T) {
	r := &goRenderer{unit: &unit{}}
	err := r.renderEncoder(&wireType{Kind: "bogus", Original: types.Typ[types.Int]})
	if err == nil || !strings.Contains(err.Error(), "no Go encoder") {
		t.Fatalf("unknown encoder kind should error, got %v", err)
	}
}
