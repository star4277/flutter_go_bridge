package generator

import (
	"go/types"

	bridgemodel "github.com/star4277/flutter_go_bridge/internal/model"
)

type typeKind string

const (
	kindBool        typeKind = "bool"
	kindString      typeKind = "string"
	kindSigned      typeKind = "signed"
	kindUnsigned    typeKind = "unsigned"
	kindFloat       typeKind = "float"
	kindError       typeKind = "error"
	kindBigInt      typeKind = "big_int"
	kindTime        typeKind = "time"
	kindInternetIP  typeKind = "internet_ip"
	kindIPPrefix    typeKind = "ip_prefix"
	kindURL         typeKind = "url"
	kindUUID        typeKind = "uuid"
	kindDecimal     typeKind = "decimal"
	kindDuration    typeKind = "duration"
	kindAny         typeKind = "any"
	kindPointer     typeKind = "pointer"
	kindSlice       typeKind = "slice"
	kindArray       typeKind = "array"
	kindMap         typeKind = "map"
	kindStruct      typeKind = "struct"
	kindOpaque      typeKind = "opaque"
	kindDartOpaque  typeKind = "dart_opaque"
	kindCallback    typeKind = "callback"
	kindStreamSink  typeKind = "stream_sink"
	kindInterface   typeKind = "interface"
	kindNamed       typeKind = "named"
	kindAtomic      typeKind = "atomic"
	kindBytes       typeKind = "bytes"
	kindInt32List   typeKind = "int32_list"
	kindInt64List   typeKind = "int64_list"
	kindFloat64List typeKind = "float64_list"
)

type unit struct {
	PackagePath string
	PackageName string
	InputDir    string
	// MirrorRoot anchors the Dart output tree: source paths are mirrored
	// relative to it, so the Dart layout matches the Go package layout.
	MirrorRoot   string
	SourceFiles  []string
	Direct       bool
	NeedsMain    bool
	LibraryName  string
	ClassName    string
	GoPreamble   string
	DartPreamble string

	Calls                 []*callModel
	TopCalls              []*callModel
	Types                 []*wireType
	Structs               []*structModel
	Opaques               []*opaqueModel
	Named                 []*namedModel
	Interfaces            []*interfaceModel
	UsesTime              bool
	UsesInternetIP        bool
	UsesIPPrefix          bool
	UsesURL               bool
	UsesUUID              bool
	UsesDecimal           bool
	UsesShopspringDecimal bool
	UsesAPDDecimal        bool
	UsesBigInt            bool
	UsesDartOpaque        bool
	UsesStreamSink        bool
	// UsesRuntimePackage tracks whether the generated bridge has to import the
	// fgb runtime package (DartOpaque and StreamSink live there).
	UsesRuntimePackage bool
	// SupportPackagePath is the import path of the generated support package.
	SupportPackagePath string
	// ExternalImports are packages that own reachable named types outside the
	// input package. Generated codec signatures mention those concrete Go
	// types, so the bridge source must import them explicitly.
	ExternalImports []goImportModel
	// GoPackageAliases is the renderer lookup paired with ExternalImports.
	GoPackageAliases map[string]string
	codecSupport     map[codecCacheKey]bool
}

type goImportModel struct {
	Alias string
	Path  string
}

// codecMode mirrors flutter_rust_bridge's directional codec model. The
// preferred pair is CST for Dart -> Go and DCO for Go -> Dart. Calls containing
// types that cannot be represented by that pair fall back to the existing
// StandardMethodCodec-compatible transport.
type codecMode string

const (
	codecModeCST      codecMode = "cst"
	codecModeDCO      codecMode = "dco"
	codecModeStandard codecMode = "standard"
)

type codecModePack struct {
	DartToGo codecMode
	GoToDart codecMode
}

type callModel struct {
	ID          int
	GoName      string
	DartName    string
	Mode        bridgemodel.CallMode
	WireName    string
	Docs        string
	SourceFile  string
	Receiver    *wireType
	PointerRecv bool
	Params      []*paramModel
	// Results holds the non-error results in Go order. Two or more become a
	// Dart record; exactly one stays a plain value.
	Results []*resultModel
	// NamedResults records that every Go result carries a name, so the Dart
	// record uses named fields instead of positional ones.
	NamedResults bool
	// ErrorCount is how many `error` results the Go function declares. Any
	// non-nil one fails the call; several are reported together.
	ErrorCount int
	GoTarget   string
	Codec      codecModePack
	// StreamParam is set when the call produces a Dart Stream on its own
	// (exactly one StreamSink parameter and no non-error result). The
	// parameter is then hidden from the Dart signature and the generated
	// function returns Stream<T> instead of Future<void>.
	StreamParam *paramModel
	// ContextIndex is the position of a context.Context parameter in the Go
	// signature, or -1. The bridge supplies the context and cancels it when
	// the Dart side stops listening to the stream this call owns.
	ContextIndex int
	// Overrides marks a method that shadows one promoted from an embedded
	// struct, so the Dart declaration carries @override.
	Overrides bool
	// Operator is the Dart operator symbol this method is rendered as ("+",
	// "<=", "~", ...), or "" for an ordinary method. A Go method qualifies by
	// name and signature alone; see dartOperatorFor.
	Operator string
}

// dartMember is what the call occupies in its receiver's Dart member
// namespace. An operator does not take the ordinary method name, so collision
// checks and override detection compare this rather than DartName: otherwise a
// promoted `operator +` looks like the member an unrelated `add()` overrides.
func (c *callModel) dartMember() string {
	if c == nil {
		return ""
	}
	if c.Operator != "" {
		return "operator " + c.Operator
	}
	return c.DartName
}

// resultModel is one non-error result of a bridged call.
type resultModel struct {
	GoName   string
	DartName string
	// GoIndex is the position in the Go result list, which errors share.
	GoIndex int
	Type    *wireType
}

// resultShape reports, in Go result order, whether each result is an error.
// Errors may appear anywhere in the signature.
func (c *callModel) resultShape() []bool {
	shape := make([]bool, 0, len(c.Results)+c.ErrorCount)
	next := 0
	for _, result := range c.Results {
		for next < result.GoIndex {
			shape = append(shape, true)
			next++
		}
		shape = append(shape, false)
		next++
	}
	for len(shape) < len(c.Results)+c.ErrorCount {
		shape = append(shape, true)
	}
	return shape
}

type paramModel struct {
	GoName   string
	DartName string
	CName    string
	Type     *wireType
	// Nullable marks a callback parameter listed in `//fgb:nullable`: the Dart
	// signature accepts null and Go receives a nil func value.
	Nullable bool
}

// wireType is the generator's typed IR, analogous to flutter_rust_bridge's
// MirType.  Parsing produces one canonical node per go/types.Type; codec
// capability checks and every language-specific renderer consume this model
// instead of rediscovering type behavior independently.
type wireType struct {
	ID       int
	Kind     typeKind
	Original types.Type
	DartType string
	Elem     *wireType
	Key      *wireType
	Length   int64
	Named    *namedModel
	Atomic   *atomicModel
	Struct   *structModel
	Opaque   *opaqueModel
	Callback *callbackModel
	// Interface is set for a Go interface type.
	Interface *interfaceModel
	// Stream is the element type of a fgb.StreamSink[T] parameter or field.
	Stream *wireType
	// ChannelStream marks a `chan<- T` parameter: the bridge owns the channel,
	// drains it into the Dart stream, and closes it when the call returns.
	ChannelStream bool
	BasicKind     types.BasicKind
	BitSize       int
	Signed        bool
}

// atomicModel maps a Go atomic wrapper to the value returned by Load and
// accepted by Store. Dart sees only Value's type; pointer nullability remains
// represented by the ordinary kindPointer wrapper.
type atomicModel struct {
	Value *wireType
}

// usesPointerCodec reports whether passing this Go value to a generated codec
// by value would copy an atomic wrapper (or a struct containing one).
func (t *wireType) usesPointerCodec(seen map[int]bool) bool {
	if t == nil || seen[t.ID] {
		return false
	}
	seen[t.ID] = true
	defer delete(seen, t.ID)
	switch t.Kind {
	case kindAtomic:
		return true
	case kindStruct:
		for _, field := range t.Struct.allFields() {
			if field.Type.usesPointerCodec(seen) {
				return true
			}
		}
	case kindNamed:
		return t.Named.Underlying.usesPointerCodec(seen)
	case kindArray:
		return t.Elem.usesPointerCodec(seen)
	}
	return false
}

// nilableWithoutPointer reports whether the Go type can be nil on its own,
// without being wrapped in a pointer: closures, slices, maps and the typed
// list shapes. These are exactly the types `//fgb:nullable` may mark - every
// other type expresses optionality through a Go pointer.
func (t *wireType) nilableWithoutPointer() bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case kindError, kindCallback, kindSlice, kindMap, kindBytes, kindInt32List, kindInt64List, kindFloat64List, kindInterface:
		return true
	default:
		// Arrays are fixed-size values in Go and can never be nil.
		return false
	}
}

type namedModel struct {
	GoName     string
	DartName   string
	Docs       string
	SourceFile string
	Type       *wireType
	Underlying *wireType
	Constants  []*constantModel
	Methods    []*callModel
}

type constantModel struct {
	GoName      string
	DartName    string
	Docs        string
	DartLiteral string
	IsConst     bool
}

type structModel struct {
	GoName     string
	DartName   string
	Docs       string
	SourceFile string
	Type       *wireType
	// Super is the embedded struct this one promotes, rendered as a Dart
	// superclass. Its fields travel flattened into this struct's wire form,
	// exactly like Go promotes them.
	Super *structModel
	// Subclassed marks a struct another one embeds: Dart forbids extending a
	// `final` class, so it is rendered `base` instead.
	Subclassed bool
	// Interfaces are the bridged interfaces this struct satisfies, rendered
	// as a Dart `implements` clause.
	Interfaces []*interfaceModel
	Fields     []*fieldModel
	Methods    []*callModel
}

// allFields lists the wire fields of a struct: the promoted ones first (in
// superclass order, matching Go's memory layout), then its own. Both language
// codecs walk this list so the two sides always agree.
func (s *structModel) allFields() []*fieldModel {
	if s == nil {
		return nil
	}
	if s.Super == nil {
		return s.Fields
	}
	inherited := s.Super.allFields()
	result := make([]*fieldModel, 0, len(inherited)+len(s.Fields))
	result = append(result, inherited...)
	return append(result, s.Fields...)
}

type fieldModel struct {
	GoName   string
	DartName string
	CName    string
	WireName string
	Type     *wireType
	Optional bool
	// Nullable marks a field carrying `fgb:"nullable"`: its Go type can be nil
	// without a pointer, and that nil survives in both directions.
	Nullable bool
	// NonFinal drops the Dart `final` keyword (fgb:"non-final").
	NonFinal bool
	// DefaultValue is a raw Dart expression used as the constructor default
	// (fgb:"defaultValue: ..."); such fields are not `required`.
	DefaultValue string
}

// dartType is the Dart spelling of the field, widened to nullable when the
// field carries `fgb:"nullable"`. Pointer fields already carry their `?`.
func (f *fieldModel) dartType() string {
	if f.Nullable {
		return nullableDartType(f.Type.DartType)
	}
	return f.Type.DartType
}

// optional reports whether the Dart constructor may omit the field.
func (f *fieldModel) optional() bool { return f.Optional || f.Nullable }

// interfaceModel is a Go interface rendered as a Dart
// `abstract interface class`. Values travel with an implementation tag and
// payload, so the receiving side knows which decoder to use.
type interfaceModel struct {
	GoName     string
	DartName   string
	Docs       string
	SourceFile string
	Type       *wireType
	// Methods are declaration-only: a call on an interface value dispatches
	// to the concrete Dart class, which owns the bridged implementation.
	Methods []*callModel
	// Implementors are the bridged types that satisfy the interface, in a
	// stable order. Input interfaces use the index as the tag; dependency
	// interfaces use WireTag.
	Implementors []*implementorModel
	// UsesContentTags reports whether every implementor carries a string
	// WireTag (dependency interfaces) rather than a positional integer index
	// (input-package interfaces). Decided once in mapInterface so the encoder,
	// decoder, and tag-literal helpers all read the same source of truth
	// instead of inferring the scheme from the first implementor.
	UsesContentTags bool
}

// implementorModel links a concrete bridged type to the interface it
// satisfies.
type implementorModel struct {
	DartName string
	Type     *wireType
	// WireTag is a stable content identifier for dependency interfaces. Input
	// package interfaces leave it empty and retain their historical index ABI.
	WireTag string
	// DecodeOnly marks an additional Go dynamic representation of the same
	// Dart class. For example, both T and *T implement an interface when T has
	// value-receiver methods; Dart encodes the canonical T tag, while Go may
	// decode either tag when returning an interface value.
	DecodeOnly bool
	// GoTypes are the concrete dynamic types accepted when Go encodes this
	// tagged implementation. AddressValue is used when an opaque T must be
	// copied to *T before it can enter the handle registry.
	GoTypes []implementorGoType
}

type implementorGoType struct {
	Type         types.Type
	AddressValue bool
}

type opaqueModel struct {
	GoName     string
	DartName   string
	Docs       string
	SourceFile string
	Type       *wireType
	// Synthetic marks a collection represented by a handle because copying its
	// atomic elements would make generated Go invalid.
	Synthetic bool
	// Interfaces are the bridged interfaces this handle satisfies.
	Interfaces []*interfaceModel
	Methods    []*callModel
}

// callbackModel describes a Go function-type parameter: Dart supplies a
// closure (sync or async), Go receives a synthesized func value that parks
// its goroutine until the Dart side replies.
type callbackModel struct {
	// Params travel Go -> Dart when the callback is invoked.
	Params []*wireType
	// Result (nil for void) travels Dart -> Go in the reply.
	Result *wireType
	// HasError marks a trailing `error` result: Dart exceptions surface as a
	// returned error instead of a panic.
	HasError bool
}
