// Package fgb holds the types that must appear in the signatures of a Go API
// bridged to Dart.
//
// This file is generated under `internal` next to bridge_generated.go and
// lives inside your own module, so a bridged project needs no external
// dependency and these types stay out of your public API. It is rewritten on
// every code generation run - keep your own code out of it.
package fgb

import (
	"errors"
	"runtime"
	"sync/atomic"
)

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

// Stream event kinds shared with the generated bridge and the Dart runtime.
const (
	StreamEventItem  int32 = 0
	StreamEventClose int32 = 1
	StreamEventError int32 = 2
)

// ErrStreamClosed is returned by StreamSink methods once the stream can no
// longer accept values: after Close, or after the Dart side stopped listening
// (the subscription was cancelled or its StreamController was closed).
var ErrStreamClosed = errors.New("fgb: the Dart side is no longer listening to this stream")

// StreamSink delivers a sequence of values produced by Go to a Dart Stream.
// It mirrors flutter_rust_bridge's StreamSink.
//
// A Go function receives a sink as an ordinary parameter (or struct field);
// generated bridge code creates it. Values added after the Dart side stops
// listening are dropped and reported as ErrStreamClosed, which is the signal
// to stop producing:
//
//	//fgb:async
//	func Ticks(count int, sink fgb.StreamSink[int]) error {
//		defer sink.Close()
//		for i := 0; i < count; i++ {
//			if err := sink.Add(i); err != nil {
//				return nil // the listener went away
//			}
//		}
//		return nil
//	}
//
// Copies share one underlying stream and are safe to use from several
// goroutines. The Dart stream is closed by Close, by the Go side dropping its
// last copy, or by the Dart side itself. When you do not need AddError and the
// values are produced before the function returns, a plain `chan<- T`
// parameter is simpler: the bridge then owns the channel and closes it for you.
type StreamSink[T any] struct {
	ref    *streamSinkRef
	encode func(T) (any, error)
}

type streamSinkRef struct {
	handle int64
	// post reports false once the Dart side stopped listening.
	post   func(handle int64, kind int32, payload any) bool
	closed atomic.Bool
}

// NewStreamSink is called by generated bridge code; user code never needs it.
// release runs after the last copy of the returned sink becomes unreachable.
func NewStreamSink[T any](
	handle int64,
	encode func(T) (any, error),
	post func(handle int64, kind int32, payload any) bool,
	release func(handle int64),
) StreamSink[T] {
	ref := &streamSinkRef{handle: handle, post: post}
	if release != nil {
		runtime.AddCleanup(ref, release, handle)
	}
	return StreamSink[T]{ref: ref, encode: encode}
}

// Add delivers one value to the Dart stream. It does not block: the value is
// posted to the Dart event loop and delivered there. ErrStreamClosed means
// the listener is gone and production should stop.
func (s StreamSink[T]) Add(value T) error {
	if s.IsClosed() {
		return ErrStreamClosed
	}
	encoded, err := s.encode(value)
	if err != nil {
		return err
	}
	if !s.ref.post(s.ref.handle, StreamEventItem, encoded) {
		s.ref.closed.Store(true)
		return ErrStreamClosed
	}
	return nil
}

// AddError delivers err to the Dart stream as a stream error. The stream stays
// open; use Close when the sequence is finished.
func (s StreamSink[T]) AddError(err error) error {
	if s.IsClosed() {
		return ErrStreamClosed
	}
	message := "unknown error"
	if err != nil {
		message = err.Error()
	}
	if !s.ref.post(s.ref.handle, StreamEventError, message) {
		s.ref.closed.Store(true)
		return ErrStreamClosed
	}
	return nil
}

// Close ends the Dart stream. It is idempotent, and calling it is the normal
// way to signal completion - `defer sink.Close()` is the expected idiom.
func (s StreamSink[T]) Close() {
	if s.ref == nil || !s.ref.closed.CompareAndSwap(false, true) {
		return
	}
	s.ref.post(s.ref.handle, StreamEventClose, nil)
}

// IsClosed reports whether the stream stopped accepting values.
func (s StreamSink[T]) IsClosed() bool {
	return s.ref == nil || s.ref.closed.Load()
}
