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
