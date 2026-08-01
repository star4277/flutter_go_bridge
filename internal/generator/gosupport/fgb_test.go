package fgb

import (
	"errors"
	"sync/atomic"
	"testing"
)

var (
	opaqueReleaseHandle atomic.Int64
	opaqueReleaseSeen   atomic.Bool
)

func opaqueRelease(h int64) {
	opaqueReleaseHandle.Store(h)
	opaqueReleaseSeen.Store(true)
}

func TestDartOpaqueZero(t *testing.T) {
	var zero DartOpaque
	if zero.Handle() != 0 {
		t.Fatalf("zero handle = %d", zero.Handle())
	}
	if zero.IsValid() {
		t.Fatalf("zero should be invalid")
	}
}

func TestDartOpaqueNew(t *testing.T) {
	opaqueReleaseSeen.Store(false)
	opaqueReleaseHandle.Store(-1)
	d := NewDartOpaque(42, opaqueRelease)
	if d.Handle() != 42 {
		t.Fatalf("handle = %d", d.Handle())
	}
	if !d.IsValid() {
		t.Fatalf("expected valid")
	}
	// zero-handle variant is invalid even when non-nil ref.
	d0 := NewDartOpaque(0, nil)
	if d0.IsValid() {
		t.Fatalf("handle 0 must be invalid")
	}
	if d0.Handle() != 0 {
		t.Fatalf("handle = %d", d0.Handle())
	}
}

func TestNewStreamSinkAndCloseIdempotent(t *testing.T) {
	var posts atomic.Int32
	posted := func(handle int64, kind int32, payload any) bool {
		_ = handle
		_ = payload
		if kind == StreamEventClose {
			posts.Add(1)
		}
		return true
	}
	sink := NewStreamSink[int](
		1,
		func(v int) (any, error) { return v, nil },
		posted,
		func(handle int64) {},
	)
	if sink.IsClosed() {
		t.Fatalf("new sink should be open")
	}
	if err := sink.Add(7); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := sink.AddError(errors.New("boom")); err != nil {
		t.Fatalf("AddError failed: %v", err)
	}
	sink.Close()
	if !sink.IsClosed() {
		t.Fatalf("expected closed")
	}
	sink.Close() // idempotent
	if err := sink.Add(1); err != ErrStreamClosed {
		t.Fatalf("early Add got %v", err)
	}
	if posts.Load() != 1 {
		t.Fatalf("close posted %d times", posts.Load())
	}
}

func TestStreamSinkPostReturnsFalse(t *testing.T) {
	sink := NewStreamSink[int](
		1,
		func(v int) (any, error) { return v, nil },
		func(handle int64, kind int32, payload any) bool { return false },
		nil,
	)
	if err := sink.Add(1); err != ErrStreamClosed {
		t.Fatalf("Add got %v, want ErrStreamClosed", err)
	}
	if !sink.IsClosed() {
		t.Fatalf("expected closed after failed post")
	}
	if err := sink.Add(1); err != ErrStreamClosed {
		t.Fatalf("second Add got %v", err)
	}
	if err := sink.AddError(errors.New("x")); err != ErrStreamClosed {
		t.Fatalf("AddError got %v", err)
	}
}

func TestStreamSinkEncodeError(t *testing.T) {
	want := errors.New("encode failed")
	sink := NewStreamSink[int](
		1,
		func(v int) (any, error) { return nil, want },
		func(handle int64, kind int32, payload any) bool { return true },
		nil,
	)
	if err := sink.Add(1); err != want {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestStreamSinkZeroValue(t *testing.T) {
	var sink StreamSink[int]
	if !sink.IsClosed() {
		t.Fatalf("zero sink should be closed")
	}
	sink.Close() // must not panic
	if err := sink.Add(1); err != ErrStreamClosed {
		t.Fatalf("Add got %v", err)
	}
	if err := sink.AddError(errors.New("x")); err != ErrStreamClosed {
		t.Fatalf("AddError got %v", err)
	}
}

func TestStreamSinkAddErrorNilMessage(t *testing.T) {
	posted := false
	sink := NewStreamSink[int](
		1,
		func(v int) (any, error) { return v, nil },
		func(handle int64, kind int32, payload any) bool {
			if kind == StreamEventError && payload == "unknown error" {
				posted = true
			}
			return true
		},
		nil,
	)
	if err := sink.AddError(nil); err != nil {
		t.Fatalf("AddError(nil) = %v", err)
	}
	if !posted {
		t.Fatalf("nil error not posted with default message")
	}
}



func TestStreamSinkAddErrorPostReturnsFalse(t *testing.T) {
	sink := NewStreamSink[int](
		1,
		func(v int) (any, error) { return v, nil },
		func(handle int64, kind int32, payload any) bool { return false },
		nil,
	)
	if err := sink.AddError(errors.New("burst")); err != ErrStreamClosed {
		t.Fatalf("AddError got %v, want ErrStreamClosed", err)
	}
	if !sink.IsClosed() {
		t.Fatalf("expected closed after failed error post")
	}
}
