package generator

import (
	"fmt"
	"strconv"
	"strings"
)

// cstStorage describes the field representation used by the generated C
// structs. Scalars stay inline; all compound values are pointers into a
// short-lived arena owned by the Dart call wrapper. This mirrors FRB's CST
// wire structs while keeping ownership explicit and easy to audit.
type cstStorage struct {
	CType       string
	DartType    string
	DartField   string
	Scalar      bool
	Bool        bool
	Pointer     bool
	Definition  string
	ElementType string
}

func cstTypeName(typ *wireType) string {
	return fmt.Sprintf("FgbCstType%d", typ.ID)
}

func cstDartTypeName(typ *wireType) string {
	return fmt.Sprintf("_FgbCstType%d", typ.ID)
}

func cstArgsName(call *callModel) string {
	return fmt.Sprintf("FgbCstArgs%d", call.ID)
}

func cstDartArgsName(call *callModel) string {
	return fmt.Sprintf("_FgbCstArgs%d", call.ID)
}

func cstIsScalar(typ *wireType) bool {
	if typ == nil {
		return false
	}
	switch typ.Kind {
	case kindBool, kindSigned, kindUnsigned, kindFloat, kindDuration, kindOpaque, kindDartOpaque, kindCallback, kindStreamSink:
		return true
	case kindNamed:
		return cstIsScalar(typ.Named.Underlying)
	case kindAtomic:
		return cstIsScalar(typ.Atomic.Value)
	default:
		return false
	}
}

func cstScalarBase(typ *wireType) *wireType {
	for typ != nil && (typ.Kind == kindNamed || typ.Kind == kindAtomic) {
		if typ.Kind == kindNamed {
			typ = typ.Named.Underlying
		} else {
			typ = typ.Atomic.Value
		}
	}
	return typ
}

func cstStorageFor(typ *wireType) cstStorage {
	base := cstScalarBase(typ)
	if base == nil {
		return cstStorage{}
	}
	switch base.Kind {
	case kindBool:
		return cstStorage{CType: "uint8_t", DartType: "ffi.Uint8", DartField: "@ffi.Uint8() external int", Scalar: true, Bool: true}
	case kindSigned:
		return cstStorage{CType: "int64_t", DartType: "ffi.Int64", DartField: "@ffi.Int64() external int", Scalar: true}
	case kindUnsigned:
		if strings.TrimSuffix(base.DartType, "?") == "BigInt" {
			return cstStorage{CType: "FgbCstBytes*", DartType: "ffi.Pointer<_FgbCstBytes>", Pointer: true}
		}
		return cstStorage{CType: "uint64_t", DartType: "ffi.Uint64", DartField: "@ffi.Uint64() external int", Scalar: true}
	case kindFloat:
		return cstStorage{CType: "double", DartType: "ffi.Double", DartField: "@ffi.Double() external double", Scalar: true}
	case kindDuration:
		return cstStorage{CType: "int64_t", DartType: "ffi.Int64", DartField: "@ffi.Int64() external int", Scalar: true}
	case kindOpaque:
		return cstStorage{CType: "uintptr_t", DartType: "ffi.UintPtr", DartField: "@ffi.UintPtr() external int", Scalar: true}
	case kindDartOpaque, kindCallback, kindStreamSink:
		return cstStorage{CType: "int64_t", DartType: "ffi.Int64", DartField: "@ffi.Int64() external int", Scalar: true}
	case kindString, kindTime, kindBigInt, kindInternetIP, kindIPPrefix, kindURL, kindUUID:
		return cstStorage{CType: "FgbCstBytes*", DartType: "ffi.Pointer<_FgbCstBytes>", Pointer: true}
	case kindBytes:
		return cstStorage{CType: cstTypeName(base) + "*", DartType: "ffi.Pointer<" + cstDartTypeName(base) + ">", Pointer: true}
	case kindInt32List, kindInt64List, kindFloat64List, kindSlice, kindArray, kindStruct:
		return cstStorage{CType: cstTypeName(base) + "*", DartType: "ffi.Pointer<" + cstDartTypeName(base) + ">", Pointer: true}
	case kindPointer:
		inner := cstStorageFor(base.Elem)
		if inner.Scalar {
			inner.CType += "*"
			inner.DartType = "ffi.Pointer<" + inner.DartType + ">"
			inner.DartField = ""
			inner.Scalar = false
			inner.Pointer = true
		}
		return inner
	case kindMap, kindAny:
		return cstStorage{}
	default:
		return cstStorage{}
	}
}

func cstDefinitionNeeded(typ *wireType) bool {
	base := cstScalarBase(typ)
	if base == nil {
		return false
	}
	switch base.Kind {
	case kindBytes, kindInt32List, kindInt64List, kindFloat64List, kindSlice, kindArray, kindStruct:
		return true
	default:
		return false
	}
}

func cstElementStorage(typ *wireType) cstStorage {
	return cstStorageFor(typ)
}

func cstCTypeForSignature(typ *wireType) string {
	return cstStorageFor(typ).CType
}

func cstDartTypeForSignature(typ *wireType) string {
	return cstStorageFor(typ).DartType
}

// cstGoTypeForSignature is the cgo spelling of a CST field type.  Keeping
// this mapping next to cstStorageFor is important: the generated C preamble
// and the generated Go decoder must agree byte-for-byte on every field.
func cstGoTypeForSignature(typ *wireType) string {
	storage := cstStorageFor(typ)
	if storage.CType == "" {
		return "unsafe.Pointer"
	}
	base := storage.CType
	stars := ""
	for strings.HasSuffix(base, "*") {
		base = strings.TrimSpace(strings.TrimSuffix(base, "*"))
		stars += "*"
	}
	var goBase string
	switch base {
	case "uint8_t":
		goBase = "C.uint8_t"
	case "int8_t":
		goBase = "C.int8_t"
	case "uint16_t":
		goBase = "C.uint16_t"
	case "int16_t":
		goBase = "C.int16_t"
	case "uint32_t":
		goBase = "C.uint32_t"
	case "int32_t":
		goBase = "C.int32_t"
	case "uint64_t":
		goBase = "C.uint64_t"
	case "int64_t":
		goBase = "C.int64_t"
	case "uintptr_t":
		goBase = "C.uintptr_t"
	case "double":
		goBase = "C.double"
	case "FgbCstBytes":
		goBase = "C.FgbCstBytes"
	default:
		// All generated compound types are named FgbCstTypeN.  Avoid
		// accepting arbitrary C text here; this is generator-owned input.
		if strings.HasPrefix(base, "FgbCstType") {
			goBase = "C." + base
		} else {
			return "unsafe.Pointer"
		}
	}
	return stars + goBase
}

func cstCFieldLines(name string, typ *wireType) []string {
	storage := cstStorageFor(typ)
	if storage.CType == "" {
		return nil
	}
	return []string{fmt.Sprintf("  %s %s;", storage.CType, name)}
}

func cstDartFieldLines(name string, typ *wireType) []string {
	storage := cstStorageFor(typ)
	if storage.DartType == "" {
		return nil
	}
	if storage.Scalar {
		return []string{fmt.Sprintf("  %s %s;", storage.DartField, name)}
	}
	return []string{fmt.Sprintf("  external %s %s;", storage.DartType, name)}
}

func cstCDefinition(typ *wireType) string {
	base := cstScalarBase(typ)
	if base == nil || !cstDefinitionNeeded(base) {
		return ""
	}
	name := cstTypeName(base)
	var lines []string
	switch base.Kind {
	case kindBytes:
		lines = []string{"uint8_t* ptr;", "int64_t len;"}
	case kindInt32List:
		lines = []string{"int32_t* ptr;", "int64_t len;"}
	case kindInt64List:
		lines = []string{"int64_t* ptr;", "int64_t len;"}
	case kindFloat64List:
		lines = []string{"double* ptr;", "int64_t len;"}
	case kindSlice, kindArray:
		storage := cstElementStorage(base.Elem)
		if storage.CType == "" {
			return ""
		}
		lines = []string{fmt.Sprintf("%s* ptr;", storage.CType), "int64_t len;"}
	case kindStruct:
		for _, field := range base.Struct.allFields() {
			lines = append(lines, cstCFieldLines(field.CName, field.Type)...)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "struct %s {\n", name)
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("};\n")
	return b.String()
}

func cstDartDefinition(typ *wireType) string {
	base := cstScalarBase(typ)
	if base == nil || !cstDefinitionNeeded(base) {
		return ""
	}
	name := cstDartTypeName(base)
	var lines []string
	switch base.Kind {
	case kindBytes:
		lines = []string{"  external ffi.Pointer<ffi.Uint8> ptr;", "  @ffi.Int64() external int len;"}
	case kindInt32List:
		lines = []string{"  external ffi.Pointer<ffi.Int32> ptr;", "  @ffi.Int64() external int len;"}
	case kindInt64List:
		lines = []string{"  external ffi.Pointer<ffi.Int64> ptr;", "  @ffi.Int64() external int len;"}
	case kindFloat64List:
		lines = []string{"  external ffi.Pointer<ffi.Double> ptr;", "  @ffi.Int64() external int len;"}
	case kindSlice, kindArray:
		storage := cstElementStorage(base.Elem)
		if storage.DartType == "" {
			return ""
		}
		lines = []string{fmt.Sprintf("  external ffi.Pointer<%s> ptr;", storage.DartType), "  @ffi.Int64() external int len;"}
	case kindStruct:
		for _, field := range base.Struct.allFields() {
			lines = append(lines, cstDartFieldLines(field.CName, field.Type)...)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "final class %s extends ffi.Struct {\n", name)
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.String()
}

func cstCArgsDefinition(call *callModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "typedef struct %s %s;\n", cstArgsName(call), cstArgsName(call))
	fmt.Fprintf(&b, "struct %s {\n", cstArgsName(call))
	if call.Receiver != nil {
		fmt.Fprintf(&b, "  %s receiver;\n", cstCTypeForSignature(call.Receiver))
	}
	for _, param := range call.Params {
		fmt.Fprintf(&b, "  %s %s;\n", cstCTypeForSignature(param.Type), param.CName)
	}
	if call.Receiver == nil && len(call.Params) == 0 {
		// C and dart:ffi both reject empty structs; keep the layouts in sync.
		b.WriteString("  uint8_t fgbPad;\n")
	}
	b.WriteString("};\n")
	return b.String()
}

func cstDartArgsDefinition(call *callModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "final class %s extends ffi.Struct {\n", cstDartArgsName(call))
	if call.Receiver != nil {
		for _, line := range cstDartFieldLines("receiver", call.Receiver) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	for _, param := range call.Params {
		for _, line := range cstDartFieldLines(param.CName, param.Type) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if call.Receiver == nil && len(call.Params) == 0 {
		b.WriteString("  @ffi.Uint8() external int fgbPad;\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func cstForwardDeclarations(unit *unit) string {
	var b strings.Builder
	declared := map[int]bool{}
	for _, typ := range unit.Types {
		base := cstScalarBase(typ)
		if base != nil && cstDefinitionNeeded(base) && !declared[base.ID] {
			declared[base.ID] = true
			fmt.Fprintf(&b, "typedef struct %s %s;\n", cstTypeName(base), cstTypeName(base))
		}
	}
	for _, call := range unit.Calls {
		if call.usesCstDco() {
			b.WriteString(cstCArgsDefinition(call))
		}
	}
	defined := map[int]bool{}
	for _, typ := range unit.Types {
		base := cstScalarBase(typ)
		if base == nil || defined[base.ID] {
			continue
		}
		if definition := cstCDefinition(base); definition != "" {
			defined[base.ID] = true
			b.WriteString(definition)
		}
	}
	return b.String()
}

func cstDartDefinitions(unit *unit) string {
	var b strings.Builder
	b.WriteString("final class _FgbCstBytes extends ffi.Struct {\n  external ffi.Pointer<ffi.Uint8> ptr;\n  @ffi.Int64() external int len;\n}\n")
	defined := map[int]bool{}
	for _, typ := range unit.Types {
		base := cstScalarBase(typ)
		if base == nil || defined[base.ID] {
			continue
		}
		if definition := cstDartDefinition(base); definition != "" {
			defined[base.ID] = true
			b.WriteString(definition)
		}
	}
	for _, call := range unit.Calls {
		if call.usesCstDco() {
			b.WriteString(cstDartArgsDefinition(call))
		}
	}
	return b.String()
}

func cstDartStorageType(typ *wireType) string {
	return cstStorageFor(typ).DartType
}

func cstDartPointerElementType(typ *wireType) string {
	storage := cstStorageFor(typ)
	if strings.HasPrefix(storage.DartType, "ffi.Pointer<") && strings.HasSuffix(storage.DartType, ">") {
		return storage.DartType
	}
	return storage.DartType
}

func cstDartScalarAssignment(target, value string, typ *wireType) string {
	base := cstScalarBase(typ)
	if base != nil && base.Kind == kindBool {
		return fmt.Sprintf("%s = %s ? 1 : 0;", target, value)
	}
	return fmt.Sprintf("%s = %s;", target, value)
}

func cstDartStringLiteral(value string) string { return strconv.Quote(value) }
