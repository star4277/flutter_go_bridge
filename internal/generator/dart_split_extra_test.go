package generator

import "testing"

// TestDartImportPathFallback drives dartImportPath's "."-relative fallback.
func TestDartImportPathFallback(t *testing.T) {
	got := dartImportPath("/a/b.py", "/a/b.py")
	if got != "\"b.py\"" {
		t.Fatalf("dartImportPath same-file should fall back to the base name, got %q", got)
	}
}

// TestDartSourceGroupsEmpty drives the "<generated>" placeholder used when a
// unit has no source files at all.
func TestDartSourceGroupsEmpty(t *testing.T) {
	groups, paths := dartSourceGroups(&unit{})
	if len(groups) != 1 || len(paths) != 1 {
		t.Fatalf("expected one generated group, got groups=%d paths=%d", len(groups), len(paths))
	}
	if paths["<generated>"] != "_generated.dart" {
		t.Fatalf("unexpected generated path: %q", paths["<generated>"])
	}
}
