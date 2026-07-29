package names

import "testing"

func TestDartNames(t *testing.T) {
	if got := LowerCamel("HTTPServerURL"); got != "httpServerUrl" {
		t.Fatalf("got %q", got)
	}
	if got := LowerCamel("class"); got != "class_" {
		t.Fatalf("got %q", got)
	}
	if got := UpperCamel("hello_world"); got != "HelloWorld" {
		t.Fatalf("got %q", got)
	}
}

func TestLibraryBase(t *testing.T) {
	if got := LibraryBase("example.com/acme/my-plugin/v2"); got != "my_plugin" {
		t.Fatalf("got %q", got)
	}
}

func TestCIdentifierAvoidsGoKeywordsUsedByCgoSelectors(t *testing.T) {
	if got := CIdentifier("type"); got != "type_" {
		t.Fatalf("got %q, want type_", got)
	}
	if got := CIdentifier("struct"); got != "field_struct" {
		t.Fatalf("got %q, want field_struct", got)
	}
}
