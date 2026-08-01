package names

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/iancoleman/strcase"
)

var invalidIdentifier = regexp.MustCompile(`[^A-Za-z0-9_$]`)

var dartReserved = map[string]struct{}{
	"abstract": {}, "as": {}, "assert": {}, "async": {}, "await": {},
	"base": {}, "break": {}, "case": {}, "catch": {}, "class": {},
	"const": {}, "continue": {}, "covariant": {}, "default": {},
	"deferred": {}, "do": {}, "dynamic": {}, "else": {}, "enum": {},
	"export": {}, "extends": {}, "extension": {}, "external": {}, "factory": {},
	"false": {}, "final": {}, "finally": {}, "for": {}, "Function": {},
	"get": {}, "hide": {}, "if": {}, "implements": {}, "import": {},
	"in": {}, "interface": {}, "is": {}, "late": {}, "library": {},
	"mixin": {}, "new": {}, "null": {}, "of": {}, "on": {}, "operator": {},
	"part": {}, "required": {}, "rethrow": {}, "return": {}, "sealed": {},
	"set": {}, "show": {}, "static": {}, "super": {}, "switch": {},
	"sync": {}, "this": {}, "throw": {}, "true": {}, "try": {}, "typedef": {},
	"var": {}, "void": {}, "when": {}, "while": {}, "with": {}, "yield": {},
}

func LowerCamel(value string) string {
	return Sanitize(strcase.ToLowerCamel(normalizeIdentifier(value)), false)
}

func UpperCamel(value string) string {
	return Sanitize(strcase.ToCamel(normalizeIdentifier(value)), true)
}

func normalizeIdentifier(value string) string {
	runes := []rune(value)
	if len(runes) < 2 {
		return value
	}
	var words []string
	start := 0
	for index := 1; index < len(runes); index++ {
		previous := runes[index-1]
		current := runes[index]
		var next rune
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		boundary := unicode.IsUpper(current) && (unicode.IsLower(previous) || unicode.IsDigit(previous))
		boundary = boundary || unicode.IsUpper(previous) && unicode.IsUpper(current) && next != 0 && unicode.IsLower(next)
		if boundary {
			words = append(words, string(runes[start:index]))
			start = index
		}
	}
	words = append(words, string(runes[start:]))
	return strings.Join(words, "_")
}

func Sanitize(value string, upper bool) string {
	value = invalidIdentifier.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		if upper {
			return "Generated"
		}
		return "generated"
	}

	runes := []rune(value)
	if unicode.IsDigit(runes[0]) {
		if upper {
			value = "Generated" + value
		} else {
			value = "n" + value
		}
	} else if upper {
		runes[0] = unicode.ToUpper(runes[0])
		value = string(runes)
	} else {
		runes[0] = unicode.ToLower(runes[0])
		value = string(runes)
	}

	if _, found := dartReserved[value]; found {
		return value + "_"
	}
	return value
}

// CIdentifier turns a Go/wire field name into a stable identifier suitable
// for generated C structs. It intentionally keeps the original spelling
// where possible so the generated CST remains readable.
func CIdentifier(value string) string {
	value = invalidIdentifier.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "field"
	}
	if unicode.IsDigit([]rune(value)[0]) {
		value = "field_" + value
	}
	if _, found := cReserved[value]; found {
		value = "field_" + value
	}
	// cgo exposes C struct members through Go selectors. A name such as
	// `type` is valid in C but cannot appear in `value.type`, so the shared CST
	// field spelling must also avoid Go keywords.
	if _, found := goReserved[value]; found {
		value += "_"
	}
	// The same identifier is emitted as a Dart FFI Struct field.  Keep it
	// legal in both languages; a suffix preserves readability and avoids a
	// second, generator-local name mapping that could drift from the C/Go side.
	if _, found := dartReserved[value]; found {
		value += "_"
	}
	return value
}

var goReserved = map[string]struct{}{
	"break": {}, "default": {}, "func": {}, "interface": {}, "select": {},
	"case": {}, "defer": {}, "go": {}, "map": {}, "struct": {},
	"chan": {}, "else": {}, "goto": {}, "package": {}, "switch": {},
	"const": {}, "fallthrough": {}, "if": {}, "range": {}, "type": {},
	"continue": {}, "for": {}, "import": {}, "return": {}, "var": {},
}

var cReserved = map[string]struct{}{
	"auto": {}, "break": {}, "case": {}, "char": {}, "const": {}, "continue": {},
	"default": {}, "do": {}, "double": {}, "else": {}, "enum": {}, "extern": {},
	"float": {}, "for": {}, "goto": {}, "if": {}, "inline": {}, "int": {},
	"long": {}, "register": {}, "restrict": {}, "return": {}, "short": {},
	"signed": {}, "sizeof": {}, "static": {}, "struct": {}, "switch": {},
	"typedef": {}, "union": {}, "unsigned": {}, "void": {}, "volatile": {},
	"while": {},
}

func LibraryBase(modulePath string) string {
	parts := strings.Split(strings.Trim(modulePath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "flutter_go_bridge"
	}
	last := parts[len(parts)-1]
	if len(parts) > 1 && len(last) > 1 && last[0] == 'v' {
		allDigits := true
		for _, r := range last[1:] {
			allDigits = allDigits && unicode.IsDigit(r)
		}
		if allDigits {
			last = parts[len(parts)-2]
		}
	}
	return strings.ReplaceAll(strcase.ToSnake(last), "-", "_")
}
