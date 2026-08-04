package generator

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGenerateDependencyInterfaceUsesStableReachableTypeTags(t *testing.T) {
	generate := func(extraImport, extraCall string) (string, string) {
		t.Helper()
		source := `package api

import (
	"fmt"
` + extraImport + `
)

type Text struct { Value string }

func (value Text) String() string { return value.Value }

type Box struct {
	Value fmt.Stringer
	Text  Text
}

func Echo(value Box) Box { return value }

` + extraCall
		_, dartSource, goSource, _, err := generateFixture(t, source)
		if err != nil {
			t.Fatal(err)
		}
		return dartSource, goSource
	}

	baseDart, baseGo := generate("", "")
	extraDart, extraGo := generate(`"net/url"`, `func Where(value url.URL) string { return value.Path }`)
	for _, source := range []string{baseDart, extraDart, baseGo, extraGo} {
		for _, tag := range []string{`"example.com/fixture/api.Text"`, `"fmt.Stringer#opaque"`} {
			if !strings.Contains(source, tag) {
				t.Fatalf("generated source missing stable dependency-interface tag %s:\n%s", tag, source)
			}
		}
		if strings.Contains(source, `"net/url.URL"`) {
			t.Fatalf("an unrelated imported type entered the dependency-interface union:\n%s", source)
		}
	}
}

func TestGenerateInputInterfaceKeepsInt64WireTags(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

type Shape interface { Area() int }

type Square struct { Side int }

func (value Square) Area() int { return value.Side * value.Side }

func Echo(value Shape) Shape { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSource, "return []any{int64(0), encoded}, nil") {
		t.Fatalf("input-package interface must retain its historical int64 tag encoding:\n%s", goSource)
	}
}

func TestGenerateOrdinaryErrorValuesAsNullableText(t *testing.T) {
	apiDart, central, goSource, warnings, err := generateFixture(t, `package api

type Box struct { Err error }

func Echo(value Box) Box { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("the dedicated error mapping should not emit dependency-interface warnings: %v", warnings)
	}
	if !strings.Contains(apiDart, "final String? err;") {
		t.Fatalf("ordinary error fields should expose nullable text:\n%s", apiDart)
	}
	if strings.Contains(central, "String??") {
		t.Fatalf("nullable error codecs must not append a second question mark:\n%s", central)
	}
	for _, expected := range []string{
		"expected error string",
		"return value.Error(), nil",
		"return fmt.Errorf(\"%s\", raw), nil",
	} {
		if !strings.Contains(central+goSource, expected) {
			t.Fatalf("generated error codec missing %q:\n%s", expected, central+goSource)
		}
	}
}

func TestGenerateFixedImportInterfaceImplementorCompilesWithoutAlias(t *testing.T) {
	var fixtureDir string
	_, _, goSource, _, err := generateFixture(t, `package api

import (
	"fmt"
	"os"
)

type Box struct {
	Value fmt.Stringer
	State *os.ProcessState
}

func Echo(value Box) Box { return value }
`, func(dir string) { fixtureDir = dir })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(goSource, "fgbext") {
		t.Fatalf("a fixed runtime import was registered again under an external alias:\n%s", goSource)
	}
	if !strings.Contains(goSource, `"*os.ProcessState"`) {
		t.Fatalf("the reachable fixed-package implementor is missing its stable tag:\n%s", goSource)
	}
	if strings.Contains(goSource, `"*time.Location"`) {
		t.Fatalf("a dedicated time.Time mapping leaked its private representation into the interface union:\n%s", goSource)
	}
	command := exec.Command("go", "vet", "./...")
	command.Dir = fixtureDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated Go with a fixed-package interface implementor failed go vet: %v\n%s", err, output)
	}
}

func TestCstAsyncPanicRollsBackOpaqueTransfer(t *testing.T) {
	start := strings.Index(goRuntimeSource, "func fgb_cst_async")
	end := strings.Index(goRuntimeSource[start:], "//export fgb_dco_free")
	if start < 0 || end < 0 {
		t.Fatal("could not locate fgb_cst_async in generated runtime")
	}
	body := goRuntimeSource[start : start+end]
	for _, expected := range []string{
		"if recovered := recover(); recovered != nil",
		"if transfer != nil { transfer.rollback() }",
		"fgbDcoEnvelopeError",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("fgb_cst_async panic path missing %q:\n%s", expected, body)
		}
	}
}
