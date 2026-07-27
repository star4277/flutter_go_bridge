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
	kindNamed       typeKind = "named"
	kindBytes       typeKind = "bytes"
	kindInt32List   typeKind = "int32_list"
	kindInt64List   typeKind = "int64_list"
	kindFloat64List typeKind = "float64_list"
)

type unit struct {
	PackagePath  string
	PackageName  string
	InputDir     string
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
}

type paramModel struct {
	GoName   string
	DartName string
	CName    string
	Type     *wireType
}

// wireType is the generator's typed IR, analogous to flutter_rust_bridge's
// MirType.  Parsing produces one canonical node per go/types.Type; codec
// capability checks and every language-specific renderer consume this model
// instead of rediscovering type behavior independently.
type wireType struct {
	ID        int
	Kind      typeKind
	Original  types.Type
	DartType  string
	Elem      *wireType
	Key       *wireType
	Length    int64
	Named     *namedModel
	Struct    *structModel
	Opaque    *opaqueModel
	BasicKind types.BasicKind
	BitSize   int
	Signed    bool
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
