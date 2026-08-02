package generator

import (
	"strings"
	"testing"
)

func TestCstRenderersIgnoreUnreachableStandardOnlyTypes(t *testing.T) {
	if got := cstDcoReachableTypes(nil); got != nil {
		t.Fatalf("nil unit should have no CST/DCO types, got %#v", got)
	}
	standardOnly := &wireType{ID: 1, Kind: kindStruct, DartType: "StandardOnly"}
	standardOnly.Struct = &structModel{Type: standardOnly}
	unit := &unit{
		Types:        []*wireType{standardOnly},
		codecSupport: map[codecCacheKey]bool{},
		Calls: []*callModel{{
			Codec: codecModePack{DartToGo: codecModeStandard, GoToDart: codecModeStandard},
		}},
	}
	if got := cstDcoReachableTypes(unit); len(got) != 0 {
		t.Fatalf("standard-only call should not make types CST/DCO reachable, got %#v", got)
	}

	dart := &splitDartRenderer{unit: unit}
	dart.renderCstEncoders()
	if dart.buffer.Len() != 0 {
		t.Fatalf("unreachable Dart CST encoder leaked into output:\n%s", dart.buffer.String())
	}
	goCode := &goRenderer{unit: unit}
	goCode.renderCstDecoders()
	goCode.renderDcoEncoders()
	if strings.Contains(goCode.buffer.String(), "fgbCstDecode") || strings.Contains(goCode.buffer.String(), "fgbDcoEncode") {
		t.Fatalf("unreachable Go CST/DCO codec leaked into output:\n%s", goCode.buffer.String())
	}
}

func TestCstTypedListAndStructDco(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

type Item struct {
	Count int32
	Label string
}

func Process(list []int32, longs []int64, flts []float64, item Item) (Item, error) {
	return item, nil
}
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"unsafe.Slice((*C.int32_t)(unsafe.Pointer(value.ptr))",
		"unsafe.Slice((*C.int64_t)(unsafe.Pointer(value.ptr))",
		"unsafe.Slice((*C.double)(unsafe.Pointer(value.ptr))",
		"DCO struct allocation failed",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("goSource missing %q", expected)
		}
	}
}

func TestDcoStructEncodeNullableField(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

type Item struct {
	Tags []string `+"`fgb:\"nullable\"`"+`
	Age  int
}

func Take(item Item) (Item, error) { return item, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSource, "if value.Tags == nil {") {
		t.Fatalf("nullable DCO branch not generated")
	}
}
