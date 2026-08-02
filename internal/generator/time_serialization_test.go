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
		"return DateTime.fromMicrosecondsSinceEpoch(value);",
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
		"return time.UnixMicro(raw), nil",
		"return time.UnixMicro(int64(value)), nil",
		"return value.UnixMicro(), nil",
		"return fgbDcoInt64(value.UnixMicro())",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go source missing %q:\n%s", expected, goSource)
		}
	}
	if strings.Contains(goSource, "time.Parse(time.RFC3339Nano") || strings.Contains(goSource, "Format(time.RFC3339Nano)") {
		t.Fatalf("generated Go time codecs must not use text serialization")
	}
}
