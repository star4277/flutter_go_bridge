package generator

import (
	"strings"
	"testing"
)

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
