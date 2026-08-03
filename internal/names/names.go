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

// dartAmbientTypes are type names that a generated Dart library sees without
// writing an import for them. A generated top-level declaration reusing one of
// these names shadows the ambient declaration inside that library, and the
// generated code itself relies on the ambient meaning: `List<T>` for slices,
// `Map<K, V>` for maps, `Duration(microseconds: ...)` for time.Duration,
// `DateTime` for time.Time, `Uint8List` for []byte, `Stream`/`StreamSink` for
// stream parameters. Shadowing therefore does not merely confuse a reader, it
// makes the generated library fail to compile.
//
// The set covers all of dart:core, which needs no import at all, plus the
// names from dart:typed_data, dart:async and dart:io that the renderers emit
// unprefixed. dart:ffi is imported with the `ffi` prefix, so its names cannot
// collide and are deliberately absent.
var dartAmbientTypes = map[string]struct{}{
	// dart:core
	"Object": {}, "Null": {}, "Never": {}, "Function": {}, "Type": {},
	"Symbol": {}, "Record": {}, "Enum": {}, "Deprecated": {}, "Invocation": {},
	"BigInt": {}, "String": {}, "Comparable": {}, "Comparator": {},
	"Duration": {}, "DateTime": {}, "Stopwatch": {},
	"Uri": {}, "UriData": {}, "RegExp": {}, "Match": {}, "Pattern": {},
	"List": {}, "Map": {}, "MapEntry": {}, "Set": {},
	"Iterable": {}, "Iterator": {}, "Sink": {}, "StringSink": {},
	"StringBuffer": {}, "Runes": {}, "RuneIterator": {}, "Expando": {},
	"WeakReference": {}, "Finalizer": {},
	"StackTrace": {}, "Exception": {}, "Error": {},
	"ArgumentError": {}, "AssertionError": {}, "CastError": {},
	"ConcurrentModificationError": {}, "FormatException": {}, "IndexError": {},
	"IntegerDivisionByZeroException": {}, "NoSuchMethodError": {},
	"OutOfMemoryError": {}, "RangeError": {}, "StackOverflowError": {},
	"StateError": {}, "TypeError": {}, "UnimplementedError": {},
	"UnsupportedError": {},
	// dart:core re-exports these two from dart:async.
	"Future": {}, "Stream": {},
	// dart:async, emitted unprefixed by the stream renderers.
	"StreamController": {}, "StreamSink": {}, "StreamSubscription": {},
	"Completer": {}, "Timer": {}, "FutureOr": {},
	// dart:typed_data, emitted unprefixed for byte and numeric slices.
	"ByteBuffer": {}, "ByteData": {}, "Endian": {}, "TypedData": {},
	"Int8List": {}, "Uint8List": {}, "Uint8ClampedList": {},
	"Int16List": {}, "Uint16List": {}, "Int32List": {}, "Uint32List": {},
	"Int64List": {}, "Uint64List": {}, "Float32List": {}, "Float64List": {},
	// dart:io, emitted unprefixed for netip.Addr.
	"InternetAddress": {}, "InternetAddressType": {},
}

// IsDartAmbientType reports whether a generated top-level Dart type named
// value would shadow a type that is already in scope in the generated library.
func IsDartAmbientType(value string) bool {
	_, found := dartAmbientTypes[value]
	return found
}

// AvoidDartAmbientType returns a top-level Dart type name that cannot shadow an
// ambient type. A `Go` prefix keeps the original spelling readable, where an
// underscore or digit suffix would not say why the name changed.
//
// The result is only a proposal: callers still have to resolve it against the
// names they have already handed out.
func AvoidDartAmbientType(value string) string {
	if IsDartAmbientType(value) {
		return "Go" + value
	}
	return value
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
