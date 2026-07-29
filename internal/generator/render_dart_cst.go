package generator

import (
	"strconv"
	"strings"
)

// renderCstEncoders emits the Dart half of the CST wire format.  These
// functions deliberately return native FFI values instead of API values:
// the only place that knows about this representation is bridge_generated.dart
// (the per-source Dart files remain ordinary Dart APIs).
func (r *splitDartRenderer) renderCstEncoders() {
	for _, typ := range r.unit.Types {
		if !typ.supportsCodec(codecModeCST, map[int]bool{}) {
			continue
		}
		if cstStorageFor(typ).DartType == "" {
			continue
		}
		r.renderCstEncoder(typ)
		r.line("")
	}
}

func (r *splitDartRenderer) renderCstEncoder(typ *wireType) {
	r.line("%s fgbCstEncode%d(%s value, _FgbArena arena, String path) {", cstDartEncoderReturnType(typ), typ.ID, dartEncoderValueType(typ))
	// Nilable compound values (slices and the typed lists) travel as arena
	// pointers, so a null value is simply a null pointer. Closures encode
	// their own null case as handle 0.
	if typ.Kind != kindCallback && typ.nilableWithoutPointer() {
		r.line("  if (value == null) return ffi.nullptr;")
	}
	switch typ.Kind {
	case kindBool:
		r.line("  return value ? 1 : 0;")
	case kindString:
		r.renderCstStringBody("value")
	case kindSigned, kindFloat:
		r.line("  return value;")
	case kindDuration:
		r.line("  return value.inMicroseconds;")
	case kindUnsigned:
		if isDartBigIntType(typ) {
			r.renderCstStringBody("value.toRadixString(16)")
		} else {
			r.line("  return value;")
		}
	case kindBigInt:
		if strings.HasSuffix(typ.DartType, "?") {
			r.line("  if (value == null) return ffi.nullptr;")
		}
		r.renderCstStringBody("value.toRadixString(16)")
	case kindTime:
		r.renderCstStringBody("value.toIso8601String()")
	case kindInternetIP:
		r.renderCstStringBody("value.address")
	case kindUUID:
		r.renderCstStringBody("value.uuid")
	case kindPointer:
		r.line("  if (value == null) return ffi.nullptr;")
		inner := cstStorageFor(typ.Elem)
		if inner.Scalar {
			r.line("  final result = arena.allocate<%s>(ffi.sizeOf<%s>());", inner.DartType, inner.DartType)
			r.line("  result.value = fgbCstEncode%d(value, arena, path);", typ.Elem.ID)
			r.line("  return result;")
		} else {
			r.line("  return fgbCstEncode%d(value, arena, path);", typ.Elem.ID)
		}
	case kindBytes:
		r.renderCstTypedListBody(typ, "ffi.Uint8")
	case kindInt32List:
		r.renderCstTypedListBody(typ, "ffi.Int32")
	case kindInt64List:
		r.renderCstTypedListBody(typ, "ffi.Int64")
	case kindFloat64List:
		r.renderCstTypedListBody(typ, "ffi.Double")
	case kindSlice, kindArray:
		r.renderCstGenericList(typ)
	case kindStruct:
		r.line("  final result = arena.allocate<%s>(ffi.sizeOf<%s>());", cstDartTypeName(typ), cstDartTypeName(typ))
		for _, field := range typ.Struct.allFields() {
			r.line("  result.ref.%s = fgbCstEncode%d(value.%s, arena, path + %s);", field.CName, field.Type.ID, field.DartName, strconv.Quote("."+field.WireName))
		}
		r.line("  return result;")
	case kindOpaque:
		r.line("  if (value == null) return 0;")
		r.line("  if (!identical(value.fgbBridge, arena.bridge)) throw StateError('opaque value belongs to a different bridge');")
		r.line("  return value.fgbHandle;")
	case kindDartOpaque:
		r.line("  return arena.bridge.fgbInternalRegisterDartOpaque(value);")
	case kindStreamSink:
		r.renderStreamSinkRegistration(typ, "arena.bridge")
	case kindCallback:
		r.renderCallbackRegistration(typ, "arena.bridge")
	case kindNamed:
		r.line("  return fgbCstEncode%d(value.value, arena, path);", typ.Named.Underlying.ID)
	case kindAtomic:
		r.line("  return fgbCstEncode%d(value, arena, path);", typ.Atomic.Value.ID)
	default:
		// Unsupported types are intentionally omitted by renderCstEncoders.
		r.line("  throw UnsupportedError('CST does not support %s');", typ.Kind)
	}
	r.line("}")
}

// cstDartEncoderReturnType is the Dart expression type produced by an
// encoder.  Scalar CST fields are represented by native ffi field types, but
// assignments still use ordinary Dart values (int/double); compound values
// are native pointers.
func cstDartEncoderReturnType(typ *wireType) string {
	if typ == nil {
		return "Object?"
	}
	if typ.Kind == kindNamed && typ.Named != nil {
		return cstDartEncoderReturnType(typ.Named.Underlying)
	}
	if typ.Kind == kindAtomic && typ.Atomic != nil {
		return cstDartEncoderReturnType(typ.Atomic.Value)
	}
	storage := cstStorageFor(typ)
	if !storage.Scalar {
		return storage.DartType
	}
	switch typ.Kind {
	case kindBool, kindSigned, kindUnsigned, kindDuration, kindOpaque, kindDartOpaque, kindCallback, kindStreamSink:
		return "int"
	case kindFloat:
		return "double"
	default:
		return storage.DartType
	}
}

func isDartBigIntType(typ *wireType) bool {
	return typ != nil && strings.TrimSuffix(typ.DartType, "?") == "BigInt"
}

func (r *splitDartRenderer) renderCstStringBody(expression string) {
	r.line("  final raw = utf8.encode(%s);", expression)
	r.line("  final result = arena.allocate<_FgbCstBytes>(ffi.sizeOf<_FgbCstBytes>());")
	r.line("  result.ref.ptr = arena.bytes(raw);")
	r.line("  result.ref.len = raw.length;")
	r.line("  return result;")
}

func (r *splitDartRenderer) renderCstGenericList(typ *wireType) {
	r.line("  final result = arena.allocate<%s>(ffi.sizeOf<%s>());", cstDartTypeName(typ), cstDartTypeName(typ))
	r.line("  final length = value.length;")
	r.line("  result.ref.len = length;")
	elem := cstStorageFor(typ.Elem)
	r.line("  final data = arena.allocate<%s>(ffi.sizeOf<%s>() * (length == 0 ? 1 : length));", elem.DartType, elem.DartType)
	r.line("  result.ref.ptr = data;")
	r.line("  for (var index = 0; index < length; index++) {")
	r.line("    (data + index).value = fgbCstEncode%d(value[index], arena, '$path[' + index.toString() + ']');", typ.Elem.ID)
	r.line("  }")
	r.line("  return result;")
}

// renderCstTypedListBody emits the four Dart typed-list descriptors.  It is
// called from renderCstEncoder with the concrete wire type so no guessed type
// names can leak into generated code.
func (r *splitDartRenderer) renderCstTypedListBody(typ *wireType, nativeElement string) {
	r.line("  final result = arena.allocate<%s>(ffi.sizeOf<%s>());", cstDartTypeName(typ), cstDartTypeName(typ))
	r.line("  result.ref.len = value.length;")
	r.line("  final data = arena.allocate<%s>(ffi.sizeOf<%s>() * (value.isEmpty ? 1 : value.length));", nativeElement, nativeElement)
	r.line("  result.ref.ptr = data;")
	r.line("  if (value.isNotEmpty) data.asTypedList(value.length).setAll(0, value);")
	r.line("  return result;")
}

// renderCentralCstCall is the sole generated bridge entry point for a
// CST+DCO call.  It owns the temporary C arena for exactly the duration of
// the native invocation and result conversion.
func (r *splitDartRenderer) renderCentralCstCall(call *callModel, arguments []string) {
	r.line("  final arena = _FgbArena(this);")
	r.line("  try {")
	r.line("    final args = arena.allocate<%s>(ffi.sizeOf<%s>());", cstDartArgsName(call), cstDartArgsName(call))
	argIndex := 0
	if call.Receiver != nil {
		r.line("    args.ref.receiver = fgbCstEncode%d(%s, arena, 'receiver');", call.Receiver.ID, arguments[argIndex])
		argIndex++
	}
	for _, param := range call.Params {
		r.line("    args.ref.%s = fgbCstEncode%d(%s, arena, %s);", param.CName, param.Type.ID, arguments[argIndex], strconv.Quote(param.DartName))
		argIndex++
	}
	if len(call.Results) != 0 {
		if isAsyncCall(call) {
			r.line("    final wireResult = await fgbInvokeCstAsync(%d, args.cast<ffi.Void>());", call.ID)
		} else {
			r.line("    final wireResult = fgbInvokeCstSync(%d, args.cast<ffi.Void>());", call.ID)
		}
		for _, line := range dartResultDecodeLines(call, "wireResult", "    ") {
			r.line("%s", line)
		}
	} else if isAsyncCall(call) {
		r.line("    await fgbInvokeCstAsync(%d, args.cast<ffi.Void>());", call.ID)
		r.line("    return;")
	} else {
		r.line("    fgbInvokeCstSync(%d, args.cast<ffi.Void>());", call.ID)
		r.line("    return;")
	}
	r.line("  } finally {")
	r.line("    arena.close();")
	r.line("  }")
}
