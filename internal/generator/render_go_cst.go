package generator

import (
	"fmt"
	"go/types"
	"strconv"
)

func (r *goRenderer) renderCstDecoders() {
	for _, typ := range cstDcoReachableTypes(r.unit) {
		r.renderCstDecoder(typ)
		r.line("")
	}
}

func (r *goRenderer) renderCstDecoder(typ *wireType) {
	goType := r.goType(typ.Original)
	decodeType := goType
	if typ.usesPointerCodec(map[int]bool{}) {
		decodeType = "*" + goType
	}
	r.line("func fgbCstDecode%d(value %s, path string) (%s, error) {", typ.ID, cstGoTypeForSignature(typ), decodeType)
	switch typ.Kind {
	case kindBool:
		r.line("\tif value != 0 && value != 1 { var zero %s; return zero, fmt.Errorf(\"%%s: invalid bool value\", path) }", goType)
		r.line("\treturn value != 0, nil")
	case kindSigned:
		r.line("\traw := int64(value)")
		r.renderSignedRangeCheck(typ, goType)
		r.line("\treturn %s(raw), nil", goType)
	case kindUnsigned:
		if isDartBigIntType(typ) {
			r.line("\traw, err := fgbCstReadBigInt(value, path)")
			r.line("\tif err != nil { var zero %s; return zero, err }", goType)
			r.line("\tif raw.Sign() < 0 || !raw.IsUint64() { var zero %s; return zero, fmt.Errorf(\"%%s: unsigned integer out of range\", path) }", goType)
			r.renderUnsignedRangeCheckFromUint64(typ, goType, "raw.Uint64()")
			r.line("\treturn %s(raw.Uint64()), nil", goType)
		} else {
			r.line("\traw := uint64(value)")
			r.renderUnsignedRangeCheckFromUint64(typ, goType, "raw")
			r.line("\treturn %s(raw), nil", goType)
		}
	case kindString:
		r.raw("\tif value == nil { return \"\", fmt.Errorf(\"%s: null String\", path) }")
		r.line("\traw, err := fgbCstReadString(unsafe.Pointer(value.ptr), value.len, path)")
		r.line("\tif err != nil { return \"\", err }")
		r.line("\treturn raw, nil")
	case kindError:
		r.line("\tif value == nil { return nil, nil }")
		r.line("\traw, err := fgbCstReadString(unsafe.Pointer(value.ptr), value.len, path)")
		r.line("\tif err != nil { return nil, err }")
		r.raw("\treturn fmt.Errorf(\"%s\", raw), nil")
	case kindFloat:
		r.line("\treturn %s(value), nil", goType)
	case kindBigInt:
		_, pointer := types.Unalias(typ.Original).(*types.Pointer)
		if pointer {
			r.line("\tif value == nil { return nil, nil }")
		}
		r.line("\traw, err := fgbCstReadBigInt(value, path)")
		if pointer {
			r.line("\tif err != nil { return nil, err }")
			r.line("\treturn raw, nil")
		} else {
			r.line("\tif err != nil { var zero %s; return zero, err }", goType)
			r.line("\treturn *raw, nil")
		}
	case kindTime:
		r.line("\treturn time.UnixMicro(int64(value)).UTC(), nil")
	case kindInternetIP:
		r.line("\tif value == nil { var zero %s; return zero, fmt.Errorf(\"%%s: null IP\", path) }", goType)
		r.line("\traw, err := fgbCstReadString(unsafe.Pointer(value.ptr), value.len, path)")
		r.line("\tif err != nil { var zero %s; return zero, err }", goType)
		r.line("\tif raw == \"\" { return netip.Addr{}, nil }")
		r.line("\tresult, err := netip.ParseAddr(raw)")
		r.line("\tif err != nil { var zero %s; return zero, fmt.Errorf(\"%%s: invalid IP: %%w\", path, err) }", goType)
		r.line("\treturn result, nil")
	case kindIPPrefix:
		r.line("\tif value == nil { var zero %s; return zero, fmt.Errorf(\"%%s: null CIDR prefix\", path) }", goType)
		r.line("\traw, err := fgbCstReadString(unsafe.Pointer(value.ptr), value.len, path)")
		r.line("\tif err != nil { var zero %s; return zero, err }", goType)
		r.line("\tif raw == \"\" { return netip.Prefix{}, nil }")
		r.line("\tresult, err := netip.ParsePrefix(raw)")
		r.line("\tif err != nil { var zero %s; return zero, fmt.Errorf(\"%%s: invalid CIDR prefix: %%w\", path, err) }", goType)
		r.line("\treturn result, nil")
	case kindURL:
		r.line("\tif value == nil { var zero %s; return zero, fmt.Errorf(\"%%s: null URI\", path) }", goType)
		r.line("\traw, err := fgbCstReadString(unsafe.Pointer(value.ptr), value.len, path)")
		r.line("\tif err != nil { var zero %s; return zero, err }", goType)
		r.renderURLParse(goType, "raw")
	case kindUUID:
		r.line("\tif value == nil { var zero %s; return zero, fmt.Errorf(\"%%s: null UUID\", path) }", goType)
		r.line("\traw, err := fgbCstReadString(unsafe.Pointer(value.ptr), value.len, path)")
		r.line("\tif err != nil { var zero %s; return zero, err }", goType)
		r.line("\tresult, err := uuid.FromString(raw)")
		r.line("\tif err != nil { var zero %s; return zero, fmt.Errorf(\"%%s: invalid UUID: %%w\", path, err) }", goType)
		r.line("\treturn result, nil")
	case kindDuration:
		r.line("\traw := int64(value)")
		r.renderDurationFromMicroseconds(goType, "raw")
	case kindPointer:
		r.line("\tif value == nil { return nil, nil }")
		inner := cstStorageFor(typ.Elem)
		if inner.Scalar {
			r.line("\tdecoded, err := fgbCstDecode%d(*value, path)", typ.Elem.ID)
		} else {
			r.line("\tdecoded, err := fgbCstDecode%d(value, path)", typ.Elem.ID)
		}
		r.line("\tif err != nil { return nil, err }")
		if typ.Elem.usesPointerCodec(map[int]bool{}) {
			r.line("\treturn decoded, nil")
		} else {
			r.line("\treturn &decoded, nil")
		}
	case kindBytes:
		r.renderCstByteListDecoder(typ, "[]byte")
	case kindInt32List:
		r.renderCstTypedListDecoder(typ, "C.int32_t", "int32", "[]int32")
	case kindInt64List:
		r.renderCstTypedListDecoder(typ, "C.int64_t", "int64", "[]int64")
	case kindFloat64List:
		r.renderCstTypedListDecoder(typ, "C.double", "float64", "[]float64")
	case kindSlice:
		r.renderCstSliceDecoder(typ, false)
	case kindArray:
		r.renderCstSliceDecoder(typ, true)
	case kindStruct:
		if typ.usesPointerCodec(map[int]bool{}) {
			r.raw("\tif value == nil { return nil, fmt.Errorf(\"%s: null struct\", path) }\n")
			r.line("\tresult := new(%s)", goType)
		} else {
			r.line("\tif value == nil { var zero %s; return zero, fmt.Errorf(\"%%s: null struct\", path) }", goType)
			r.line("\tvar result %s", goType)
		}
		for _, field := range typ.Struct.allFields() {
			if field.Type.Kind == kindAtomic {
				r.line("\tdecoded%s, err := fgbCstDecode%d(value.%s, path+%s)", field.GoName, field.Type.Atomic.Value.ID, field.CName, strconv.Quote("."+field.WireName))
				if typ.usesPointerCodec(map[int]bool{}) {
					r.line("\tif err != nil { return nil, err }")
				} else {
					r.line("\tif err != nil { var zero %s; return zero, err }", goType)
				}
				r.line("\tresult.%s.Store(decoded%s)", field.GoName, field.GoName)
				continue
			}
			r.line("\tdecoded%s, err := fgbCstDecode%d(value.%s, path+%s)", field.GoName, field.Type.ID, field.CName, strconv.Quote("."+field.WireName))
			if typ.usesPointerCodec(map[int]bool{}) {
				r.line("\tif err != nil { return nil, err }")
			} else {
				r.line("\tif err != nil { var zero %s; return zero, err }", goType)
			}
			r.line("\tresult.%s = decoded%s", field.GoName, field.GoName)
		}
		r.line("\treturn result, nil")
	case kindOpaque:
		r.line("\tif value == 0 { return nil, nil }")
		r.line("\thandle := uintptr(value)")
		r.line("\traw, ok := fgbLoadOpaque(handle)")
		r.raw("\tif !ok { return nil, fmt.Errorf(\"%s: unknown or released handle %d\", path, handle) }")
		if typ.Opaque.Synthetic && !isPointerType(typ.Original) {
			r.line("\tboxed, ok := raw.(*%s)", goType)
			r.raw("\tif !ok { return nil, fmt.Errorf(\"%s: handle %d has incompatible Go type %T\", path, handle, raw) }")
			r.line("\treturn *boxed, nil")
			break
		}
		r.line("\tresult, ok := raw.(%s)", goType)
		r.raw("\tif !ok { return nil, fmt.Errorf(\"%s: handle %d has incompatible Go type %T\", path, handle, raw) }")
		r.line("\treturn result, nil")
	case kindDartOpaque:
		r.line("\tif value == 0 { var zero %s; return zero, fmt.Errorf(\"%%s: invalid DartOpaque handle 0\", path) }", goType)
		r.line("\tgeneration := fgbCurrentDartOpaqueGeneration()")
		r.line("\treturn fgbrt.NewDartOpaque(int64(value), func(handle int64) { fgbReleaseDartOpaque(generation, handle) }), nil")
	case kindCallback:
		r.line("\treturn fgbMakeCallback%d(context.Background(), int64(value)), nil", typ.ID)
	case kindStreamSink:
		if typ.ChannelStream {
			r.line("\tvar zero %s", goType)
			r.raw("\treturn zero, fmt.Errorf(\"%s: channel streams are only supported as direct call parameters\", path)\n")
			break
		}
		r.line("\tif value == 0 { var zero %s; return zero, fmt.Errorf(\"%%s: invalid stream handle 0\", path) }", goType)
		r.line("\treturn fgbMakeStreamSink%d(int64(value)), nil", typ.ID)
	case kindNamed:
		r.line("\tdecoded, err := fgbCstDecode%d(value, path)", typ.Named.Underlying.ID)
		r.line("\tif err != nil { var zero %s; return zero, err }", goType)
		r.line("\treturn %s(decoded), nil", goType)
	case kindAtomic:
		r.line("\tdecoded, err := fgbCstDecode%d(value, path)", typ.Atomic.Value.ID)
		r.line("\tif err != nil { return nil, err }")
		r.line("\tresult := new(%s)", goType)
		r.line("\tresult.Store(decoded)")
		r.line("\treturn result, nil")
	}
	r.line("}")
}

func (r *goRenderer) renderUnsignedRangeCheckFromUint64(typ *wireType, goType, expression string) {
	switch typ.BasicKind {
	case types.Uint8:
		r.line("\tif %s > 255 { var zero %s; return zero, fmt.Errorf(\"%%s: integer out of uint8 range\", path) }", expression, goType)
	case types.Uint16:
		r.line("\tif %s > 65535 { var zero %s; return zero, fmt.Errorf(\"%%s: integer out of uint16 range\", path) }", expression, goType)
	case types.Uint32:
		r.line("\tif %s > 4294967295 { var zero %s; return zero, fmt.Errorf(\"%%s: integer out of uint32 range\", path) }", expression, goType)
	case types.Uint, types.Uintptr:
		r.line("\tif uint64(%s(%s)) != %s { var zero %s; return zero, fmt.Errorf(\"%%s: integer out of %s range\", path) }", goType, expression, expression, goType, goType)
	}
}

func (r *goRenderer) renderCstByteListDecoder(typ *wireType, resultType string) {
	goType := r.goType(typ.Original)
	r.line("\tif value == nil { return nil, nil }")
	r.line("\traw, err := fgbCstReadBytes(unsafe.Pointer(value.ptr), value.len, path)")
	r.line("\tif err != nil { var zero %s; return zero, err }", goType)
	r.line("\treturn %s(raw), nil", resultType)
}

func (r *goRenderer) renderCstTypedListDecoder(typ *wireType, cElement, goElement, resultType string) {
	goType := r.goType(typ.Original)
	r.line("\tif value == nil { return nil, nil }")
	r.line("\tlength, err := fgbCstLength(value.len, path)")
	r.line("\tif err != nil { var zero %s; return zero, err }", goType)
	r.line("\tif length != 0 && value.ptr == nil { var zero %s; return zero, fmt.Errorf(\"%%s: nil data pointer\", path) }", goType)
	r.line("\traw := unsafe.Slice((*%s)(unsafe.Pointer(value.ptr)), length)", cElement)
	r.line("\tresult := make(%s, length)", resultType)
	r.line("\tfor index, item := range raw { result[index] = %s(item) }", goElement)
	r.line("\treturn result, nil")
}

func (r *goRenderer) renderCstSliceDecoder(typ *wireType, array bool) {
	goType := r.goType(typ.Original)
	if array {
		// A Go array is a fixed-size value and can never be nil.
		r.line("\tif value == nil { var zero %s; return zero, fmt.Errorf(\"%%s: null list\", path) }", goType)
	} else {
		r.line("\tif value == nil { return nil, nil }")
	}
	r.line("\tlength, err := fgbCstLength(value.len, path)")
	r.line("\tif err != nil { var zero %s; return zero, err }", goType)
	if array {
		r.line("\tif length != %d { var zero %s; return zero, fmt.Errorf(\"%%s: expected %d elements, got %%d\", path, length) }", typ.Length, goType, typ.Length)
	} else {
		r.line("\tif length != 0 && value.ptr == nil { var zero %s; return zero, fmt.Errorf(\"%%s: nil data pointer\", path) }", goType)
	}
	r.line("\traw := unsafe.Slice(value.ptr, length)")
	if array {
		r.line("\tvar result %s", goType)
	} else {
		r.line("\tresult := make(%s, length)", goType)
	}
	r.line("\tfor index, item := range raw {")
	r.line("\t\tdecoded, err := fgbCstDecode%d(item, fmt.Sprintf(\"%%s[%%d]\", path, index))", typ.Elem.ID)
	if array {
		r.line("\t\tif err != nil { var zero %s; return zero, err }", goType)
	} else {
		r.line("\t\tif err != nil { return nil, err }")
	}
	r.line("\t\tresult[index] = decoded")
	r.line("\t}")
	r.line("\treturn result, nil")
}

func (r *goRenderer) renderDcoEncoders() {
	for _, typ := range cstDcoReachableTypes(r.unit) {
		r.renderDcoEncoder(typ)
		r.line("")
	}
}

func (r *goRenderer) renderDcoEncoder(typ *wireType) {
	goType := r.goType(typ.Original)
	encodeType := goType
	if typ.usesPointerCodec(map[int]bool{}) {
		encodeType = "*" + goType
	}
	r.line("func fgbDcoEncode%d(value %s, depth int, transfer *fgbOpaqueTransfer) (*C.FgbDartCObject, error) {", typ.ID, encodeType)
	r.line("\tif depth > 64 { return nil, fmt.Errorf(\"value nesting exceeds 64 levels (cyclic reference?)\") }")
	switch typ.Kind {
	case kindBool:
		r.line("\treturn fgbDcoBool(value)")
	case kindString:
		r.line("\treturn fgbDcoString(value)")
	case kindError:
		r.line("\tif value == nil { return fgbDcoNull() }")
		r.line("\treturn fgbDcoString(value.Error())")
	case kindSigned:
		if typ.BitSize <= 32 && typ.BasicKind != types.Int {
			r.line("\treturn fgbDcoInt32(int32(value))")
		} else {
			r.line("\treturn fgbDcoInt64(int64(value))")
		}
	case kindUnsigned:
		if isDartBigIntType(typ) {
			r.line("\treturn fgbDcoString(new(big.Int).SetUint64(uint64(value)).Text(16))")
		} else if typ.BitSize <= 16 {
			// The slot is chosen by range, not by width: uint32 does not fit in
			// a signed 32-bit slot, and narrowing it there would deliver
			// 4294967295 to Dart as -1. Matches the standard codec.
			r.line("\treturn fgbDcoInt32(int32(value))")
		} else {
			r.line("\treturn fgbDcoInt64(int64(value))")
		}
	case kindFloat:
		r.line("\treturn fgbDcoDouble(float64(value))")
	case kindBigInt:
		_, pointer := types.Unalias(typ.Original).(*types.Pointer)
		if pointer {
			r.line("\tif value == nil { return fgbDcoNull() }")
			r.line("\treturn fgbDcoString(value.Text(16))")
		} else {
			r.line("\treturn fgbDcoString(value.Text(16))")
		}
	case kindTime:
		r.line("\tmicros, err := fgbTimeUnixMicro(value)")
		r.line("\tif err != nil { return nil, err }")
		r.line("\treturn fgbDcoInt64(micros)")
	case kindInternetIP:
		r.line("\treturn fgbDcoString(value.String())")
	case kindIPPrefix:
		r.line("\tif !value.IsValid() { return fgbDcoString(\"\") }")
		r.line("\treturn fgbDcoString(value.String())")
	case kindUUID:
		r.line("\treturn fgbDcoString(value.String())")
	case kindURL:
		r.line("\treturn fgbDcoString(value.String())")
	case kindDuration:
		r.line("\treturn fgbDcoInt64(int64(value / time.Microsecond))")
	case kindPointer:
		r.line("\tif value == nil { return fgbDcoNull() }")
		if typ.Elem.usesPointerCodec(map[int]bool{}) {
			r.line("\treturn fgbDcoEncode%d(value, depth+1, transfer)", typ.Elem.ID)
		} else {
			r.line("\treturn fgbDcoEncode%d(*value, depth+1, transfer)", typ.Elem.ID)
		}
	case kindBytes:
		r.line("\treturn fgbDcoBytes(value)")
	case kindInt32List:
		r.line("\treturn fgbDcoInt32List(value)")
	case kindInt64List:
		r.line("\treturn fgbDcoInt64List(value)")
	case kindFloat64List:
		r.line("\treturn fgbDcoFloat64List(value)")
	case kindSlice, kindArray:
		r.renderDcoArrayEncoder(typ)
	case kindStruct:
		if typ.usesPointerCodec(map[int]bool{}) {
			r.line("\tif value == nil { return fgbDcoNull() }")
		}
		r.renderDcoStructEncoder(typ)
	case kindOpaque:
		r.line("\tif value == nil { return fgbDcoNull() }")
		if typ.Opaque.Synthetic && !isPointerType(typ.Original) {
			r.line("\tboxed := new(%s)", goType)
			r.line("\t*boxed = value")
			r.line("\thandle, err := fgbStoreOpaque(boxed, transfer)")
		} else {
			r.line("\thandle, err := fgbStoreOpaque(value, transfer)")
		}
		r.line("\tif err != nil || uint64(handle) > uint64(^uint64(0)>>1) { return nil, fmt.Errorf(\"opaque handle space exhausted\") }")
		r.line("\treturn fgbDcoInt64(int64(handle))")
	case kindDartOpaque:
		r.line("\tif !value.IsValid() { return nil, fmt.Errorf(\"cannot encode an invalid DartOpaque\") }")
		r.line("\treturn fgbDcoInt64(value.Handle())")
	case kindCallback:
		r.line("\treturn nil, fmt.Errorf(\"function values cannot be encoded\")")
	case kindStreamSink:
		r.line("\treturn nil, fmt.Errorf(\"stream sinks cannot be sent to Dart\")")
	case kindNamed:
		r.line("return fgbDcoEncode%d(%s(value), depth+1, transfer)", typ.Named.Underlying.ID, r.goType(typ.Named.Underlying.Original))
	case kindAtomic:
		r.line("if value == nil { return fgbDcoNull() }")
		r.line("return fgbDcoEncode%d(value.Load(), depth+1, transfer)", typ.Atomic.Value.ID)
	default:
		r.line("return nil, fmt.Errorf(\"DCO does not support %s\")", typ.Kind)
	}
	r.line("}")
}

func (r *goRenderer) renderDcoArrayEncoder(typ *wireType) {
	goType := r.goType(typ.Original)
	r.line("\tresult := C.fgb_dco_array_new(C.int64_t(len(value)))")
	r.line("\tif result == nil { return nil, fmt.Errorf(\"DCO array allocation failed\") }")
	r.line("\tfor index, item := range value {")
	r.line("\t\tchild, err := fgbDcoEncode%d(item, depth+1, transfer)", typ.Elem.ID)
	r.raw("\t\tif err != nil { C.fgb_internal_dco_free(result); return nil, fmt.Errorf(\"element %d: %w\", index, err) }")
	r.line("\t\tC.fgb_dco_array_set(result, C.int64_t(index), child)")
	r.line("\t}")
	_ = goType
	r.line("\treturn result, nil")
}

func (r *goRenderer) renderDcoStructEncoder(typ *wireType) {
	fields := typ.Struct.allFields()
	r.line("\tresult := C.fgb_dco_array_new(%d)", len(fields))
	r.line("\tif result == nil { return nil, fmt.Errorf(\"DCO struct allocation failed\") }")
	for index, field := range fields {
		encodeID := field.Type.ID
		encodeExpr := fmt.Sprintf("value.%s", field.GoName)
		if field.Type.Kind == kindAtomic {
			encodeID = field.Type.Atomic.Value.ID
			encodeExpr = fmt.Sprintf("value.%s.Load()", field.GoName)
		}
		if field.Nullable {
			// Keep nil distinct from empty for a field marked fgb:"nullable".
			r.line("\tvar child%d *C.FgbDartCObject", index)
			r.line("\tif value.%s == nil {", field.GoName)
			r.line("\t\tencoded, err := fgbDcoNull()")
			r.line("\t\tif err != nil { C.fgb_internal_dco_free(result); return nil, err }")
			r.line("\t\tchild%d = encoded", index)
			r.line("\t} else {")
			r.line("\t\tencoded, err := fgbDcoEncode%d(value.%s, depth+1, transfer)", field.Type.ID, field.GoName)
			r.line("\t\tif err != nil { C.fgb_internal_dco_free(result); return nil, fmt.Errorf(%s+\": %%w\", err) }", strconv.Quote(field.WireName))
			r.line("\t\tchild%d = encoded", index)
			r.line("\t}")
			r.line("\tC.fgb_dco_array_set(result, %d, child%d)", index, index)
			continue
		}
		r.line("\tchild%d, err := fgbDcoEncode%d(%s, depth+1, transfer)", index, encodeID, encodeExpr)
		r.line("\tif err != nil { C.fgb_internal_dco_free(result); return nil, fmt.Errorf(%s+\": %%w\", err) }", strconv.Quote(field.WireName))
		r.line("\tC.fgb_dco_array_set(result, %d, child%d)", index, index)
	}
	r.line("\treturn result, nil")
}

func (r *goRenderer) renderCstDispatch() {
	r.line("func fgbDispatchCst(callID int32, raw unsafe.Pointer, transfer *fgbOpaqueTransfer) (*C.FgbDartCObject, *fgbCallError) {")
	r.line("\tswitch callID {")
	for _, call := range r.unit.Calls {
		if !call.usesCstDco() {
			continue
		}
		r.line("\tcase %d:", call.ID)
		r.renderCallContextSetup(call, "\t\t")
		r.line("\t\tif raw == nil { return nil, fgbInvalidArguments(%s, fmt.Errorf(\"null CST arguments\")) }", strconv.Quote(call.WireName))
		if call.Receiver != nil || len(call.Params) != 0 {
			r.line("\t\targs := (*C.%s)(raw)", cstArgsName(call))
		}
		argOffset := 0
		if call.Receiver != nil {
			r.line("\t\treceiver, err := fgbCstDecode%d(args.receiver, \"receiver\")", call.Receiver.ID)
			r.line("\t\tif err != nil { return nil, fgbInvalidArguments(%s, err) }", strconv.Quote(call.WireName))
			argOffset++
		}
		for index, param := range call.Params {
			if param.Type.Kind == kindStreamSink && param.Type.ChannelStream {
				continue
			}
			if param.Type.Kind == kindCallback {
				r.line("\t\targ%d := fgbMakeCallback%d(fgbCtx, int64(args.%s))", index, param.Type.ID, param.CName)
				continue
			} else {
				r.line("\t\targ%d, err := fgbCstDecode%d(args.%s, %s)", index, param.Type.ID, param.CName, strconv.Quote(param.DartName))
			}
			r.line("\t\tif err != nil { return nil, fgbInvalidArguments(%s, err) }", strconv.Quote(call.WireName))
		}
		r.renderStreamChannelSetup(call, "\t\t", func(index int) string {
			return fmt.Sprintf("int64(args.%s)", call.Params[index].CName)
		})
		_ = argOffset
		r.renderGoCallStatement(call, "\t\t", strconv.Quote(call.WireName))
		encodeError := fmt.Sprintf("map[any]any{\"method\": %q}", call.WireName)
		switch len(call.Results) {
		case 0:
			r.line("\t\treturn nil, nil")
		case 1:
			r.line("\t\tencoded, err := fgbDcoEncode%d(result0, 0, transfer)", call.Results[0].Type.ID)
			r.line("\t\tif err != nil { return nil, &fgbCallError{Code: \"encode_error\", Message: err.Error(), Details: %s} }", encodeError)
			r.line("\t\treturn encoded, nil")
		default:
			// Several results become one DCO array, which Dart rebuilds into a
			// record.
			r.line("\t\tencoded := C.fgb_dco_array_new(%d)", len(call.Results))
			r.line("\t\tif encoded == nil { return nil, &fgbCallError{Code: \"encode_error\", Message: \"DCO array allocation failed\", Details: %s} }", encodeError)
			for index, result := range call.Results {
				r.line("\t\tchild%d, err := fgbDcoEncode%d(result%d, 0, transfer)", index, result.Type.ID, index)
				r.line("\t\tif err != nil { C.fgb_internal_dco_free(encoded); return nil, &fgbCallError{Code: \"encode_error\", Message: err.Error(), Details: %s} }", encodeError)
				r.line("\t\tC.fgb_dco_array_set(encoded, %d, child%d)", index, index)
			}
			r.line("\t\treturn encoded, nil")
		}
	}
	r.line("\tdefault:")
	r.raw("\t\treturn nil, &fgbCallError{Code: \"method_not_found\", Message: fmt.Sprintf(\"unknown CST call id %d\", callID)}")
	r.line("\t}")
	r.line("}")
}
