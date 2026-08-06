package generator

import (
	"strings"
	"testing"
)

// dcoEncoderBody returns the generated Go -> Dart encoder for one Go type.
func dcoEncoderBody(t *testing.T, goSource, goType string) string {
	t.Helper()
	marker := "(value " + goType + ", depth int, transfer *fgbOpaqueTransfer)"
	start := strings.Index(goSource, marker)
	if start < 0 {
		t.Fatalf("no DCO encoder for %s in:\n%s", goType, goSource)
	}
	body := goSource[start:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}
	return body
}

// A Go value must never be narrowed into a signed slot too small to hold it.
// uint32(0xFFFFFFFF) sent as int32 arrives in Dart as -1, which contradicts the
// documented mapping of uint32 to a Dart int. The standard codec already picks
// the slot by range; the DCO path has to agree with it.
func TestUnsignedDcoEncodersDoNotNarrow(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

func ReturnUnsigned() (uint8, uint16, uint32, uint64, error) { return 0, 0, 0, 0, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		goType string
		want   string
	}{
		// Both fit in int32 with room to spare, so they stay in the smaller slot.
		{"uint8", "return fgbDcoInt32(int32(value))"},
		{"uint16", "return fgbDcoInt32(int32(value))"},
		// 4294967295 does not fit in int32.
		{"uint32", "return fgbDcoInt64(int64(value))"},
		// Anything wider than the signed 64-bit slot travels as a hex string.
		{"uint64", "return fgbDcoString(new(big.Int).SetUint64(uint64(value)).Text(16))"},
	} {
		if body := dcoEncoderBody(t, goSource, test.goType); !strings.Contains(body, test.want) {
			t.Fatalf("%s encoder should use %q:\n%s", test.goType, test.want, body)
		}
	}
}

func TestNarrowSignedDecodeRanges(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

import (
	"math/big"
)

func TakeNarrowSigned(a int8, b int16, c int32) (int8, int16, int32, error) { return a, b, c, nil }

func TakeIntAndUint(
	a int,
	b uint8, c uint16, d uint32, e uint,
	f uintptr,
) (int, uint8, uint16, uint32, uint, uintptr, error) { return a, b, c, d, e, f, nil }

func TakeBigInt(n big.Int, p *big.Int) (big.Int, *big.Int, error) { return n, p, nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"integer out of int8 range",
		"integer out of int16 range",
		"integer out of int32 range",
		"integer out of int range",
		"integer out of uint8 range",
		"integer out of uint16 range",
		"integer out of uint32 range",
		"invalid hexadecimal BigInt",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("goSource missing %q", expected)
		}
	}
}
