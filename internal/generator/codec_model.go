package generator

func preferredCodecForCall(call *callModel, cache map[codecCacheKey]bool) codecModePack {
	if call == nil {
		return codecModePack{DartToGo: codecModeStandard, GoToDart: codecModeStandard}
	}
	cst := true
	if call.Receiver != nil {
		cst = call.Receiver.supportsCodecCached(codecModeCST, map[int]bool{}, cache)
	}
	for _, param := range call.Params {
		cst = cst && param.Type.supportsCodecCached(codecModeCST, map[int]bool{}, cache)
	}
	dco := true
	for _, result := range call.Results {
		dco = dco && result.Type.supportsCodecCached(codecModeDCO, map[int]bool{}, cache)
	}
	if cst && dco {
		return codecModePack{DartToGo: codecModeCST, GoToDart: codecModeDCO}
	}
	return codecModePack{DartToGo: codecModeStandard, GoToDart: codecModeStandard}
}

type codecCacheKey struct {
	id   int
	mode codecMode
}

func (t *wireType) supportsCodec(mode codecMode, seen map[int]bool) bool {
	return t.supportsCodecCached(mode, seen, nil)
}

func (t *wireType) supportsCodecCached(mode codecMode, seen map[int]bool, cache map[codecCacheKey]bool) bool {
	result, _ := t.codecSupportResult(mode, seen, cache)
	return result
}

// codecSupportResult also reports whether the answer relied on the optimistic
// assumption used to break a recursive type cycle. Such a true result cannot
// be cached until the outer traversal has checked the rest of the cycle.
func (t *wireType) codecSupportResult(mode codecMode, seen map[int]bool, cache map[codecCacheKey]bool) (bool, bool) {
	if t == nil {
		return true, false
	}
	if mode == codecModeStandard {
		return true, false
	}
	if seen[t.ID] {
		return true, true
	}
	key := codecCacheKey{id: t.ID, mode: mode}
	if cache != nil {
		if result, ok := cache[key]; ok {
			return result, false
		}
	}
	seen[t.ID] = true
	defer delete(seen, t.ID)

	switch t.Kind {
	case kindBool, kindString, kindSigned, kindUnsigned, kindFloat, kindBigInt,
		kindTime, kindInternetIP, kindIPPrefix, kindURL, kindUUID, kindDuration, kindBytes, kindInt32List, kindInt64List, kindFloat64List,
		kindOpaque, kindDartOpaque, kindCallback, kindStreamSink:
		if cache != nil {
			cache[key] = true
		}
		return true, false
	case kindPointer:
		result, cyclic := t.Elem.codecSupportResult(mode, seen, cache)
		if cache != nil && (!cyclic || !result) {
			cache[key] = result
		}
		return result, cyclic
	case kindSlice, kindArray:
		result, cyclic := t.Elem.codecSupportResult(mode, seen, cache)
		if cache != nil && (!cyclic || !result) {
			cache[key] = result
		}
		return result, cyclic
	case kindStruct:
		cyclic := false
		for _, field := range t.Struct.allFields() {
			supported, childCyclic := field.Type.codecSupportResult(mode, seen, cache)
			cyclic = cyclic || childCyclic
			if !supported {
				if cache != nil {
					cache[key] = false
				}
				return false, cyclic
			}
		}
		if cache != nil && !cyclic {
			cache[key] = true
		}
		return true, cyclic
	case kindNamed:
		result, cyclic := t.Named.Underlying.codecSupportResult(mode, seen, cache)
		if cache != nil && (!cyclic || !result) {
			cache[key] = result
		}
		return result, cyclic
	case kindAtomic:
		result, cyclic := t.Atomic.Value.codecSupportResult(mode, seen, cache)
		if cache != nil && (!cyclic || !result) {
			cache[key] = result
		}
		return result, cyclic
	case kindMap, kindAny, kindInterface:
		// Dart_CObject has no map representation, a C struct cannot safely
		// represent arbitrary dynamic values, and an interface is a tagged
		// union. These remain on the fallback StandardMethodCodec path.
		if cache != nil {
			cache[key] = false
		}
		return false, false
	default:
		return false, false
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
