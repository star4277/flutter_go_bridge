// Package fgb is the tiny runtime companion of flutter_go_bridge_codegen.
//
// Generated Go bridges are self-contained; this package only holds the types
// that must appear in user API signatures, currently DartOpaque. A Go module
// needs this dependency only when its bridged API uses those types.
package fgb

import "runtime"

// DartOpaque is an opaque reference to a Dart object. The object itself stays
// on the Dart side; Go can hold the reference, pass it around, and hand it
// back to Dart, but cannot look inside it. This mirrors flutter_rust_bridge's
// DartOpaque.
//
// Values are created by generated bridge code when a Dart object crosses into
// Go. Once the last Go copy becomes unreachable, the Dart side is notified and
// releases the underlying object. The zero DartOpaque references nothing.
type DartOpaque struct {
	ref *dartOpaqueRef
}

type dartOpaqueRef struct {
	handle int64
}

// NewDartOpaque wraps a Dart-side handle. It is invoked by generated bridge
// code; release (may be nil) runs exactly once after the last copy of the
// returned value becomes unreachable.
func NewDartOpaque(handle int64, release func(handle int64)) DartOpaque {
	ref := &dartOpaqueRef{handle: handle}
	if release != nil {
		runtime.AddCleanup(ref, release, handle)
	}
	return DartOpaque{ref: ref}
}

// Handle returns the Dart-side handle, or 0 for the zero DartOpaque.
func (d DartOpaque) Handle() int64 {
	if d.ref == nil {
		return 0
	}
	return d.ref.handle
}

// IsValid reports whether the value references a Dart object.
func (d DartOpaque) IsValid() bool {
	return d.ref != nil && d.ref.handle != 0
}
