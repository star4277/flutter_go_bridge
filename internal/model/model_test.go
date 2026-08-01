package model

import (
	"go/types"
	"testing"
)

func TestCallableIsMethod(t *testing.T) {
	var (
		sig   = types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(), false)
		recv  = types.NewNamed(types.NewTypeName(0, nil, "Widget", nil), types.NewStruct(nil, nil), nil)
		method = types.NewFunc(0, nil, "Do", sig)
	)
	methodValue := &Callable{Func: method, Signature: sig, Receiver: recv}
	if !methodValue.IsMethod() {
		t.Fatalf("expected receiver callable to be a method")
	}
	if methodValue.IsMethod() == false {
		t.Fatalf("redundant")
	}

	free := &Callable{Func: method, Signature: sig}
	if free.IsMethod() {
		t.Fatalf("expected free function callable to not be a method")
	}
}
