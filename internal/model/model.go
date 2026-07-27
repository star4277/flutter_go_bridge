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
}

type Constant struct {
	Object   *types.Const
	Position token.Pos
	Docs     string
	DartName string
}
