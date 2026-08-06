package generator

import (
	"strings"
	"testing"
)

func TestGeneratedDartRuntimeUsesBracedControlFlow(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

func Ping() {}
`)
	if err != nil {
		t.Fatal(err)
	}

	for sourceName, source := range map[string]string{"central": central, "api": apiDart} {
		// The Dart analyzer's curly_braces lint rejects a control statement
		// whose body is written on the same line. Looking at the final
		// parenthesis on each control line handles for-loop semicolons.
		for lineNumber, line := range strings.Split(source, "\n") {
			trimmed := strings.TrimSpace(line)
			isControl := strings.HasPrefix(trimmed, "if (") ||
				strings.HasPrefix(trimmed, "else if (") ||
				strings.HasPrefix(trimmed, "for (") ||
				strings.HasPrefix(trimmed, "while (")
			if !isControl {
				if strings.HasPrefix(trimmed, "else ") && !strings.HasPrefix(trimmed, "else if ") {
					after := strings.TrimSpace(strings.TrimPrefix(trimmed, "else "))
					if after != "" && !strings.HasPrefix(after, "{") {
						t.Fatalf("generated %s Dart contains an unbraced single-line else at line %d: %q", sourceName, lineNumber+1, line)
					}
				}
				continue
			}
			close := strings.LastIndex(trimmed, ")")
			if close < 0 {
				continue
			}
			after := strings.TrimSpace(trimmed[close+1:])
			if after != "" && !strings.HasPrefix(after, "{") {
				t.Fatalf("generated %s Dart contains an unbraced single-line control flow at line %d: %q", sourceName, lineNumber+1, line)
			}
		}
	}
}
