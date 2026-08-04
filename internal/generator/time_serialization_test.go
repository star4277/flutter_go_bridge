package generator

import (
	"strings"
	"testing"
)

func TestGenerateTimeSerializationUsesEpochMicroseconds(t *testing.T) {
	_, dartSource, goSource, _, err := generateFixture(t, `package api

import "time"

func Echo(value time.Time) time.Time { return value }
`)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"if (value is! int) throw FormatException('$path: expected time microseconds');",
		"if (value < -8640000000000000000 || value > 8640000000000000000)",
		"return DateTime.fromMicrosecondsSinceEpoch(value, isUtc: true);",
		"return value.microsecondsSinceEpoch;",
	} {
		if !strings.Contains(dartSource, expected) {
			t.Fatalf("generated Dart source missing %q:\n%s", expected, dartSource)
		}
	}
	if strings.Contains(dartSource, "DateTime.parse(value)") || strings.Contains(dartSource, "toIso8601String()") {
		t.Fatalf("generated Dart time codecs must not use text serialization")
	}

	for _, expected := range []string{
		"return time.UnixMicro(raw).UTC(), nil",
		"return time.UnixMicro(int64(value)).UTC(), nil",
		"return fgbTimeUnixMicro(value)",
		"return fgbDcoInt64(micros)",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go source missing %q:\n%s", expected, goSource)
		}
	}
	if strings.Contains(goSource, "time.Parse(time.RFC3339Nano") {
		t.Fatalf("generated Go time codecs must not parse text serialization")
	}
}
