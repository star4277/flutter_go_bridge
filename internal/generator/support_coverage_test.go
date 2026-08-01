package generator

import "testing"

// TestSupportPackageImportPathEmptyModule covers the early return when the
// module path or directory is empty.
func TestSupportPackageImportPathEmptyModule(t *testing.T) {
	if got := supportPackageImportPath("", "m", "s"); got != "" {
		t.Fatalf("empty module should yield empty import path, got %q", got)
	}
	if got := supportPackageImportPath("m", "", "s"); got != "" {
		t.Fatalf("empty module dir should yield empty import path, got %q", got)
	}
}

// TestSupportPackageImportPathAtRoot covers the "."-relative branch.
func TestSupportPackageImportPathAtRoot(t *testing.T) {
	if got := supportPackageImportPath("example.com/m", "/proj/m", "/proj/m"); got != "example.com/m" {
		t.Fatalf("same-dir support should map to the module path, got %q", got)
	}
}

// TestSupportPackageImportPathOutside covers the "../"-relative branch.
func TestSupportPackageImportPathOutside(t *testing.T) {
	if got := supportPackageImportPath("example.com/m", "/proj/m", "/other"); got != "" {
		t.Fatalf("outside-module support should yield empty import path, got %q", got)
	}
}
