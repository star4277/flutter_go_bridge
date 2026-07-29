package generator

import (
	"go/types"

	bridgemodel "github.com/star4277/flutter-go-bridge-gokit/internal/model"
)

type typeKind string

const (
	kindBool        typeKind = "bool"
	kindString      typeKind = "string"
	kindSigned      typeKind = "signed"
	kindUnsigned    typeKind = "unsigned"
	kindFloat       typeKind = "float"
	kindBigInt      typeKind = "big_int"
	kindTime        typeKind = "time"
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

	Calls          []*callModel
	TopCalls       []*callModel
	Types          []*wireType
	Structs        []*structModel
	Opaques        []*opaqueModel
	Named          []*namedModel
	Interfaces     []*interfaceModel
	UsesTime       bool
	UsesBigInt     bool
	UsesDartOpaque bool
	UsesStreamSink bool
	// UsesRuntimePackage tracks whether the generated bridge has to import the
	// fgb runtime package (DartOpaque and StreamSink live there).
	UsesRuntimePackage bool
	// SupportPackagePath is the import path of the generated support package.
	SupportPackagePath string
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
}

// resultModel is one non-error result of a bridged call.
type resultModel struct {
	GoName   string
	DartName string
	// GoIndex is the position in the Go result list, which errors share.
	GoIndex int
	Type    *wireType
}

func (c *callModel) hasError() bool { return c != nil && c.ErrorCount > 0 }

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

// singleResult is the only non-error result, or nil when the call returns
// nothing or a record.
func (c *callModel) singleResult() *wireType {
	if c == nil || len(c.Results) != 1 {
		return nil
	}
	return c.Results[0].Type
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

// nilableWithoutPointer reports whether the Go type can be nil on its own,
// without being wrapped in a pointer: closures, slices, maps and the typed
// list shapes. These are exactly the types `//fgb:nullable` may mark - every
// other type expresses optionality through a Go pointer.
func (t *wireType) nilableWithoutPointer() bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case kindCallback, kindSlice, kindMap, kindBytes, kindInt32List, kindInt64List, kindFloat64List, kindInterface:
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
		return f.Type.DartType + "?"
	}
	return f.Type.DartType
}

// optional reports whether the Dart constructor may omit the field.
func (f *fieldModel) optional() bool { return f.Optional || f.Nullable }

// interfaceModel is a Go interface rendered as a Dart
// `abstract interface class`. Values travel tagged with the index of their
// concrete type, so the receiving side knows which decoder to use.
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
	// stable order; the index is the wire tag.
	Implementors []*implementorModel
}

// implementorModel links a concrete bridged type to the interface it
// satisfies.
type implementorModel struct {
	DartName string
	Type     *wireType
}

type opaqueModel struct {
	GoName     string
	DartName   string
	Docs       string
	SourceFile string
	Type       *wireType
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
