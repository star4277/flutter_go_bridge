package generator

import (
	"go/importer"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgemodel "github.com/star4277/flutter_go_bridge/internal/model"
	bridgeparser "github.com/star4277/flutter_go_bridge/internal/parser"
	"golang.org/x/tools/go/packages"
)

func TestAtomicCollectionClassificationAndSyntheticNames(t *testing.T) {
	atomicPackage, err := importer.Default().Import("sync/atomic")
	if err != nil {
		t.Fatal(err)
	}
	atomicInt64 := atomicPackage.Scope().Lookup("Int64").Type().(*types.Named)
	inputPackage := types.NewPackage("example.com/fixture/api", "api")
	b := &builder{api: &bridgemodel.API{Package: &packages.Package{Types: inputPackage}}}

	tests := []struct {
		name string
		typ  types.Type
		want string
	}{
		{"basic", types.Typ[types.String], "String"},
		{"pointer", types.NewPointer(atomicInt64), "AtomicInt64"},
		{"slice", types.NewSlice(atomicInt64), "AtomicInt64Slice"},
		{"map", types.NewMap(types.Typ[types.String], atomicInt64), "StringAtomicInt64Map"},
		{"fallback", types.NewSignatureType(nil, nil, nil, nil, nil, false), "AtomicOpaque"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := b.syntheticOpaqueName(test.typ); got != test.want {
				t.Fatalf("syntheticOpaqueName(%s) = %q, want %q", test.typ, got, test.want)
			}
		})
	}

	if got := b.atomicCollectionShape(types.NewSlice(atomicInt64)); got != atomicCollectionOpaque {
		t.Fatalf("atomic slice class = %v", got)
	}
	if got := b.atomicCollectionShape(types.NewMap(types.Typ[types.String], atomicInt64)); got != atomicCollectionOpaque {
		t.Fatalf("atomic map class = %v", got)
	}
	if !atomicValueByCopy(types.NewMap(types.Typ[types.String], atomicInt64), map[types.Type]bool{}) {
		t.Fatal("map values containing atomic state must be classified as by-value copies")
	}
	if got := b.atomicCollectionShape(types.NewArray(atomicInt64, 2)); got != atomicCollectionReject {
		t.Fatalf("atomic array class = %v", got)
	}
	if got := b.atomicCollectionShape(types.NewSlice(types.NewPointer(atomicInt64))); got != atomicCollectionNone {
		t.Fatalf("atomic pointer slice class = %v", got)
	}
	if atomicValueByCopy(types.NewPointer(atomicInt64), map[types.Type]bool{}) {
		t.Fatal("a pointer to atomic state must not count as a by-value copy")
	}
	if atomicValueByCopy(types.Typ[types.Int], map[types.Type]bool{}) {
		t.Fatal("a basic type must not count as atomic state")
	}
}

func TestCanReferenceImplementationPackageRespectsGoImportRules(t *testing.T) {
	b := &builder{unit: &unit{PackagePath: "example.com/owner/app/bridge"}}
	packageFor := func(path, name string) *packages.Package {
		return &packages.Package{PkgPath: path, Name: name, Types: types.NewPackage(path, name)}
	}
	tests := []struct {
		name string
		pkg  *packages.Package
		want bool
	}{
		{name: "nil", pkg: nil},
		{name: "missing types", pkg: &packages.Package{PkgPath: "example.com/plain", Name: "plain"}},
		{name: "main package", pkg: packageFor("example.com/tool", "main")},
		{name: "ordinary dependency", pkg: packageFor("example.com/dependency", "dependency"), want: true},
		{name: "standard internal root", pkg: packageFor("internal", "internal")},
		{name: "relative internal tree", pkg: packageFor("internal/helper", "helper")},
		{name: "owner internal root", pkg: packageFor("example.com/owner/internal", "internal"), want: true},
		{name: "owner nested internal", pkg: packageFor("example.com/owner/internal/models", "models"), want: true},
		{name: "foreign internal root", pkg: packageFor("example.com/foreign/internal", "internal")},
		{name: "foreign nested internal", pkg: packageFor("example.com/foreign/internal/models", "models")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := b.canReferenceImplementationPackage(test.pkg); got != test.want {
				t.Fatalf("canReferenceImplementationPackage() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAtomicStructRecursionAndCycleGuards(t *testing.T) {
	atomicPackage, err := importer.Default().Import("sync/atomic")
	if err != nil {
		t.Fatal(err)
	}
	atomicInt64 := atomicPackage.Scope().Lookup("Int64").Type().(*types.Named)
	inputPackage := types.NewPackage("example.com/fixture/api", "api")

	innerName := types.NewTypeName(0, inputPackage, "Inner", nil)
	inner := types.NewNamed(innerName, types.NewStruct(
		[]*types.Var{types.NewField(0, inputPackage, "Hits", atomicInt64, false)},
		[]string{""},
	), nil)
	outerName := types.NewTypeName(0, inputPackage, "Outer", nil)
	outer := types.NewNamed(outerName, types.NewStruct(
		[]*types.Var{types.NewField(0, inputPackage, "Inner", inner, false)},
		[]string{""},
	), nil)
	if !atomicValueByCopy(outer, map[types.Type]bool{}) {
		t.Fatal("nested value struct should report atomic state")
	}
	basicName := types.NewTypeName(0, inputPackage, "Count", nil)
	basicNamed := types.NewNamed(basicName, types.Typ[types.Int], nil)
	if atomicValueByCopy(basicNamed, map[types.Type]bool{}) {
		t.Fatal("named basic type cannot contain atomic struct state")
	}

	nodeName := types.NewTypeName(0, inputPackage, "Node", nil)
	node := types.NewNamed(nodeName, nil, nil)
	node.SetUnderlying(types.NewStruct(
		[]*types.Var{types.NewField(0, inputPackage, "Next", types.NewPointer(node), false)},
		[]string{""},
	))
	if atomicValueByCopy(node, map[types.Type]bool{}) {
		t.Fatal("a recursive pointer-only struct must not be classified as atomic by value")
	}
	seen := map[types.Type]bool{node: true}
	if atomicValueByCopy(node, seen) {
		t.Fatal("cycle guard should terminate with no duplicate atomic classification")
	}
	if got := atomicCollectionShape(node, map[types.Type]bool{node: true}); got != atomicCollectionNone {
		t.Fatalf("collection cycle guard = %v", got)
	}
}

// TestDartMethodSignatureAsyncPrefix covers the async branch of
// dartMethodSignature via a minimal call model.
func TestDartMethodSignatureAsyncPrefix(t *testing.T) {
	async := &callModel{Mode: bridgemodel.CallModeAsync, Results: []*resultModel{{DartName: "v", Type: &wireType{Kind: kindString, DartType: "String"}}}}
	sig := dartMethodSignature(async)
	if !strings.HasPrefix(sig, "async ") {
		t.Fatalf("async signature should carry the async prefix, got %q", sig)
	}
	sync := &callModel{Mode: bridgemodel.CallModeSync, Results: []*resultModel{{Type: &wireType{Kind: kindString, DartType: "String"}}}}
	if !strings.HasPrefix(dartMethodSignature(sync), "String") {
		t.Fatalf("sync signature should start with the result type, got %q", dartMethodSignature(sync))
	}
}

// TestAppendUniqueDeduplicates exercises both the append and the dedup branch
// of the package-level appendUnique helper.
func TestAppendUniqueDeduplicates(t *testing.T) {
	a := &interfaceModel{GoName: "A"}
	b := &interfaceModel{GoName: "B"}
	list := appendUnique(nil, a)
	list = appendUnique(list, a)
	list = appendUnique(list, b)
	if len(list) != 2 {
		t.Fatalf("duplicate should be dropped, got %d entries", len(list))
	}
}

// TestValidMapKeyCoverage pins the allowed and rejected map-key kinds.
func TestValidMapKeyCoverage(t *testing.T) {
	allowed := []typeKind{kindBool, kindString, kindSigned, kindUnsigned, kindFloat, kindDuration, kindIPPrefix, kindURL, kindNamed}
	for _, kind := range allowed {
		if !validMapKey(&wireType{Kind: kind}) {
			t.Fatalf("map key kind %s should be allowed", kind)
		}
	}
	rejected := []typeKind{kindMap, kindArray, kindInterface, kindCallback, kindStreamSink, kindBigInt, kindAny}
	for _, kind := range rejected {
		if validMapKey(&wireType{Kind: kind}) {
			t.Fatalf("map key kind %s should be rejected", kind)
		}
	}
}

// TestParseFieldTagEmptyDefaultValue drives parseFieldTag's empty defaultValue
// error branch.
func TestParseFieldTagEmptyDefaultValue(t *testing.T) {
	if _, err := parseFieldTag("defaultValue:"); err == nil || !strings.Contains(err.Error(), "needs a Dart expression") {
		t.Fatalf("empty defaultValue should be rejected, got %v", err)
	}
}

// TestParseFieldTagLeadingComma drives parseFieldTag's empty-token branch.
func TestParseFieldTagLeadingComma(t *testing.T) {
	opts, err := parseFieldTag(",non-final")
	if err != nil {
		t.Fatalf("leading comma should be tolerated, got %v", err)
	}
	if !opts.NonFinal {
		t.Fatal("non-final option after a leading comma must be parsed")
	}
}

// TestNewSimpleTypeCached exercises newSimpleType's cached return via two
// identical simple types.
func TestNewSimpleTypeCached(t *testing.T) {
	b := &builder{
		typeCache: map[types.Type]*wireType{},
		unit:      &unit{},
	}
	strType := types.Typ[types.String]
	first := b.newSimpleType(strType, kindString, "String")
	second := b.newSimpleType(strType, kindString, "String")
	if first == nil || first != second {
		t.Fatal("newSimpleType should return the cached node on the second call")
	}
}

// TestNewTypeCached exercises the cached branch of the newType helper.
func TestNewTypeCached(t *testing.T) {
	b := &builder{
		typeCache: map[types.Type]*wireType{},
		unit:      &unit{},
	}
	stringType := types.Typ[types.String]
	first := b.newType(stringType, kindString, "String")
	second := b.newType(stringType, kindString, "String")
	if first == nil || second == nil {
		t.Fatal("newType must not return nil")
	}
	if first != second {
		t.Fatal("a second newType call for the same type must return the cached node")
	}
}

// TestGenerateMethodDartNameCollisionRenames triggers disambiguateMethod: two
// methods on one receiver whose Dart names collide after sanitization force a
// numeric suffix and a warning.
func TestGenerateMethodDartNameCollisionRenames(t *testing.T) {
	_, _, _, warnings, err := generateFixture(t, `package api

//fgb:rename = "Go"
func (c *Counter) Alpha() int { return 0 }

//fgb:rename = "Go"
func (c *Counter) Beta() int { return 0 }

//fgb:opaque
type Counter struct {
	total int
}
`)
	if err != nil {
		t.Fatalf("method rename collision should not error: %v", err)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "renamed") && strings.Contains(warning.Error(), "Beta") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a disambiguation warning for Beta, got %v", warnings)
	}
}

// TestGenerateMethodShadowingIncompatibleSignature triggers checkMethodOverrides:
// a subclass shadowing a promoted method with a different Dart signature is a
// hard error because Dart cannot express it.
func TestGenerateMethodShadowingIncompatibleSignature(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Base struct{ V int }

func (b Base) M() int { return 0 }

type Child struct {
	Base
}

func (c Child) M(extra int) int { return 0 }

func Use() Child { return Child{} }
`)
	if err == nil || !strings.Contains(err.Error(), "shadows") {
		t.Fatalf("incompatible method shadowing must be rejected, got %v", err)
	}
}

// TestGenerateSubclassOwnMethodNotShadowing drives checkMethodOverrides's
// non-shadowing continue branch.
func TestGenerateSubclassOwnMethodNotShadowing(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Animal struct {
	Name string
}

type Dog struct {
	Animal
	Breed string
}

func (d Dog) Bark() int { return len(d.Breed) }

func MakeDog() Dog { return Dog{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "class Dog extends Animal {") {
		t.Fatalf("subclass should extend its embedded superclass:\n%s", apiDart)
	}
}

// TestGenerateInterfaceIgnoredMethod covers mapInterface's directive.Ignore
// skip branch: an interface method marked //fgb:ignore must not appear in the
// Dart interface declaration.
func TestGenerateInterfaceIgnoredMethod(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Loader interface {
	//fgb:ignore
	Hidden() int
	Load() int
}

type Worker struct{ N int }

func (w Worker) Hidden() int { return 0 }
func (w Worker) Load() int   { return 0 }

func Use(l Loader) int { return l.Load() }
`)
	if err != nil {
		t.Fatal(err)
	}
	interfaceStart := strings.Index(apiDart, "interface class Loader")
	interfaceEnd := strings.Index(apiDart, "final class Worker")
	if interfaceStart < 0 || interfaceEnd < 0 || interfaceEnd < interfaceStart {
		t.Fatalf("could not delimit the Loader interface declaration:\n%s", apiDart)
	}
	decl := apiDart[interfaceStart:interfaceEnd]
	if strings.Contains(decl, "hidden") {
		t.Fatalf("ignored interface method leaked into the interface declaration:\n%s", decl)
	}
	if !strings.Contains(decl, "int load();") {
		t.Fatalf("kept interface method missing:\n%s", decl)
	}
}

// TestGenerateInterfaceUnexportedMethodRejected triggers the unexported-method
// rejection inside mapInterface.
func TestGenerateInterfaceUnexportedMethodRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Shape interface {
	area() int
}

type Circle struct{ R int }

func (c Circle) area() int { return 0 }

func Use(s Shape) int { return 0 }
`)
	if err == nil || !strings.Contains(err.Error(), "cannot be bridged") {
		t.Fatalf("an interface with an unexported method must be rejected, got %v", err)
	}
}

// TestGenerateAutoOpaqueBlockingBasicTypes covers fieldTranslateBlocker's
// unsupported-basic branch (complex128) and the map-key blocker path.
func TestGenerateAutoOpaqueBlockingBasicTypes(t *testing.T) {
	_, _, _, warnings, err := generateFixture(t, `package api

type ComplexHolder struct {
	Z complex128
}

type MapHolder struct {
	Lookup map[complex128]string
}

func TakeComplex(c *ComplexHolder) {}
func TakeMap(m *MapHolder) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	var unsupported, mapKey bool
	for _, warning := range warnings {
		msg := warning.Error()
		if strings.Contains(msg, "unsupported basic type complex128") {
			unsupported = true
		}
		if strings.Contains(msg, "has unsupported basic type") && strings.Contains(msg, "MapHolder") {
			mapKey = true
		}
	}
	if !unsupported {
		t.Fatalf("complex128 field should warn about the unsupported basic type, got %v", warnings)
	}
	if !mapKey {
		t.Fatalf("map with complex128 key should warn via the key blocker, got %v", warnings)
	}
}

// TestGenerateAutoOpaqueBlockingNestedPointer covers the nested-pointer branch
// of fieldTranslateBlocker.
func TestGenerateAutoOpaqueBlockingNestedPointer(t *testing.T) {
	_, _, _, warnings, err := generateFixture(t, `package api

type Deep struct {
	Ref **int
}

func TakeDeep(d *Deep) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "nested pointer type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("nested pointer field should warn, got %v", warnings)
	}
}

// TestGenerateAutoOpaqueBlockingAnonymousInterface covers the non-empty
// anonymous interface branch of fieldTranslateBlocker.
func TestGenerateAutoOpaqueBlockingAnonymousInterface(t *testing.T) {
	_, _, _, warnings, err := generateFixture(t, `package api

type Caller struct {
	C interface{ Invoke() }
}

func TakeCaller(c *Caller) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "non-empty interface type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a non-empty anonymous interface field should warn, got %v", warnings)
	}
}

// TestGenerateAutoOpaqueBlockingFunction covers the Signature (function type)
// branch of fieldTranslateBlocker.
func TestGenerateAutoOpaqueBlockingFunction(t *testing.T) {
	_, _, _, warnings, err := generateFixture(t, `package api

type Handler struct {
	On func(int) int
}

func TakeHandler(h *Handler) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "has a function type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a func field should warn, got %v", warnings)
	}
}

// TestGenerateNamedEmptyInterfaceField drives fieldTranslateBlocker's named
// empty-interface return.
func TestGenerateNamedEmptyInterfaceField(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Any interface{}

type Box struct {
	Name string
	V    Any
}

func Take(b Box) string { return b.Name }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "Box extends GoOpaque") {
		t.Fatalf("a named empty interface field must not force the struct opaque:\n%s", apiDart)
	}
}

// TestGenerateArrayField drives fieldTranslateBlocker's array traversal.
func TestGenerateArrayField(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Grid struct {
	Cells [4]int
}

func Take(g Grid) int { return g.Cells[0] }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "Grid extends GoOpaque") {
		t.Fatalf("an array field must not force the struct opaque:\n%s", apiDart)
	}
}

// TestGenerateInputNamedInterfaceFieldBridgesValue covers the input-package
// named-interface branch of fieldTranslateBlocker.
func TestGenerateInputNamedInterfaceFieldBridgesValue(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Reader interface{ Read() int }

type Holder struct {
	R Reader
}

type Box struct{ Data string }

func (b Box) Read() int { return 0 }
func TakeHolder(h Holder) string { return "" }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "Holder extends GoOpaque") {
		t.Fatalf("a named-interface field must not force the struct opaque:\n%s", apiDart)
	}
}

// TestGenerateHoldGoOpaqueByValueWarns covers the hold-GoOpaque-struct-by-value
// branch of fieldTranslateBlocker.
func TestGenerateHoldGoOpaqueByValueWarns(t *testing.T) {
	_, _, _, warnings, err := generateFixture(t, `package api

//fgb:opaque
type Session struct{ state int }

type Wrapper struct {
	Session Session
}

func TakeWrapper(w *Wrapper) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "holds GoOpaque struct Session by value") {
			found = true
		}
	}
	if !found {
		t.Fatalf("holding an opaque struct by value should warn, got %v", warnings)
	}
}

// TestGenerateMapEmbeddedPointerRejected drives mapEmbedded's pointer-embed
// early return.
func TestGenerateMapEmbeddedPointerRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Base struct{ V int }

type Derived struct {
	*Base
}

func Use(d Derived) int { return 0 }
`)
	if err == nil || !strings.Contains(err.Error(), "embeds a pointer type") {
		t.Fatalf("pointer embedding must be rejected, got %v", err)
	}
}

// TestGenerateEmbeddedOpaqueByValueFallsBack verifies that a value struct
// embedding an explicit-opaque struct by value degrades to a GoOpaque handle.
func TestGenerateEmbeddedOpaqueByValueFallsBack(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

//fgb:opaque
type Base struct{ hidden int }

type Derived struct {
	Base
}

func (d Derived) Use() int { return 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Derived extends GoOpaque {") {
		t.Fatalf("embedding an opaque struct by value should degrade Derived to GoOpaque:\n%s", apiDart)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Error(), "Base") && strings.Contains(warning.Error(), "by value") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the opaque-by-value field should warn, got %v", warnings)
	}
}

// TestGenerateEmbeddedNonStructStaysField covers mapEmbedded's non-struct
// early return.
func TestGenerateEmbeddedNonStructStaysField(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type ID int64

type Record struct {
	ID
	Name string
}

func Use(r Record) string { return r.Name }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "class Record extends ID") {
		t.Fatalf("embedded non-struct should not become a superclass:\n%s", apiDart)
	}
}

// TestGenerateEmbeddedTimeStaysField drives mapEmbedded's dedicated-mapping
// early return.
func TestGenerateEmbeddedTimeStaysField(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

import "time"

type Event struct {
	time.Time
	Label string
}

func Use(e Event) string { return e.Label }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "extends DateTime") {
		t.Fatalf("embedding time.Time must not create a Dart superclass:\n%s", apiDart)
	}
}

// TestGenerateNestedPointerParameterRejected drives mapType's nested-pointer
// rejection at parameter level.
func TestGenerateNestedPointerParameterRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Use(pp **int) {}
`)
	requireError(t, err, "nested pointers")
}

// TestGenerateAnonymousEmptyInterfaceParameter drives mapType's anonymous
// empty-interface branch (kindAny).
func TestGenerateAnonymousEmptyInterfaceParameter(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

func Echo(value interface{}) interface{} { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Object? echo({required Object? value})") {
		t.Fatalf("anonymous empty interface should map to Object?:\n%s", apiDart)
	}
}

// TestGenerateAnonymousNonEmptyInterfaceRejected drives mapType's rejection of
// an unnamed non-empty interface parameter.
func TestGenerateAnonymousNonEmptyInterfaceRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Use(c interface{ Invoke() int }) int { return 0 }
`)
	requireError(t, err, "unnamed non-empty interface")
}

// TestGenerateNamedEmptyInterfaceParameter drives mapType's named empty
// interface (kindAny) branch.
func TestGenerateNamedEmptyInterfaceParameter(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Any interface{}

func Store(v Any) Any { return v }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "Object?") {
		t.Fatalf("a named empty interface should bridge as Object?:\n%s", apiDart)
	}
}

// TestGenerateInvalidMapKeyRejected drives mapType's map-key validity guard.
func TestGenerateInvalidMapKeyRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Use(m map[[2]int]string) {}
`)
	requireError(t, err, "not supported by StandardMessageCodec")
}

// TestGenerateNamedFunctionTypeUnderlyingRejected drives mapNamed's underlying
// mapping error.
func TestGenerateNamedFunctionTypeUnderlyingRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Callback func() int

func Use(f Callback) int { return 0 }
`)
	if err == nil {
		t.Fatalf("a named function type parameter must be rejected, got nil")
	}
}

// TestGenerateRecursiveValueStructStaysValue drives classifyStruct's
// classInProgress cycle short-circuit.
func TestGenerateRecursiveValueStructStaysValue(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Node struct {
	Name string
	Tags []Node
}

func Take(n Node) string { return n.Name }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "class Node extends GoOpaque {") {
		t.Fatalf("a recursive value struct should stay a value class:\n%s", apiDart)
	}
}

// TestGenerateInvalidFGBNonKeywordFieldTag drives classifyStruct's tag parse
// error branch by placing an unknown fgb option directly on a struct field.
func TestGenerateInvalidFGBNonKeywordFieldTag(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Item struct {
	Kept string
	Bad  string `+"`fgb:\"not-an-option\"`"+`
}
func Make(i Item) Item { return i }
`)
	requireError(t, err, "unknown fgb field tag option")
}

// TestGenerateNamedTypeReusedStaysStable covers mapNamed's cached-return and
// dartNameFor's cached-return by referencing the same named type in several
// signatures.
func TestGenerateNamedTypeReusedStaysStable(t *testing.T) {
	apiDart, _, goSource, _, err := generateFixture(t, `package api

type ID string

func First(id ID) ID { return id }
func Second(id ID) ID { return id }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "extension type const Id(String value)") {
		t.Fatalf("named type should be emitted once as a Dart extension:\n%s", apiDart)
	}
	if !strings.Contains(goSource, "ID") {
		t.Fatalf("the named Go type should appear in the bridge:\n%s", goSource)
	}
}

// TestGenerateGenericFunctionRejected drives mapCallable's generic-function
// guard.
func TestGenerateGenericFunctionRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Generic[T any](x T) T { return x }
`)
	requireError(t, err, "generic functions are not supported")
}

// TestGenerateVariadicFunctionRejected drives mapCallable's variadic-function
// guard.
func TestGenerateVariadicFunctionRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Sum(nums ...int) int { return 0 }
`)
	requireError(t, err, "variadic functions are not supported")
}

// TestGenerateFloat32 drives mapBasic's Float32 branch.
func TestGenerateFloat32(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

func Scale(value float32) float32 { return value }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "double scale({required double value})") {
		t.Fatalf("float32 should map to Dart double:\n%s", apiDart)
	}
}

// TestGenerateComplexParameterRejected drives mapBasic's unsupported-basic
// branch through a function parameter.
func TestGenerateComplexParameterRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

func Use(c complex128) {}
`)
	requireError(t, err, "unsupported basic type")
}

// TestGenerateChannelWithChannelElementRejected drives mapChannelStream's
// element-mapping error return.
func TestGenerateChannelWithChannelElementRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Relay(out chan<- chan int) error { return nil }
`)
	requireError(t, err, "stream element")
}

// TestGenerateDuplicateWireFieldAfterPromotion drives mapStruct's duplicate
// wire field guard when a shadowed field collides with a promoted super field.
func TestGenerateDuplicateWireFieldAfterPromotion(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Base struct{ X int }

type Sub struct {
	Base
	X string
}

func Use(s Sub) Sub { return s }
`)
	requireError(t, err, "duplicate wire field")
}

// TestGenerateDuplicateWireFieldViaJSON drives mapStruct's duplicate wire
// field guard when two fields share the same json name.
func TestGenerateDuplicateWireFieldViaJSON(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

type Dupe struct {
	A string `+"`json:\"x\"`"+`
	B string `+"`json:\"x\"`"+`
}

func Use(d Dupe) Dupe { return d }
`)
	requireError(t, err, "duplicate wire field")
}

func TestGenerateAtomicInsideCollectionFieldUsesSyntheticOpaque(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

import "sync/atomic"

type Recorder struct {
	Flags []atomic.Int64
}

func Make() *Recorder { return &Recorder{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final AtomicInt64Slice? flags;") || len(warnings) == 0 {
		t.Fatalf("atomic collection field should use synthetic opaque: %s warnings=%v", apiDart, warnings)
	}
}

func TestGenerateValueStructWithAtomicFieldFallsBackToOpaque(t *testing.T) {
	apiDart, _, _, warnings, err := generateFixture(t, `package api

import "sync/atomic"

type Stats struct {
	Hits atomic.Int64
}

type Meter struct {
	S Stats
}

func Make() *Meter { return &Meter{} }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Meter extends GoOpaque") || len(warnings) == 0 {
		t.Fatalf("nested atomic value struct should fall back to opaque: %s warnings=%v", apiDart, warnings)
	}
}

// TestGenerateStructFieldWithStreamSinkResultRejected drives containsStreamSink's
// struct-field traversal.
func TestGenerateStructFieldWithStreamSinkResultRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

type Watcher struct {
	Name   string
	Events fgb.StreamSink[string]
}

//fgb:async
func Make() (Watcher, error) { return Watcher{}, nil }
`)
	requireError(t, err, "cannot be returned to Dart")
}

// TestGenerateStructRepeatedCoversMapStructCached drives mapStruct's cached
// return by passing the same struct twice.
func TestGenerateStructRepeatedCoversMapStructCached(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Pair struct {
	Left  string
	Right int
}

func Swap(p Pair, q Pair) Pair { return p }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "class Pair {") {
		t.Fatalf("struct should be emitted as a Dart value class:\n%s", apiDart)
	}
}

// TestGenerateTwoOpaqueParamsCoversMapOpaqueCached drives mapOpaque's cached
// return by passing the same opaque pointer type twice.
func TestGenerateTwoOpaqueParamsCoversMapOpaqueCached(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

//fgb:opaque
type Handle struct{ state int }

func Transfer(a *Handle, b *Handle) bool { return a == b }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Handle extends GoOpaque {") {
		t.Fatalf("opaque handle missing:\n%s", apiDart)
	}
}

// TestGenerateRepeatedByteParamsCoversNewSimpleCached drives newSimpleType's
// cached branch for byte slices.
func TestGenerateRepeatedByteParamsCoversNewSimpleCached(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

func Concat(a []byte, b []byte) []byte { return append(a, b...) }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSource, "[]byte") {
		t.Fatalf("byte params missing from generated bridge:\n%s", goSource)
	}
}

// TestGenerateRepeatedTimeParamsCoversNewSimpleCached covers time mapping.
func TestGenerateRepeatedTimeParamsCoversNewSimpleCached(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

import "time"

func Compare(a time.Time, b time.Time, c time.Duration, d time.Duration) bool { return a.Before(b) }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "DateTime") || !strings.Contains(apiDart, "Duration") {
		t.Fatalf("time types missing from generated Dart:\n%s", apiDart)
	}
}

// TestGenerateVariadicFuncTypeRejected drives mapCallback's variadic guard.
func TestGenerateVariadicFuncTypeRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Use(f func(args ...int)) int { return 0 }
`)
	requireError(t, err, "variadic function types")
}

// TestGenerateCallbackParameterMapError drives mapCallback's callback-
// parameter mapping error return.
func TestGenerateCallbackParameterMapError(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Use(cb func(m map[[2]int]string)) int { return 0 }
`)
	requireError(t, err, "callback parameter")
}

// TestGenerateCallbackResultMapError drives mapCallback's callback-result
// mapping error return.
func TestGenerateCallbackResultMapError(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Use(cb func() map[[2]int]string) int { return 0 }
`)
	requireError(t, err, "callback result")
}

// TestGenerateRepeatedSinkParamsCoversStreamCache drives mapStreamSink's and
// mapChannelStream's stream generation for repeated sink/channel types.
func TestGenerateRepeatedSinkParamsCoversStreamCache(t *testing.T) {
	_, _, goSource, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

//fgb:async
func Merge(a fgb.StreamSink[string], b fgb.StreamSink[string], out chan<- int, out2 chan<- int) error { return nil }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSource, "StreamSink") {
		t.Fatalf("sink handling missing from generated Go:\n%s", goSource)
	}
}

// TestGenerateStreamSinkCannotHoldStreamSink drives mapStreamSink's stream-of-
// sinks guard.
func TestGenerateStreamSinkCannotHoldStreamSink(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

//fgb:async
func Watch(sink fgb.StreamSink[fgb.StreamSink[string]]) error { return nil }
`)
	requireError(t, err, "stream of stream sinks")
}

// TestGenerateInterfaceParameterRepeated drives mapInterface's cached return.
func TestGenerateInterfaceParameterRepeated(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Shape interface{ Area() int }

type Square struct{ Side int }

func (s Square) Area() int { return s.Side }

func First(s Shape) int { return 0 }
func Second(s Shape) int { return 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "abstract interface class Shape {") {
		t.Fatalf("interface missing from generated Dart:\n%s", apiDart)
	}
}

// TestGenerateCollectImplementorsSkipsNonStruct drives collectImplementors's
// non-struct candidate skip.
func TestGenerateCollectImplementorsSkipsNonStruct(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Label string

type Shape interface{ Area() int }

type Square struct{ Side int }

func (s Square) Area() int { return s.Side }

func Use(s Shape) int { return 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "class Square implements Shape {") {
		t.Fatalf("interface implementor missing:\n%s", apiDart)
	}
}

// TestGenerateCollectImplementorsSkipsNonImplementor drives collectImplementors's
// non-implementing struct skip.
func TestGenerateCollectImplementorsSkipsNonImplementor(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Shape interface{ Area() int }

type Square struct{ Side int }

func (s Square) Area() int { return s.Side }

type Circle struct{ R int }

func Use(s Shape) int { return 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "class Square implements Shape {") {
		t.Fatalf("interface implementor missing:\n%s", apiDart)
	}
}

// TestGenerateOpaqueInterfaceImplementor drives collectImplementors's opaque
// branch: an opaque struct satisfying the interface is tagged via a pointer.
func TestGenerateOpaqueInterfaceImplementor(t *testing.T) {
	apiDart, central, _, _, err := generateFixture(t, `package api

type Shape interface{ Area() int }

//fgb:opaque
type Handle struct{ n int }

func (h *Handle) Area() int { return 0 }

func Use(s Shape) int { return 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apiDart, "final class Handle extends GoOpaque implements Shape {") {
		t.Fatalf("opaque implementation missing:\n%s", apiDart)
	}
	if !strings.Contains(central, "value is Handle") {
		t.Fatalf("the encoder should recognize the opaque implementor:\n%s", central)
	}
}

// TestGenerateExternalNamedInterfaceWithoutNamedImplementationUsesFallback
// verifies that an open dependency interface still has an opaque union member
// for implementations generated Go code cannot name.
func TestGenerateExternalNamedInterfaceWithoutNamedImplementationUsesFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.24\n\nrequire example.com/thirdparty v0.0.0\n\nreplace example.com/thirdparty => ./thirdparty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(dir, "thirdparty")
	if err := os.MkdirAll(filepath.Join(externalDir, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "go.mod"), []byte("module example.com/thirdparty\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "contracts", "contracts.go"), []byte("package contracts\n\ntype Service interface {\n\tRun() int\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte("package api\n\nimport \"example.com/thirdparty/contracts\"\n\nfunc Use(s contracts.Service) int { return 0 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: filepath.Join(dir, "bridge_generated.go"),
		DartOutput: filepath.Join(dir, "dart", "bridge_generated.dart"), LibraryName: "fixture",
		DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	external := mustRead(t, filepath.Join(dir, "dart", "_generated.dart"))
	if !strings.Contains(external, "final class ServiceOpaque extends GoOpaque implements Service {") {
		t.Fatalf("external interface is missing its open-world opaque fallback:\n%s", external)
	}
	goSource := mustRead(t, filepath.Join(dir, "bridge_generated.go"))
	if !strings.Contains(goSource, "case fgbext0.Service:") {
		t.Fatalf("external interface fallback must be the final Go type-switch case:\n%s", goSource)
	}
}

// TestGenerateExternalNamedInterfaceField verifies that a struct carrying a
// dependency interface remains a value type when a reachable implementation
// can populate the interface serialization union.
func TestGenerateExternalNamedInterfaceField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.24\n\nrequire example.com/thirdparty v0.0.0\n\nreplace example.com/thirdparty => ./thirdparty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(dir, "thirdparty")
	if err := os.MkdirAll(filepath.Join(externalDir, "shapes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "go.mod"), []byte("module example.com/thirdparty\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "shapes", "shapes.go"), []byte("package shapes\n\ntype Drawable interface {\n\tDraw() int\n}\n\ntype Line struct {\n\tLength int\n}\n\nfunc (Line) Draw() int { return 0 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte("package api\n\nimport \"example.com/thirdparty/shapes\"\n\ntype Draft struct {\n\tTarget shapes.Drawable\n\tName   string\n}\n\nfunc New() *Draft { return &Draft{} }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: filepath.Join(dir, "bridge_generated.go"),
		DartOutput: filepath.Join(dir, "dart", "bridge_generated.dart"), LibraryName: "fixture",
		DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning.Error(), "external interface") {
			t.Fatalf("a supported external interface field must not force an opaque fallback: %v", result.Warnings)
		}
	}
	dart := mustRead(t, filepath.Join(dir, "dart", "api", "api.dart"))
	if !strings.Contains(dart, "final Drawable target;") || !strings.Contains(dart, "final class Draft") {
		t.Fatalf("external interface field did not remain serializable:\n%s", dart)
	}
}

// TestGenerateExternalAtomicWithoutMatchingMethods drives atomicValueType's
// negative branches: types in a package named `atomic` whose method sets do
// not match the Load/Store contract are not recognized as atomic wrappers.
func TestGenerateExternalAtomicWithoutMatchingMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.24\n\nrequire example.com/thirdparty v0.0.0\n\nreplace example.com/thirdparty => ./thirdparty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(dir, "thirdparty")
	if err := os.MkdirAll(filepath.Join(externalDir, "atomic"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "go.mod"), []byte("module example.com/thirdparty\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	custom := `package atomic

type Counter struct{ n int }

func (c *Counter) Inc() int { c.n++; return c.n }

type Flag struct{ b bool }

func (f *Flag) Read() bool { return f.b }
func (f *Flag) Set(v bool, extra int) bool { f.b = v; return f.b }

type Box struct{ v int }

func (b *Box) Load() int { return b.v }
func (b *Box) Store(a, extra int) { b.v = a }

type Odd struct{ p *int }

func (o *Odd) Pick() *int { return o.p }
func (o *Odd) Put(p *int) { o.p = p }
`
	if err := os.WriteFile(filepath.Join(externalDir, "atomic", "atomic.go"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "api.go"), []byte("package api\n\nimport \"example.com/thirdparty/atomic\"\n\nfunc Make() *atomic.Counter { return &atomic.Counter{} }\nfunc Flip() *atomic.Flag { return &atomic.Flag{} }\nfunc TakeBox(b *atomic.Box) {}\nfunc TakeOdd(o *atomic.Odd) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := bridgeparser.Parse(bridgeparser.Options{Input: inputDir, BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(api, config.Resolved{
		BaseDir: dir, GoInput: inputDir, GoOutput: filepath.Join(dir, "bridge_generated.go"),
		DartOutput: filepath.Join(dir, "dart", "bridge_generated.dart"), LibraryName: "fixture",
		DartEntrypointClassName: "FixtureBridge", StopOnError: true,
	}); err != nil {
		t.Fatal(err)
	}
	externalDart := mustRead(t, filepath.Join(dir, "dart", "_generated.dart"))
	if strings.Contains(externalDart, "class Counter") && strings.Contains(externalDart, "Atomic") {
		t.Fatalf("non-matching atomic types should not map with atomic semantics:\n%s", externalDart)
	}
}
