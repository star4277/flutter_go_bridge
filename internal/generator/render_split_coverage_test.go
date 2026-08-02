package generator

import (
	"strings"
	"testing"

	bridgemodel "github.com/star4277/flutter_go_bridge/internal/model"
)

// TestRenderDecoderUnknownKind drives the default branch of renderDecoder,
// which has no decoder for an unknown kind.
func TestRenderDecoderUnknownKind(t *testing.T) {
	r := &splitDartRenderer{}
	err := r.renderDecoder(&wireType{Kind: "bogus", DartType: "Object?"})
	if err == nil || !strings.Contains(err.Error(), "no Dart decoder") {
		t.Fatalf("unknown decoder kind should error, got %v", err)
	}
}

// TestRenderEncoderUnknownKind drives the default branch of renderEncoder.
func TestRenderEncoderUnknownKind(t *testing.T) {
	r := &splitDartRenderer{}
	err := r.renderEncoder(&wireType{Kind: "bogus", DartType: "Object"})
	if err == nil || !strings.Contains(err.Error(), "no Dart encoder") {
		t.Fatalf("unknown encoder kind should error, got %v", err)
	}
}

// TestRenderDecoderNullableBigInt covers the nullable-BigInt branch of the
// signed decoder.
func TestRenderDecoderNullableBigInt(t *testing.T) {
	r := &splitDartRenderer{}
	err := r.renderDecoder(&wireType{Kind: kindSigned, DartType: "BigInt?"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.buffer.String(), "if (value == null) return null;") {
		t.Fatalf("nullable BigInt decoder should guard null:\n%s", r.buffer.String())
	}
}

// TestRenderNamedOpaqueEarlyReturn covers renderNamed's early return for a
// named type whose underlying is nil or opaque.
func TestRenderNamedOpaqueEarlyReturn(t *testing.T) {
	r := &splitDartRenderer{}
	r.renderNamed(&namedModel{Docs: "", Underlying: nil, Constants: []*constantModel{}})
	if r.buffer.Len() != 0 {
		t.Fatalf("an opaque/nil named type should render nothing, got %q", r.buffer.String())
	}
	r2 := &splitDartRenderer{}
	r2.renderNamed(&namedModel{
		DartName: "H", Underlying: &wireType{Kind: kindOpaque, DartType: "int"},
		Constants: []*constantModel{}, Methods: []*callModel{},
	})
	if r2.buffer.Len() != 0 {
		t.Fatalf("an opaque named type should render nothing, got %q", r2.buffer.String())
	}
}

// TestSortedImplementorsNonStructDepth covers the non-struct branch of the
// depth function used to order implementors.
func TestSortedImplementorsNonStructDepth(t *testing.T) {
	decl := &interfaceModel{
		Implementors: []*implementorModel{
			{DartName: "B", Type: &wireType{Kind: kindString, DartType: "String"}},
			{DartName: "A", Type: &wireType{Kind: kindString, DartType: "String"}},
		},
	}
	result := sortedImplementors(decl)
	if len(result) != 2 {
		t.Fatalf("expected two implementors, got %d", len(result))
	}
}

// TestRenderInstanceCallStream drives renderInstanceCall's stream-sink branch.
func TestRenderInstanceCallStream(t *testing.T) {
	r := &splitDartRenderer{}
	sink := &paramModel{
		GoName: "sink", DartName: "sink",
		Type: &wireType{Kind: kindStreamSink, DartType: "Sink<String>", Stream: &wireType{Kind: kindString, DartType: "String"}},
	}
	call := &callModel{ID: 1, DartName: "watch", Params: []*paramModel{sink}, StreamParam: sink}
	r.renderInstanceCall(call, "this", "__FGB_BRIDGE_CLASS__.instance", true, "  ")
	if !strings.Contains(r.buffer.String(), "Stream<String> watch(") {
		t.Fatalf("stream call should render a Stream return:\n%s", r.buffer.String())
	}
}

// TestRenderInstanceCallAsyncVoid drives the async call with no results, so
// the generated body `await`s the invocation instead of returning it.
func TestRenderInstanceCallAsyncVoid(t *testing.T) {
	r := &splitDartRenderer{}
	call := &callModel{ID: 2, DartName: "poke", Mode: bridgemodel.CallModeAsync, Results: []*resultModel{}}
	r.renderInstanceCall(call, "this", "__FGB_BRIDGE_CLASS__.instance", true, "  ")
	if !strings.Contains(r.buffer.String(), "await") {
		t.Fatalf("async void call should await the invocation:\n%s", r.buffer.String())
	}
}

func TestDartPathFieldEscapesDartStringSyntax(t *testing.T) {
	got := dartPathField("a\\b'c$d\n\r\t")
	want := `'$path.a\\b\'c\$d\n\r\t'`
	if got != want {
		t.Fatalf("dartPathField() = %q, want %q", got, want)
	}
}
