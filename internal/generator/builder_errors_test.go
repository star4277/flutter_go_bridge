package generator

import (
	"strings"
	"testing"
)

func requireError(t *testing.T, err error, fragment string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), fragment) {
		t.Fatalf("expected error containing %q, got %v", fragment, err)
	}
}

// TestGenerateTwoContextParametersRejected drives mapCallable's duplicate
// context guard.
func TestGenerateTwoContextParametersRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "context"

func Use(a context.Context, b context.Context) {}
`)
	requireError(t, err, "only one context.Context")
}

// TestGenerateAtomicInCollectionParameterRejected drives mapCallable's
// atomic-inside-a-collection parameter guard.
func TestGenerateAtomicInCollectionParameterRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "sync/atomic"

func Use(flags []atomic.Int64) {}
`)
	requireError(t, err, "contains atomic values inside a collection")
}

// TestGenerateAtomicByValueParameterRejected drives mapCallable's must-be-
// pointer guard for atomic parameters.
func TestGenerateAtomicByValueParameterRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "sync/atomic"

func Use(flag atomic.Int64) {}
`)
	requireError(t, err, "must be passed by pointer")
}

// TestGenerateAtomicInCollectionResultRejected drives mapCallable's
// atomic-inside-a-collection result guard.
func TestGenerateAtomicInCollectionResultRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "sync/atomic"

func Use() ([]atomic.Int64, error) { return nil, nil }
`)
	requireError(t, err, "contains atomic values inside a collection")
}

// TestGenerateRecvOnlyChannelRejected drives mapChannelStream's direction guard.
func TestGenerateRecvOnlyChannelRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Poll(out <-chan int) error { return nil }
`)
	requireError(t, err, "send-only channels")
}

// TestGenerateStreamOfStreamSinksRejected drives mapChannelStream's
// stream-of-sinks guard.
func TestGenerateStreamOfStreamSinksRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

import "example.com/fixture/internal/fgb"

//fgb:async
func Watch(out chan<- fgb.StreamSink[string]) error { return nil }
`)
	requireError(t, err, "stream of stream sinks")
}

// TestGenerateCallbackNestedFunctionRejected drives mapCallback's nested
// function-type guard.
func TestGenerateCallbackNestedFunctionRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Transform(make func(f func(int)) int) int { return 0 }
`)
	requireError(t, err, "nested function types")
}

// TestGenerateCallbackTooManyResultsRejected drives mapCallback's at-most-one
// non-error result guard.
func TestGenerateCallbackTooManyResultsRejected(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:async
func Add(f func() (int, string)) int { return 0 }
`)
	requireError(t, err, "at most one non-error result")
}

// TestGenerateFlutterGoBridgeTagSkip drives skipStructField's
// flutter_go_bridge:"-" branch: a field opted out through that tag is dropped
// from the value struct but no GoOpaque fallback is forced.
func TestGenerateFlutterGoBridgeTagSkip(t *testing.T) {
	apiDart, _, _, _, err := generateFixture(t, `package api

type Item struct {
	Kept  string
	Dropped string `+"`flutter_go_bridge:\"-\"`"+`
}

func Make(i Item) Item { return i }
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apiDart, "dropped") {
		t.Fatalf("flutter_go_bridge:\"-\" field leaked into Dart:\n%s", apiDart)
	}
	if !strings.Contains(apiDart, "final String kept;") {
		t.Fatalf("kept field missing:\n%s", apiDart)
	}
}

// TestGenerateIgnoredTypeUsedByNeededSignature drives ensureNamedSupported's
// fgb(ignore) rejection when an ignored type surfaces in the bridged API.
func TestGenerateIgnoredTypeUsedByNeededSignature(t *testing.T) {
	_, _, _, _, err := generateFixture(t, `package api

//fgb:ignore
type Gone struct{ X int }

func Make() Gone { return Gone{} }
`)
	requireError(t, err, "fgb(ignore)")
}
