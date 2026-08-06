package model

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// API is the typed public surface discovered in one Go package.
//
// The generator deliberately retains the official go/types objects instead of
// translating them into a second, lossy type system.
type API struct {
	Package     *packages.Package
	InputDir    string
	SourceFiles []string
	Callables   []*Callable
	Types       map[*types.TypeName]*TypeDecl
	Constants   map[*types.Named][]*Constant
	// IgnoredTypes records input-package type names carrying fgb(ignore).
	// Their methods are dropped; referencing them from a bridged signature is
	// a generation error.
	IgnoredTypes map[string]bool
	// OpaqueTypes records input-package type names carrying fgb(opaque):
	// they always bridge as GoOpaque handles, never as serialized fields.
	OpaqueTypes map[string]bool
}

// CallMode controls which Dart entrypoint is emitted for a Go callable.
// Unmarked declarations use CallModeSync for backwards-compatible, blocking
// FFI calls; an explicit fgb(async) marker opts into the Dart API DL path.
type CallMode string

const (
	CallModeSync  CallMode = "sync"
	CallModeAsync CallMode = "async"
)

type Callable struct {
	Func        *types.Func
	Signature   *types.Signature
	Position    token.Pos
	SourceFile  string
	Docs        string
	DartName    string
	Mode        CallMode
	Receiver    *types.Named
	PointerRecv bool
	// NullableParams lists Go parameter names marked by
	// `//fgb:nullable = "a,b"`. Only callback (function-typed) parameters may
	// appear here; the generator rejects anything else.
	NullableParams []string
}

func (c *Callable) IsMethod() bool { return c.Receiver != nil }

type TypeDecl struct {
	Object     *types.TypeName
	Named      *types.Named
	Position   token.Pos
	SourceFile string
	Docs       string
	DartName   string
	AST        *ast.TypeSpec
	// Enum marks a named integer/string type that explicitly requests Dart
	// enum generation through //fgb:enum.
	Enum bool
	// Methods carries the per-method directives of an interface declaration,
	// keyed by the Go method name. Interface methods have no bodies, so this
	// is the only place their //fgb: directives can come from.
	Methods map[string]*InterfaceMethod
}

// InterfaceMethod is the directive-carrying description of one interface
// method.
type InterfaceMethod struct {
	GoName   string
	DartName string
	Mode     CallMode
	Docs     string
	Ignore   bool
}

type Constant struct {
	Object   *types.Const
	Position token.Pos
	Docs     string
	DartName string
}
