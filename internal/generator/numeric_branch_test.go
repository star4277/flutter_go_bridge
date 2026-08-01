package generator

import (
	"strings"
	"testing"
)

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
