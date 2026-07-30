package generator

func preferredCodecForCall(call *callModel) codecModePack {
	if call == nil {
		return codecModePack{DartToGo: codecModeStandard, GoToDart: codecModeStandard}
	}
	cst := true
	if call.Receiver != nil {
		cst = call.Receiver.supportsCodec(codecModeCST, map[int]bool{})
	}
	for _, param := range call.Params {
		cst = cst && param.Type.supportsCodec(codecModeCST, map[int]bool{})
	}
	dco := true
	for _, result := range call.Results {
		dco = dco && result.Type.supportsCodec(codecModeDCO, map[int]bool{})
	}
	if cst && dco {
		return codecModePack{DartToGo: codecModeCST, GoToDart: codecModeDCO}
	}
	return codecModePack{DartToGo: codecModeStandard, GoToDart: codecModeStandard}
}

func (t *wireType) supportsCodec(mode codecMode, seen map[int]bool) bool {
	if t == nil {
		return true
	}
	if mode == codecModeStandard {
		return true
	}
	if seen[t.ID] {
		return true
	}
	seen[t.ID] = true
	defer delete(seen, t.ID)

	switch t.Kind {
	case kindBool, kindString, kindSigned, kindUnsigned, kindFloat, kindBigInt,
		kindTime, kindInternetIP, kindIPPrefix, kindURL, kindRegExp, kindUUID, kindDuration, kindBytes, kindInt32List, kindInt64List, kindFloat64List,
		kindOpaque, kindDartOpaque, kindCallback, kindStreamSink:
		return true
	case kindPointer:
		return t.Elem.supportsCodec(mode, seen)
	case kindSlice, kindArray:
		return t.Elem.supportsCodec(mode, seen)
	case kindStruct:
		for _, field := range t.Struct.allFields() {
			if !field.Type.supportsCodec(mode, seen) {
				return false
			}
		}
		return true
	case kindNamed:
		return t.Named.Underlying.supportsCodec(mode, seen)
	case kindAtomic:
		return t.Atomic.Value.supportsCodec(mode, seen)
	case kindMap, kindAny, kindInterface:
		// Dart_CObject has no map representation, a C struct cannot safely
		// represent arbitrary dynamic values, and an interface is a tagged
		// union. These remain on the fallback StandardMethodCodec path.
		return false
	default:
		return false
	}
}

func (call *callModel) usesCstDco() bool {
	return call != nil && call.Codec.DartToGo == codecModeCST && call.Codec.GoToDart == codecModeDCO
}

func hasCstDcoCalls(unit *unit) bool {
	if unit == nil {
		return false
	}
	for _, call := range unit.Calls {
		if call.usesCstDco() {
			return true
		}
	}
	return false
}
