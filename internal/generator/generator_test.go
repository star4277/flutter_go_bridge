package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
)

func TestGenerateStableABIAndDartAPIDL(t *testing.T) {
	input := filepath.Join("..", "parser", "testdata", "sample")
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: input, BaseDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	resolved := config.Resolved{
		BaseDir: dir, GoInput: input,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "lib", "bridge_generated.dart"),
		LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge",
		DartFormatLineLength: 100, StopOnError: true,
	}
	result, err := Generate(api, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("got %d files, want Go bridge, central Dart, and api.dart", len(result.Files))
	}
	for _, legacy := range []string{
		strings.TrimSuffix(resolved.GoOutput, ".go") + ".c",
		strings.TrimSuffix(resolved.GoOutput, ".go") + ".h",
	} {
		if _, err := os.Stat(legacy); !os.IsNotExist(err) {
			t.Fatalf("legacy C companion must not be generated: %s", legacy)
		}
	}
	goSource := mustRead(t, resolved.GoOutput)
	for _, expected := range []string{
		"//export fgb_init", "//export fgb", "//export fgb_async",
		"//export fgb_cst", "//export fgb_cst_async", "//export fgb_dco_free",
		"fgbStandardMethodCodec", "fgb_internal_post_bytes", "typedef FgbDartCObject Dart_CObject",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go source missing %q", expected)
		}
	}
	dartSource := mustRead(t, resolved.DartOutput)
	for _, expected := range []string{"final class FixtureBridge", "FgbPlatformException", "NativeApi.initializeApiDLData", "ReceivePort", "lookupFunction<_FgbSyncNative"} {
		if !strings.Contains(dartSource, expected) {
			t.Fatalf("generated Dart source missing %q", expected)
		}
	}
	if strings.Contains(dartSource, "native_assets") || strings.Contains(goSource, "native_assets") {
		t.Fatal("generated output must not depend on Native Assets")
	}
	if strings.Contains(dartSource, "package:flutter/") || strings.Contains(dartSource, "dispose(") {
		t.Fatal("generated Dart must be pure Dart and must not expose manual dispose")
	}
	sourceFile := mustRead(t, filepath.Join(dir, "lib", "api.dart"))
	for _, expected := range []string{
		"int calculate", "fgbInternalCall", `import "bridge_generated.dart";`,
		"final class Value",
	} {
		if !strings.Contains(sourceFile, expected) {
			t.Fatalf("generated per-source Dart library missing %q", expected)
		}
	}
	if strings.Contains(sourceFile, "\nexport ") {
		t.Fatalf("per-source Dart library must not re-export anything:\n%s", sourceFile)
	}
	if _, err := os.Stat(filepath.Join(dir, "lib", "_types.dart")); !os.IsNotExist(err) {
		t.Fatal("a separate _types.dart must not be generated")
	}
}

func TestGenerateCstDcoStructFieldsAndStandardFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/codec-modes\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

type Child struct { Value int }

type Value struct {
	Count int
	Optional *int
	Child *Child
}

func RoundTrip(value Value) Value { return value }

func Fallback(value map[string]int) map[string]int { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "codec_modes", DartEntrypointClassName: "CodecBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	apiDart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	for _, expected := range []string{
		"final int count;", "final int? optional;", "final Child? child;",
		"required this.count", "this.optional", "this.child", "fgbInternalCall",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart API missing %q:\n%s", expected, apiDart)
		}
	}
	if strings.Contains(apiDart, "dart:ffi") || strings.Contains(apiDart, "fgbInvokeCst") {
		t.Fatalf("per-source API leaked FFI details:\n%s", apiDart)
	}

	central := mustRead(t, dartOutput)
	for _, expected := range []string{
		"extends ffi.Struct", "external ffi.Pointer<ffi.Int64> optional;",
		"fgbInvokeCstSync(0", `fgbInvokeSync("Fallback"`, "if (value is List)",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("central Dart bridge missing %q", expected)
		}
	}

	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		"int64_t count;", "int64_t* optional;", "fgbDispatchCst", "fgbDcoEncode",
		`case "Fallback":`, "//export fgb_cst", "//export fgb_dco_free",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing %q", expected)
		}
	}
}

func TestGenerateExternalRepositoryStructTypes(t *testing.T) {
	dir := t.TempDir()
	rootModule := `module example.com/fixture

go 1.24

require example.com/thirdparty v0.0.0

replace example.com/thirdparty => ./thirdparty
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(rootModule), 0o644); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(dir, "thirdparty")
	if err := os.MkdirAll(filepath.Join(externalDir, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(externalDir, "atomic"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "go.mod"), []byte("module example.com/thirdparty\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	atomicSource := `package atomic

type Int64 struct { value int64 }

func (value *Int64) Load() int64 { return value.value }
func (value *Int64) Store(next int64) { value.value = next }
`
	if err := os.WriteFile(filepath.Join(externalDir, "atomic", "atomic.go"), []byte(atomicSource), 0o644); err != nil {
		t.Fatal(err)
	}
	externalSource := "package models\n\nimport \"example.com/thirdparty/atomic\"\n\n" +
		"type Status string\n\n" +
		"type Address struct {\n\tCity string\n}\n\n" +
		"type Profile struct {\n" +
		"\tID int64\n\tName string `json:\"display_name\"`\n" +
		"\tAddress *Address\n\tStatus Status\n" +
		"\tUpload atomic.Int64\n\tOptional *atomic.Int64\n\tsecret string\n}\n"
	if err := os.WriteFile(filepath.Join(externalDir, "models", "models.go"), []byte(externalSource), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	apiSource := `package api

import "example.com/thirdparty/models"

type Profile struct {
	Local bool
}

func LocalProfile() Profile { return Profile{} }

func LoadProfile() *models.Profile { return &models.Profile{} }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(apiSource), 0o644); err != nil {
		t.Fatal(err)
	}

	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "fixture", DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	apiDart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	for _, expected := range []string{
		"Profile localProfile()", "ModelsProfile? loadProfile()", `import "../_generated.dart";`,
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart API missing %q:\n%s", expected, apiDart)
		}
	}
	externalDart := mustRead(t, filepath.Join(dir, "dart", "_generated.dart"))
	for _, expected := range []string{
		"final class ModelsProfile", "final int id;", "final String displayName;",
		"final Address? address;", "final Status status;", "final class Address",
		"extension type const Status(String value)", "final int upload;", "final int? optional;",
	} {
		if !strings.Contains(externalDart, expected) {
			t.Fatalf("generated external Dart types missing %q:\n%s", expected, externalDart)
		}
	}
	if strings.Contains(externalDart, "secret") {
		t.Fatalf("unexported external fields must not be generated:\n%s", externalDart)
	}
	if strings.Contains(externalDart, "class Int64") || strings.Contains(externalDart, "AtomicInt64") {
		t.Fatalf("atomic wrappers must map directly to their loaded scalar type:\n%s", externalDart)
	}
	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		`fgbext0 "example.com/thirdparty/models"`, `"example.com/thirdparty/atomic"`,
		"fgbext0.Profile", "fgbext0.Address", "fgbext0.Status", ".Load()", ".Store(decoded)",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing %q:\n%s", expected, goSource)
		}
	}
}

func TestGenerateSpecialExternalValueMappings(t *testing.T) {
	dir := t.TempDir()
	module := `module example.com/special

go 1.24

require (
	github.com/cockroachdb/apd/v3 v3.0.0
	github.com/gofrs/uuid/v5 v5.0.0
	github.com/shopspring/decimal v1.0.0
)

replace github.com/cockroachdb/apd/v3 => ./apd
replace github.com/gofrs/uuid/v5 => ./uuid
replace github.com/shopspring/decimal => ./shopspring
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	uuidDir := filepath.Join(dir, "uuid")
	if err := os.MkdirAll(uuidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uuidDir, "go.mod"), []byte("module github.com/gofrs/uuid/v5\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uuidSource := `package uuid

type UUID [16]byte

func FromString(string) (UUID, error) { return UUID{}, nil }
func (UUID) String() string { return "" }
`
	if err := os.WriteFile(filepath.Join(uuidDir, "uuid.go"), []byte(uuidSource), 0o644); err != nil {
		t.Fatal(err)
	}
	shopspringDir := filepath.Join(dir, "shopspring")
	if err := os.MkdirAll(shopspringDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shopspringDir, "go.mod"), []byte("module github.com/shopspring/decimal\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shopspringSource := `package decimal

type Decimal struct{}

func NewFromString(string) (Decimal, error) { return Decimal{}, nil }
func (Decimal) String() string { return "" }
`
	if err := os.WriteFile(filepath.Join(shopspringDir, "decimal.go"), []byte(shopspringSource), 0o644); err != nil {
		t.Fatal(err)
	}
	apdDir := filepath.Join(dir, "apd")
	if err := os.MkdirAll(apdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apdDir, "go.mod"), []byte("module github.com/cockroachdb/apd/v3\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apdSource := `package apd

type Form int8
const Finite Form = 0
type Condition uint32
type Decimal struct { Form Form }

func NewFromString(string) (*Decimal, Condition, error) { return &Decimal{}, 0, nil }
func (*Decimal) String() string { return "" }
`
	if err := os.WriteFile(filepath.Join(apdDir, "decimal.go"), []byte(apdSource), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

import (
	"github.com/cockroachdb/apd/v3"
	"github.com/gofrs/uuid/v5"
	"github.com/shopspring/decimal"
	"net/netip"
)

type Values struct {
	IP netip.Addr
	ID uuid.UUID
	OptionalIP *netip.Addr
	OptionalID *uuid.UUID
	Shop decimal.Decimal
	OptionalShop *decimal.Decimal
	APD apd.Decimal
	OptionalAPD *apd.Decimal
}

func Load() Values { return Values{} }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "dart", "bridge_generated.dart"),
		LibraryName: "special", DartEntrypointClassName: "SpecialBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DartDependencies) != 2 || result.DartDependencies[0] != "uuid" || result.DartDependencies[1] != "decimal" {
		t.Fatalf("got Dart dependencies %#v, want [uuid decimal]", result.DartDependencies)
	}
	dart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	for _, expected := range []string{
		"final InternetAddress ip;", "final UuidValue id;", "final InternetAddress? optionalIp;", "final UuidValue? optionalId;",
		"final Decimal shop;", "final Decimal? optionalShop;", "final Decimal apd;", "final Decimal? optionalApd;",
		"import 'dart:io';", "import 'package:uuid/uuid.dart';", "import 'package:decimal/decimal.dart';", `import "../bridge_generated.dart";`,
	} {
		if !strings.Contains(dart, expected) {
			t.Fatalf("special external mapping missing %q:\n%s", expected, dart)
		}
	}
	central := mustRead(t, filepath.Join(dir, "dart", "bridge_generated.dart"))
	for _, expected := range []string{
		"import 'package:uuid/uuid.dart';", "UuidValue.fromString",
		"import 'package:decimal/decimal.dart';", "Decimal.parse(value)", "return value.toString();",
		`fgbInvokeSync("Load"`,
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("public bridge missing %q:\n%s", expected, central)
		}
	}
	if strings.Contains(central, "typedef UUID") {
		t.Fatalf("the generator must use uuid's native UuidValue type directly:\n%s", central)
	}
	goSource := mustRead(t, filepath.Join(dir, "bridge_generated.go"))
	if count := strings.Count(goSource, `"net/netip"`); count != 1 {
		t.Fatalf("generated Go bridge should import net/netip exactly once, got %d:\n%s", count, goSource)
	}
	if count := strings.Count(goSource, `"github.com/gofrs/uuid/v5"`); count != 1 {
		t.Fatalf("generated Go bridge should import gofrs/uuid exactly once, got %d:\n%s", count, goSource)
	}
	for _, expected := range []string{
		`"github.com/shopspring/decimal"`, `"github.com/cockroachdb/apd/v3"`,
		"decimal.NewFromString(raw)", "apd.NewFromString(raw)",
		"value.Form != apd.Finite", "apd Decimal must be finite",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing Decimal support %q:\n%s", expected, goSource)
		}
	}
}

func TestGenerateExternalInterfaceImplementationsAcrossDependencies(t *testing.T) {
	dir := t.TempDir()
	rootModule := `module example.com/external-interface

go 1.24

require example.com/thirdparty v0.0.0

replace example.com/thirdparty => ./thirdparty
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(rootModule), 0o644); err != nil {
		t.Fatal(err)
	}

	externalDir := filepath.Join(dir, "thirdparty")
	for _, subdir := range []string{"contracts", "factory", "impl"} {
		if err := os.MkdirAll(filepath.Join(externalDir, subdir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(externalDir, "go.mod"), []byte("module example.com/thirdparty\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "contracts", "contracts.go"), []byte(`package contracts

type Shape interface {
	Area() int
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "impl", "impl.go"), []byte(`package impl

type Circle struct {
	radius int
}

func NewCircle(radius int) *Circle { return &Circle{radius: radius} }
func (*Circle) Area() int { return 0 }

type Base struct {
	Value int
}

type PointerEmbedded struct {
	*Base
}

func (PointerEmbedded) Area() int { return 0 }

type PointerValue struct {
	Value int
}

func (*PointerValue) Area() int { return 0 }

type Square struct {
	Size int
}

func (value Square) Area() int { return value.Size * value.Size }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "factory", "factory.go"), []byte(`package factory

import (
	"example.com/thirdparty/contracts"
	"example.com/thirdparty/impl"
)

type hidden struct{}

func (hidden) Area() int { return 0 }

func Shapes() map[string]contracts.Shape {
	return map[string]contracts.Shape{
		"circle": impl.NewCircle(2),
		"embedded": impl.PointerEmbedded{},
		"hidden": hidden{},
		"pointerValue": &impl.PointerValue{Value: 4},
		"square": impl.Square{Size: 3},
	}
}

func Circle() *impl.Circle { return impl.NewCircle(1) }
func PointerEmbedded() *impl.PointerEmbedded { return &impl.PointerEmbedded{} }
func PointerValue() *impl.PointerValue { return &impl.PointerValue{Value: 1} }
func Square() impl.Square { return impl.Square{Size: 1} }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(`package api

import (
	"example.com/thirdparty/contracts"
	"example.com/thirdparty/factory"
	"example.com/thirdparty/impl"
)

func Shapes() map[string]contracts.Shape { return factory.Shapes() }
func Circle() *impl.Circle { return factory.Circle() }
func PointerEmbedded() *impl.PointerEmbedded { return factory.PointerEmbedded() }
func PointerValue() *impl.PointerValue { return factory.PointerValue() }
func Square() impl.Square { return factory.Square() }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "external_interface", DartEntrypointClassName: "ExternalInterfaceBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	externalDart := mustRead(t, filepath.Join(dir, "dart", "_generated.dart"))
	for _, expected := range []string{
		"abstract interface class Shape {",
		"final class Circle extends GoOpaque implements Shape {",
		"final class PointerEmbedded extends GoOpaque implements Shape {",
		"final class PointerValue implements Shape {",
		"final class ShapeOpaque extends GoOpaque implements Shape {",
		"final class Square implements Shape {",
		"final int size;",
	} {
		if !strings.Contains(externalDart, expected) {
			t.Fatalf("external interface mapping missing %q:\n%s", expected, externalDart)
		}
	}
	if strings.Contains(externalDart, "int area(") {
		t.Fatalf("third-party interface methods are outside the input API and must remain marker-only:\n%s", externalDart)
	}
	centralDart := mustRead(t, dartOutput)
	for _, expected := range []string{
		"final decoded = fgbDecode",
		"nil Circle implementation payload",
		"return decoded;",
	} {
		if !strings.Contains(centralDart, expected) {
			t.Fatalf("nullable interface implementation decoder missing %q:\n%s", expected, centralDart)
		}
	}
	if strings.Contains(centralDart, "extends ffi.Struct {}") {
		t.Fatalf("standard-codec-only interface implementations leaked empty CST definitions:\n%s", centralDart)
	}
	if strings.Contains(centralDart, "if (value == null) return null;\n  if (value == null) return null;") {
		t.Fatalf("interface encoder duplicated the shared nullable guard:\n%s", centralDart)
	}

	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		`"example.com/thirdparty/contracts"`,
		`"example.com/thirdparty/impl"`,
		"case *fgbext",
		".Circle:",
		".Shape:",
		".Square:",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go interface union missing %q:\n%s", expected, goSource)
		}
	}
	if circle := strings.Index(goSource, ".Circle:"); circle < 0 {
		t.Fatal("generated Go interface union is missing Circle")
	} else if square := strings.Index(goSource, ".Square:"); square < 0 || circle > square {
		t.Fatalf("external implementation tags must be stable by package path and type name:\n%s", goSource)
	}
	if count := strings.Count(goSource, ".Square:"); count != 2 {
		t.Fatalf("a value-receiver implementation must accept both Square and *Square Go results, got %d cases:\n%s", count, goSource)
	}
	if count := strings.Count(goSource, ".PointerEmbedded:"); count != 2 || !strings.Contains(goSource, "fgbEncode") || !strings.Contains(goSource, "(&typed, depth+1, transfer)") {
		t.Fatalf("an opaque value implementation must accept T and *T by taking a copy address, got %d cases:\n%s", count, goSource)
	}
	if fallback := strings.LastIndex(goSource, ".Shape:"); fallback < strings.LastIndex(goSource, ".Square:") {
		t.Fatalf("open-world interface fallback must follow every named implementation:\n%s", goSource)
	}
}

func TestGenerateIPPrefixMapping(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/prefix\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

import "net/netip"

type Routes struct {
	Destination netip.Prefix
	Optional *netip.Prefix
	Extra []netip.Prefix
}

func RoundTripPrefix(value netip.Prefix) netip.Prefix { return value }

func RoundTripRoutes(value Routes) Routes { return value }

func RoundTripLabels(value map[netip.Prefix]string) map[netip.Prefix]string { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "prefix", DartEntrypointClassName: "PrefixBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	apiDart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	for _, expected := range []string{
		"final String destination;", "final String? optional;", "final List<String> extra;",
		"String roundTripPrefix", "Map<String, String> roundTripLabels",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart prefix API missing %q:\n%s", expected, apiDart)
		}
	}
	// netip.Prefix travels as plain text, so it must not drag dart:io in the way
	// netip.Addr does for InternetAddress.
	if strings.Contains(apiDart, "import 'dart:io';") {
		t.Fatalf("netip.Prefix must not require dart:io:\n%s", apiDart)
	}

	central := mustRead(t, dartOutput)
	if !strings.Contains(central, "expected CIDR prefix string") {
		t.Fatalf("central Dart bridge missing the prefix decoder guard:\n%s", central)
	}

	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		"netip.ParsePrefix(raw)", "return netip.Prefix{}, nil",
		"if !value.IsValid() {", "invalid CIDR prefix",
		"return fgbDcoString(value.String())",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing prefix mapping %q:\n%s", expected, goSource)
		}
	}
	if count := strings.Count(goSource, `"net/netip"`); count != 1 {
		t.Fatalf("generated Go bridge should import net/netip exactly once, got %d", count)
	}
}

func TestGenerateURLMapping(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/uri\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

import "net/url"

type Request struct {
	Target url.URL
	Fallback *url.URL
	Mirrors []url.URL
}

func RoundTripURL(value url.URL) url.URL { return value }

func RoundTripRequest(value Request) Request { return value }

func RoundTripLabels(value map[url.URL]string) map[url.URL]string { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "uri", DartEntrypointClassName: "UriBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	apiDart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	for _, expected := range []string{
		"final Uri target;", "final Uri? fallback;", "final List<Uri> mirrors;",
		"Uri roundTripUrl", "Map<Uri, String> roundTripLabels",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart URL API missing %q:\n%s", expected, apiDart)
		}
	}
	// url.URL must not be mistaken for a struct: it hides everything behind
	// unexported state once User (*url.Userinfo) is reached.
	if strings.Contains(apiDart, "Userinfo") || strings.Contains(apiDart, "class Url") {
		t.Fatalf("url.URL must map to Uri instead of a generated class:\n%s", apiDart)
	}

	central := mustRead(t, dartOutput)
	for _, expected := range []string{"return Uri.parse(value);", "return value.toString();"} {
		if !strings.Contains(central, expected) {
			t.Fatalf("central Dart bridge missing URL mapping %q:\n%s", expected, central)
		}
	}

	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		"url.Parse(raw)", "return *parsed, nil", "invalid URI",
		"return value.String(), nil", "return fgbDcoString(value.String())",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing URL mapping %q:\n%s", expected, goSource)
		}
	}
	if count := strings.Count(goSource, `"net/url"`); count != 1 {
		t.Fatalf("generated Go bridge should import net/url exactly once, got %d", count)
	}
}

// A third-party struct cannot carry fgb markers, so a type whose state is
// entirely unexported has to be detected and bridged as a handle. Emitting a
// fieldless Dart value class instead drops the payload and does not compile.
func TestGenerateStructWithoutBridgedFieldsBecomesOpaque(t *testing.T) {
	dir := t.TempDir()
	module := "module example.com/hidden\n\ngo 1.24\n\nrequire example.com/thirdparty v0.0.0\n\nreplace example.com/thirdparty => ./thirdparty\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(dir, "thirdparty")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "go.mod"), []byte("module example.com/thirdparty\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	externalSource := `package thirdparty

// Userinfo keeps all of its state unexported, like net/url.Userinfo.
type Userinfo struct {
	username string
	password string
}

// Hidden exports a field but excludes it from the wire.
type Hidden struct {
	Internal string ` + "`json:\"-\"`" + `
}

// Marker declares no fields at all and stays a value type.
type Marker struct{}

type Holder struct {
	Name string
	Info *Userinfo
}
`
	if err := os.WriteFile(filepath.Join(externalDir, "thirdparty.go"), []byte(externalSource), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

import "example.com/thirdparty"

func Load() thirdparty.Holder { return thirdparty.Holder{} }

func Ping() thirdparty.Marker { return thirdparty.Marker{} }

func Secret() *thirdparty.Hidden { return nil }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "dart", "bridge_generated.dart"),
		LibraryName: "hidden", DartEntrypointClassName: "HiddenBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	external := mustRead(t, filepath.Join(dir, "dart", "_generated.dart"))
	for _, expected := range []string{
		"final class Userinfo extends GoOpaque {", "final class Hidden extends GoOpaque {",
		"final Userinfo? info;",
	} {
		if !strings.Contains(external, expected) {
			t.Fatalf("untranslatable struct did not bridge as GoOpaque, missing %q:\n%s", expected, external)
		}
	}
	// An empty named-parameter list is a Dart syntax error, so a fieldless
	// value class must declare a bare constructor.
	if strings.Contains(external, "({\n  });") {
		t.Fatalf("generated Dart has an empty named-parameter list:\n%s", external)
	}
	if !strings.Contains(external, "const Marker();") {
		t.Fatalf("a struct{} value class needs a parameterless constructor:\n%s", external)
	}
	var reported bool
	for _, warning := range result.Warnings {
		if strings.Contains(warning.Error(), "Userinfo") && strings.Contains(warning.Error(), "GoOpaque") {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("the downgrade to GoOpaque must be reported, got warnings %v", result.Warnings)
	}
}

func TestGenerateTimeDurationMapping(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/duration\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

import "time"

type Timing struct {
	Timeout time.Duration
	Optional *time.Duration
	Samples []time.Duration
}

func RoundTripDuration(value time.Duration) time.Duration { return value }

func RoundTripTiming(value Timing) Timing { return value }

func RoundTripLabels(value map[time.Duration]string) map[time.Duration]string { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "dart", "bridge_generated.dart")
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: goOutput, DartOutput: dartOutput,
		LibraryName: "duration", DartEntrypointClassName: "DurationBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}

	apiDart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	for _, expected := range []string{
		"final Duration timeout;", "final Duration? optional;", "final List<Duration> samples;",
		"Duration roundTripDuration", "Map<Duration, String> roundTripLabels",
	} {
		if !strings.Contains(apiDart, expected) {
			t.Fatalf("generated Dart duration API missing %q:\n%s", expected, apiDart)
		}
	}

	central := mustRead(t, dartOutput)
	for _, expected := range []string{
		"return Duration(microseconds: value);", "return value.inMicroseconds;",
		"@ffi.Int64() external int timeout;", "external ffi.Pointer<ffi.Int64> optional;",
	} {
		if !strings.Contains(central, expected) {
			t.Fatalf("central Dart bridge missing duration mapping %q", expected)
		}
	}

	goSource := mustRead(t, goOutput)
	for _, expected := range []string{
		`"time"`, "const maxDurationMicroseconds", "duration out of time.Duration range",
		"return time.Duration(raw) * time.Duration(time.Microsecond), nil",
		"return fgbDcoInt64(int64(value / time.Microsecond))",
	} {
		if !strings.Contains(goSource, expected) {
			t.Fatalf("generated Go bridge missing duration mapping %q", expected)
		}
	}
}

func TestGenerateCallModeEmitsExactlyOneDartEntrypoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/modes\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package api

func Plain(value int) int { return value }

//fgb:sync
func ExplicitSync(value int) int { return value }

//fgb:async
func ExplicitAsync(value int) int { return value }

type Worker struct{}

func NewWorker() *Worker { return &Worker{} }

//fgb:async
func (worker *Worker) Run(value int) int { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput:    filepath.Join(dir, "bridge_generated.go"),
		DartOutput:  filepath.Join(dir, "bridge_generated.dart"),
		LibraryName: "modes", DartEntrypointClassName: "ModesBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}
	dart := mustRead(t, filepath.Join(dir, "api", "api.dart"))
	for _, name := range []string{"plain", "explicitSync", "explicitAsync"} {
		if strings.Count(dart, " "+name+"(") != 1 {
			t.Fatalf("expected exactly one generated entrypoint named %s:\n%s", name, dart)
		}
	}
	if strings.Count(dart, "Future<int> run(") != 1 || strings.Contains(dart, "runSync(") || strings.Contains(dart, "runAsync(") {
		t.Fatalf("method mode/name generation is wrong:\n%s", dart)
	}
	if strings.Contains(dart, "plainSync") || strings.Contains(dart, "plainAsync") || strings.Contains(dart, "explicitSyncSync") || strings.Contains(dart, "explicitAsyncAsync") {
		t.Fatalf("mode suffix leaked into generated Dart:\n%s", dart)
	}
	central := mustRead(t, filepath.Join(dir, "bridge_generated.dart"))
	plainStart := strings.Index(central, "fgbInternalCall0")
	asyncStart := strings.Index(central, "fgbInternalCall2")
	if plainStart < 0 || !strings.Contains(central[plainStart:plainStart+280], "fgbInvokeCstSync") {
		t.Fatal("unmarked function did not use sync CST invocation")
	}
	if asyncStart < 0 || !strings.Contains(central[asyncStart:asyncStart+320], "fgbInvokeCstAsync") {
		t.Fatal("fgb(async) function did not use async CST invocation")
	}
}

func TestGenerateDirectMainPackageDoesNotDuplicateMain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/direct-main\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte("package main\n\nfunc main() {}\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: dir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := filepath.Join(dir, "bridge_generated.go")
	dartOutput := filepath.Join(dir, "bridge_generated.dart")
	_, err = Generate(api, config.Resolved{
		BaseDir: dir, GoInput: dir, GoOutput: goOutput,
		DartOutput:  dartOutput,
		LibraryName: "direct_main", DartEntrypointClassName: "MyFlutterGoBridge",
		StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustRead(t, goOutput), "func main() {}") {
		t.Fatal("generated file duplicated the user's main function")
	}
	dartSource := mustRead(t, dartOutput)
	if !strings.Contains(dartSource, "final class MyFlutterGoBridge") || strings.Contains(dartSource, "MyMyFlutterGoBridge") {
		t.Fatal("Dart entrypoint class token replacement was applied more than once")
	}
	// The input package is the module root here, so the mirrored Dart file
	// stays at the output root.
	sourcePath := filepath.Join(dir, "api.dart")
	source := mustRead(t, sourcePath)
	if !strings.Contains(source, "MyFlutterGoBridge.instance") {
		t.Fatal("per-source Dart library did not use the configured bridge class")
	}
	if strings.Contains(source, "\nexport ") {
		t.Fatal("per-source Dart library must not re-export the bridge")
	}
}

func TestGenerateDartFilesMirrorGoSourceFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/multi-source\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	firstGo := `package api

type FirstValue struct { Value int }

func First(value FirstValue) FirstValue { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "first.go"), []byte(firstGo), 0o644); err != nil {
		t.Fatal(err)
	}
	secondGo := `package api

type SecondValue struct { Value int }

func Second(value SecondValue) SecondValue { return value }
`
	if err := os.WriteFile(filepath.Join(inputDir, "second.go"), []byte(secondGo), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(dir, "dart")
	result, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir,
		GoOutput: filepath.Join(dir, "bridge_generated.go"), DartOutput: outputDir,
		LibraryName: "multi_source", DartEntrypointClassName: "MultiBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 4 {
		t.Fatalf("got %d files, want Go bridge, central, and two source Dart files", len(result.Files))
	}
	central := mustRead(t, filepath.Join(outputDir, "bridge_generated.dart"))
	if !strings.Contains(central, `import "api/first.dart";`) || !strings.Contains(central, `import "api/second.dart";`) {
		t.Fatalf("central Dart file does not mirror source files:\n%s", central)
	}
	if strings.Contains(central, `export "api/first.dart";`) || strings.Contains(central, `export "api/second.dart";`) {
		t.Fatalf("central Dart file must not re-export API files:\n%s", central)
	}
	first := mustRead(t, filepath.Join(outputDir, "api", "first.dart"))
	second := mustRead(t, filepath.Join(outputDir, "api", "second.dart"))
	if !strings.Contains(first, `import "../bridge_generated.dart";`) || !strings.Contains(first, "FirstValue first({required FirstValue value})") || !strings.Contains(first, "final class FirstValue") || strings.Contains(first, "SecondValue") {
		t.Fatalf("first.dart contains the wrong API: %s", first)
	}
	if !strings.Contains(second, "SecondValue second({required SecondValue value})") || !strings.Contains(second, "final class SecondValue") || strings.Contains(second, "FirstValue") {
		t.Fatalf("second.dart contains the wrong API: %s", second)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
