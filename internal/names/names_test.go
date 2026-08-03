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
	if got := LowerCamel("123-value"); got != "n123Value" {
		t.Fatalf("numeric lower camel name must not start with underscore, got %q", got)
	}
	if got := UpperCamel("123-value"); got != "Generated123Value" {
		t.Fatalf("numeric upper camel name must not start with underscore, got %q", got)
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

func TestNormalizeIdentifierSplitsCamelCase(t *testing.T) {
	cases := map[string]string{
		"HTTPServerURL": "HTTP_Server_URL",
		"XMLHttpRequest": "XML_Http_Request",
		"GetURL2":       "Get_URL2",
		"a":             "a",
		"ab":            "ab",
	}
	for in, want := range cases {
		if got := normalizeIdentifier(in); got != want {
			t.Errorf("normalizeIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeSpecial(t *testing.T) {
	if got := Sanitize("___", true); got != "Generated" {
		t.Errorf("empty upper = %q", got)
	}
	if got := Sanitize("___", false); got != "generated" {
		t.Errorf("empty lower = %q", got)
	}
	if got := Sanitize("---hello---", true); got != "Hello" {
		t.Errorf("trimmed upper = %q", got)
	}
	if got := Sanitize("---hello---", false); got != "hello" {
		t.Errorf("trimmed lower = %q", got)
	}
	if got := Sanitize("miXeD", true); got != "MiXeD" {
		t.Errorf("mixed upper = %q", got)
	}
	if got := Sanitize("MiXeD", false); got != "miXeD" {
		t.Errorf("mixed lower = %q", got)
	}
	if got := Sanitize("9lives", true); got != "Generated9lives" {
		t.Errorf("digit upper = %q", got)
	}
	if got := Sanitize("9lives", false); got != "n9lives" {
		t.Errorf("digit lower = %q", got)
	}
	if got := Sanitize("class", false); got != "class_" {
		t.Errorf("dart reserved lower = %q", got)
	}
	if got := Sanitize("when", true); got != "When" {
		t.Errorf("dart reserved upper = %q", got)
	}
}

func TestCIdentifierVariousReservations(t *testing.T) {
	if got := CIdentifier(""); got != "field" {
		t.Errorf("empty = %q", got)
	}
	if got := CIdentifier("123_x"); got != "field_123_x" {
		t.Errorf("digit = %q", got)
	}
	if got := CIdentifier("auto"); got != "field_auto" {
		t.Errorf("c reserved = %q", got)
	}
	if got := CIdentifier("return"); got != "field_return" {
		t.Errorf("c reserved return = %q", got)
	}
	if got := CIdentifier("type"); got != "type_" {
		t.Errorf("go keyword = %q", got)
	}
	if got := CIdentifier("struct"); got != "field_struct" {
		t.Errorf("nested struct/dart = %q", got)
	}
	if got := CIdentifier("class"); got != "class_" {
		t.Errorf("dart reserved = %q", got)
	}
	if got := CIdentifier("user_id"); got != "user_id" {
		t.Errorf("plain = %q", got)
	}
}

func TestLibraryBaseVariants(t *testing.T) {
	if got := LibraryBase("example.com/foo"); got != "foo" {
		t.Errorf("base = %q", got)
	}
	if got := LibraryBase("example.com/acme/plugin/v2"); got != "plugin" {
		t.Errorf("versioned = %q", got)
	}
	if got := LibraryBase("example.com/acme/plugin/vital"); got != "vital" {
		t.Errorf("v-prefixed non-version = %q", got)
	}
	if got := LibraryBase(""); got != "flutter_go_bridge" {
		t.Errorf("root = %q", got)
	}
	if got := LibraryBase("/"); got != "flutter_go_bridge" {
		t.Errorf("slash = %q", got)
	}
	if got := LibraryBase("example.com/my-kebab"); got != "my_kebab" {
		t.Errorf("kebab = %q", got)
	}
}

func TestAmbientDartTypeGuard(t *testing.T) {
	// dart:core needs no import, so these shadow the ambient declaration.
	for _, ambient := range []string{"List", "Map", "Set", "Duration", "DateTime", "String", "Error", "Object", "Future", "Stream"} {
		if !IsDartAmbientType(ambient) {
			t.Errorf("IsDartAmbientType(%q) = false, want true", ambient)
		}
		if got := AvoidDartAmbientType(ambient); got != "Go"+ambient {
			t.Errorf("AvoidDartAmbientType(%q) = %q, want %q", ambient, got, "Go"+ambient)
		}
	}
	// Emitted unprefixed by the renderers, so they are ambient in practice too.
	for _, ambient := range []string{"Uint8List", "Int32List", "Float64List", "StreamSink", "InternetAddress"} {
		if !IsDartAmbientType(ambient) {
			t.Errorf("IsDartAmbientType(%q) = false, want true", ambient)
		}
	}
	// dart:ffi is imported with a prefix, so its names are free.
	for _, free := range []string{"Pointer", "NativeFinalizer", "User", "Item", "GoList", "Session"} {
		if IsDartAmbientType(free) {
			t.Errorf("IsDartAmbientType(%q) = true, want false", free)
		}
		if got := AvoidDartAmbientType(free); got != free {
			t.Errorf("AvoidDartAmbientType(%q) = %q, want it unchanged", free, got)
		}
	}
}

