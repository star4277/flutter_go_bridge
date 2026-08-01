package generator

import (
	"strings"
	"testing"
)

// TestRenderGoWithPreamble drives renderGo's GoPreamble branch.
func TestRenderGoWithPreamble(t *testing.T) {
	out, err := renderGo(&unit{GoPreamble: "package fixtures"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "package fixtures") {
		t.Fatalf("renderGo should embed the Go preamble:\n%s", out)
	}
}
