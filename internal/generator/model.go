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
	ID           int
	GoName       string
	DartName     string
	Mode         bridgemodel.CallMode
	WireName     string
	Docs         string
	SourceFile   string
	Receiver     *wireType
	PointerRecv  bool
	Params       []*paramModel
	Result       *wireType
	HasError     bool
	GoTarget     string
	ResultGoName string
	Codec        codecModePack
	// StreamParam is set when the call produces a Dart Stream on its own
	// (exactly one StreamSink parameter and no non-error result). The
	// parameter is then hidden from the Dart signature and the generated
	// function returns Stream<T> instead of Future<void>.
	StreamParam *paramModel
	// ContextIndex is the position of a context.Context parameter in the Go
	// signature, or -1. The bridge supplies the context and cancels it when
	// the Dart side stops listening to the stream this call owns.
	ContextIndex int
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
	case kindCallback, kindSlice, kindMap, kindBytes, kindInt32List, kindInt64List, kindFloat64List:
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
	Fields     []*fieldModel
	Methods    []*callModel
}

type fieldModel struct {
	GoName   string
	DartName string
	CName    string
	WireName string
	Type     *wireType
	Optional bool
	// NonFinal drops the Dart `final` keyword (fgb:"non-final").
	NonFinal bool
	// DefaultValue is a raw Dart expression used as the constructor default
	// (fgb:"defaultValue: ..."); such fields are not `required`.
	DefaultValue string
}

type opaqueModel struct {
	GoName     string
	DartName   string
	Docs       string
	SourceFile string
	Type       *wireType
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
