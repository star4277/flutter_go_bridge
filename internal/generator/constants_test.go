package generator

import (
	"strings"
	"testing"
)

func TestTypedConstantLiteralsEmitted(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Str string
type B bool
type I int
type U uint64
type L int32
type F float64

const (
	A Str  = "hey"
	C B    = true
	D I    = 3
	E U    = 7
	Z L    = -9
	Y F    = 1.5
)

func Use(a Str, b B, c I, d U, e L, f F) Str { return a }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`static const a = Str("hey");`,
		`static const c = B(true);`,
		`static const d = I(3);`,
		`static final e = U(BigInt.parse("7"));`,
		`static const z = L(-9);`,
		`static const y = F(3/2);`,
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("api.dart missing %q:\n%s", expected, apiDart)
		}
	}
}

func TestSupportsCodecStandard(t *testing.T) {
	typ := &wireType{ID: 1, Kind: kindString}
	if !typ.supportsCodec(codecModeStandard, map[int]bool{}) {
		t.Fatal("standard codec should always be supported")
	}
	if !typ.supportsCodec(codecModeDCO, map[int]bool{}) {
		t.Fatal("string should support DCO")
	}
	if !typ.supportsCodec(codecModeCST, map[int]bool{}) {
		t.Fatal("string should support CST")
	}
	// map/any cannot use CST/DCO.
	m := &wireType{ID: 2, Kind: kindMap}
	if m.supportsCodec(codecModeCST, map[int]bool{}) {
		t.Fatal("map should not support CST")
	}
}

func TestNumberedSource(t *testing.T) {
	out := numberedSource([]byte("hello\nworld"))
	expected := "   1 | hello\n   2 | world\n"
	if out != expected {
		t.Fatalf("got:\n%q", out)
	}
}


