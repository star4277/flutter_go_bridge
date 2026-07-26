package generator

import (
	"errors"
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	"github.com/star4277/flutter-go-bridge-gokit/internal/model"
	"github.com/star4277/flutter-go-bridge-gokit/internal/names"
)

type namedUsage uint8

const (
	usageUnknown namedUsage = iota
	usageValue
	usageOpaque
)

type builder struct {
	api          *model.API
	config       config.Resolved
	unit         *unit
	typeCache    map[types.Type]*wireType
	namedUsage   map[*types.Named]namedUsage
	namedModels  map[*types.Named]*namedModel
	structModels map[*types.Named]*structModel
	opaqueModels map[*types.Named]*opaqueModel
	warnings     []error
	nextTypeID   int
}

func buildUnit(api *model.API, resolved config.Resolved, direct bool) (*unit, []error, error) {
	needsMain := true
	if direct {
		if object := api.Package.Types.Scope().Lookup("main"); object != nil {
			if _, isFunction := object.(*types.Func); isFunction {
				needsMain = false
			} else {
				return nil, nil, fmt.Errorf("package main declares non-function identifier main; a c-shared bridge requires func main()")
			}
		}
	}
	result := &unit{
		PackagePath:  api.Package.PkgPath,
		PackageName:  api.Package.Name,
		InputDir:     api.InputDir,
		SourceFiles:  append([]string(nil), api.SourceFiles...),
		Direct:       direct,
		NeedsMain:    needsMain,
		LibraryName:  resolved.LibraryName,
		ClassName:    names.UpperCamel(resolved.DartEntrypointClassName),
		GoPreamble:   resolved.GoPreamble,
		DartPreamble: resolved.DartPreamble,
	}
	b := &builder{
		api: api, config: resolved, unit: result,
		typeCache: map[types.Type]*wireType{}, namedUsage: map[*types.Named]namedUsage{},
		namedModels: map[*types.Named]*namedModel{}, structModels: map[*types.Named]*structModel{},
		opaqueModels: map[*types.Named]*opaqueModel{},
	}
	if err := b.discoverUsages(); err != nil {
		return nil, nil, err
	}
	for _, callable := range api.Callables {
		call, err := b.mapCallable(callable)
		if err != nil {
			wrapped := fmt.Errorf("%s: %w", callable.Func.FullName(), err)
			if resolved.StopOnError {
				return nil, b.warnings, wrapped
			}
			b.warnings = append(b.warnings, wrapped)
			continue
		}
		b.unit.Calls = append(b.unit.Calls, call)
		call.ID = len(b.unit.Calls) - 1
		call.Codec = preferredCodecForCall(call)
		if call.Receiver == nil {
			b.unit.TopCalls = append(b.unit.TopCalls, call)
		} else {
			switch call.Receiver.Kind {
			case kindOpaque:
				call.Receiver.Opaque.Methods = append(call.Receiver.Opaque.Methods, call)
			case kindStruct:
				call.Receiver.Struct.Methods = append(call.Receiver.Struct.Methods, call)
			case kindNamed:
				call.Receiver.Named.Methods = append(call.Receiver.Named.Methods, call)
			default:
				return nil, b.warnings, fmt.Errorf("method receiver %s maps to unsupported Dart receiver %s", callable.Receiver.Obj().Name(), call.Receiver.Kind)
			}
		}
	}

	sort.SliceStable(b.unit.Types, func(i, j int) bool { return b.unit.Types[i].ID < b.unit.Types[j].ID })
	return b.unit, b.warnings, nil
}

func (b *builder) discoverUsages() error {
	// Pointer receivers force reference semantics. Value receivers alone do not.
	for _, callable := range b.api.Callables {
		if callable.PointerRecv && callable.Receiver != nil {
			if err := b.markNamedUsage(callable.Receiver, usageOpaque); err != nil {
				return err
			}
		}
	}
	for _, callable := range b.api.Callables {
		sig := callable.Signature
		for i := 0; i < sig.Params().Len(); i++ {
			if err := b.discoverTypeUsage(sig.Params().At(i).Type(), map[types.Type]bool{}); err != nil {
				return fmt.Errorf("%s parameter %d: %w", callable.Func.FullName(), i, err)
			}
		}
		for i := 0; i < sig.Results().Len(); i++ {
			if isErrorType(sig.Results().At(i).Type()) {
				continue
			}
			if err := b.discoverTypeUsage(sig.Results().At(i).Type(), map[types.Type]bool{}); err != nil {
				return fmt.Errorf("%s result %d: %w", callable.Func.FullName(), i, err)
			}
		}
	}
	return nil
}

func (b *builder) discoverTypeUsage(typ types.Type, seen map[types.Type]bool) error {
	typ = types.Unalias(typ)
	if seen[typ] {
		return nil
	}
	seen[typ] = true
	switch typ := typ.(type) {
	case *types.Pointer:
		elem := types.Unalias(typ.Elem())
		if named, ok := elem.(*types.Named); ok {
			if _, ok := named.Underlying().(*types.Struct); ok && !isBigInt(named) {
				// A pointer to an ordinary value struct is an optional,
				// serializable value. Pointer receiver methods are marked
				// opaque in the pass above and retain handle semantics.
				if b.namedUsage[named] == usageOpaque {
					return nil
				}
				if err := b.markNamedUsage(named, usageValue); err != nil {
					return err
				}
				structure := named.Underlying().(*types.Struct)
				for i := 0; i < structure.NumFields(); i++ {
					field := structure.Field(i)
					if field.Name() == "_" || field.Pkg() != nil && !field.Exported() {
						continue
					}
					if err := b.discoverTypeUsage(field.Type(), seen); err != nil {
						return err
					}
				}
				return nil
			}
		}
		return b.discoverTypeUsage(elem, seen)
	case *types.Named:
		if _, ok := typ.Underlying().(*types.Struct); ok && !isTime(typ) && !isBigInt(typ) {
			if err := b.markNamedUsage(typ, usageValue); err != nil {
				return err
			}
			structure := typ.Underlying().(*types.Struct)
			for i := 0; i < structure.NumFields(); i++ {
				field := structure.Field(i)
				if field.Name() == "_" || field.Pkg() != nil && !field.Exported() {
					continue
				}
				if err := b.discoverTypeUsage(field.Type(), seen); err != nil {
					return err
				}
			}
			return nil
		}
		return b.discoverTypeUsage(typ.Underlying(), seen)
	case *types.Slice:
		return b.discoverTypeUsage(typ.Elem(), seen)
	case *types.Array:
		return b.discoverTypeUsage(typ.Elem(), seen)
	case *types.Map:
		if err := b.discoverTypeUsage(typ.Key(), seen); err != nil {
			return err
		}
		return b.discoverTypeUsage(typ.Elem(), seen)
	default:
		return nil
	}
}

func (b *builder) markNamedUsage(named *types.Named, usage namedUsage) error {
	if named.Obj() == nil || named.Obj().Pkg() == nil {
		return nil
	}
	previous := b.namedUsage[named]
	if previous != usageUnknown && previous != usage {
		return fmt.Errorf("Go struct %s is used both by value and by pointer; choose one bridge representation", named.Obj().Name())
	}
	b.namedUsage[named] = usage
	return nil
}

func (b *builder) mapCallable(source *model.Callable) (*callModel, error) {
	sig := source.Signature
	if sig.TypeParams() != nil && sig.TypeParams().Len() != 0 {
		return nil, errors.New("generic functions are not supported yet")
	}
	if sig.Variadic() {
		return nil, errors.New("variadic functions are not supported yet")
	}
	call := &callModel{
		GoName: source.Func.Name(), DartName: source.DartName, Mode: source.Mode, Docs: source.Docs, SourceFile: source.SourceFile,
		PointerRecv: source.PointerRecv,
	}
	if source.Receiver == nil {
		call.WireName = source.Func.Name()
		call.GoTarget = b.qualifyInput(source.Func.Name())
	} else {
		usage := b.namedUsage[source.Receiver]
		if usage == usageOpaque {
			pointer := types.NewPointer(source.Receiver)
			receiver, err := b.mapType(pointer)
			if err != nil {
				return nil, fmt.Errorf("receiver: %w", err)
			}
			call.Receiver = receiver
		} else {
			receiver, err := b.mapType(source.Receiver)
			if err != nil {
				return nil, fmt.Errorf("receiver: %w", err)
			}
			call.Receiver = receiver
		}
		call.WireName = source.Receiver.Obj().Name() + "." + source.Func.Name()
		call.GoTarget = source.Func.Name()
	}

	usedParamNames := map[string]int{}
	for i := 0; i < sig.Params().Len(); i++ {
		variable := sig.Params().At(i)
		mapped, err := b.mapType(variable.Type())
		if err != nil {
			return nil, fmt.Errorf("parameter %d (%s): %w", i, variable.Name(), err)
		}
		goName := variable.Name()
		if goName == "" || goName == "_" {
			goName = fmt.Sprintf("arg%d", i)
		}
		dartName := uniqueName(names.LowerCamel(goName), usedParamNames)
		call.Params = append(call.Params, &paramModel{GoName: goName, DartName: dartName, CName: names.CIdentifier(dartName), Type: mapped})
	}

	results := sig.Results()
	nonError := results.Len()
	if results.Len() != 0 && isErrorType(results.At(results.Len()-1).Type()) {
		call.HasError = true
		nonError--
	}
	if nonError > 1 {
		return nil, errors.New("multiple non-error results are not supported yet")
	}
	if nonError == 1 {
		mapped, err := b.mapType(results.At(0).Type())
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		call.Result = mapped
		call.ResultGoName = results.At(0).Name()
	}
	if results.Len() > nonError+btoi(call.HasError) {
		return nil, errors.New("error must be the final result")
	}
	return call, nil
}

func (b *builder) mapType(original types.Type) (*wireType, error) {
	original = types.Unalias(original)
	if cached := b.cachedType(original); cached != nil {
		return cached, nil
	}

	switch typ := original.(type) {
	case *types.Basic:
		return b.mapBasic(original, typ)
	case *types.Pointer:
		elemRaw := types.Unalias(typ.Elem())
		if _, nested := elemRaw.(*types.Pointer); nested {
			return nil, errors.New("nested pointers are not supported")
		}
		if named, ok := elemRaw.(*types.Named); ok {
			if isBigInt(named) {
				b.unit.UsesBigInt = true
				return b.newSimpleType(original, kindBigInt, "BigInt?"), nil
			}
			if _, ok := named.Underlying().(*types.Struct); ok {
				if err := b.ensureNamedFromInput(named); err != nil {
					return nil, err
				}
				if b.namedUsage[named] == usageOpaque {
					return b.mapOpaque(original, named)
				}
			}
		}
		elem, err := b.mapType(elemRaw)
		if err != nil {
			return nil, err
		}
		result := b.newType(original, kindPointer, elem.DartType+"?")
		result.Elem = elem
		return result, nil
	case *types.Slice:
		elemBasic, _ := types.Unalias(typ.Elem()).(*types.Basic)
		if elemBasic != nil {
			switch elemBasic.Kind() {
			case types.Uint8:
				return b.newSimpleType(original, kindBytes, "Uint8List"), nil
			case types.Int32:
				return b.newSimpleType(original, kindInt32List, "Int32List"), nil
			case types.Int64:
				return b.newSimpleType(original, kindInt64List, "Int64List"), nil
			case types.Float64:
				return b.newSimpleType(original, kindFloat64List, "Float64List"), nil
			}
		}
		elem, err := b.mapType(typ.Elem())
		if err != nil {
			return nil, err
		}
		result := b.newType(original, kindSlice, "List<"+elem.DartType+">")
		result.Elem = elem
		return result, nil
	case *types.Array:
		elem, err := b.mapType(typ.Elem())
		if err != nil {
			return nil, err
		}
		result := b.newType(original, kindArray, "List<"+elem.DartType+">")
		result.Elem = elem
		result.Length = typ.Len()
		return result, nil
	case *types.Map:
		key, err := b.mapType(typ.Key())
		if err != nil {
			return nil, fmt.Errorf("map key: %w", err)
		}
		if !validMapKey(key) {
			return nil, fmt.Errorf("map key type %s is not supported by StandardMessageCodec", key.DartType)
		}
		elem, err := b.mapType(typ.Elem())
		if err != nil {
			return nil, fmt.Errorf("map value: %w", err)
		}
		result := b.newType(original, kindMap, "Map<"+key.DartType+", "+elem.DartType+">")
		result.Key, result.Elem = key, elem
		return result, nil
	case *types.Interface:
		if typ.Empty() {
			return b.newSimpleType(original, kindAny, "Object?"), nil
		}
		return nil, errors.New("non-empty interfaces are not supported yet")
	case *types.Named:
		if isTime(typ) {
			b.unit.UsesTime = true
			return b.newSimpleType(original, kindTime, "DateTime"), nil
		}
		if isBigInt(typ) {
			b.unit.UsesBigInt = true
			return b.newSimpleType(original, kindBigInt, "BigInt"), nil
		}
		if err := b.ensureNamedFromInput(typ); err != nil {
			return nil, err
		}
		if _, ok := typ.Underlying().(*types.Struct); ok {
			if b.namedUsage[typ] == usageOpaque {
				return b.mapOpaque(types.NewPointer(typ), typ)
			}
			return b.mapStruct(original, typ)
		}
		return b.mapNamed(original, typ)
	default:
		return nil, fmt.Errorf("unsupported Go type %T (%s)", original, original.String())
	}
}

func (b *builder) mapBasic(original types.Type, basic *types.Basic) (*wireType, error) {
	var kind typeKind
	var dart string
	var bitSize int
	var signed bool
	switch basic.Kind() {
	case types.Bool:
		kind, dart = kindBool, "bool"
	case types.String:
		kind, dart = kindString, "String"
	case types.Int8:
		kind, dart, bitSize, signed = kindSigned, "int", 8, true
	case types.Int16:
		kind, dart, bitSize, signed = kindSigned, "int", 16, true
	case types.Int32:
		kind, dart, bitSize, signed = kindSigned, "int", 32, true
	case types.Int64:
		kind, dart, bitSize, signed = kindSigned, "int", 64, true
	case types.Int:
		kind, dart, bitSize, signed = kindSigned, "int", strconv.IntSize, true
	case types.Uint8:
		kind, dart, bitSize = kindUnsigned, "int", 8
	case types.Uint16:
		kind, dart, bitSize = kindUnsigned, "int", 16
	case types.Uint32:
		kind, dart, bitSize = kindUnsigned, "int", 32
	case types.Uint64:
		kind, dart, bitSize = kindUnsigned, "BigInt", 64
		b.unit.UsesBigInt = true
	case types.Uint, types.Uintptr:
		kind, dart, bitSize = kindUnsigned, "BigInt", strconv.IntSize
		b.unit.UsesBigInt = true
	case types.Float32:
		kind, dart, bitSize = kindFloat, "double", 32
	case types.Float64:
		kind, dart, bitSize = kindFloat, "double", 64
	default:
		return nil, fmt.Errorf("unsupported basic type %s", basic.Name())
	}
	result := b.newType(original, kind, dart)
	result.BasicKind, result.BitSize, result.Signed = basic.Kind(), bitSize, signed
	return result, nil
}

func (b *builder) mapNamed(original types.Type, named *types.Named) (*wireType, error) {
	if existing := b.namedModels[named]; existing != nil {
		return existing.Type, nil
	}
	decl := b.api.Types[named.Obj()]
	dartName := names.UpperCamel(named.Obj().Name())
	docs := ""
	if decl != nil {
		dartName, docs = decl.DartName, decl.Docs
	}
	result := b.newType(original, kindNamed, dartName)
	sourceFile := ""
	if decl != nil {
		sourceFile = decl.SourceFile
	}
	model := &namedModel{GoName: named.Obj().Name(), DartName: dartName, Docs: docs, SourceFile: sourceFile, Type: result}
	result.Named = model
	b.namedModels[named] = model
	b.unit.Named = append(b.unit.Named, model)
	underlying, err := b.mapType(named.Underlying())
	if err != nil {
		return nil, err
	}
	model.Underlying = underlying
	for _, item := range b.api.Constants[named] {
		literal, isConst, err := dartConstantLiteral(item.Object.Val(), underlying)
		if err != nil {
			b.warnings = append(b.warnings, fmt.Errorf("constant %s.%s: %w", item.Object.Pkg().Path(), item.Object.Name(), err))
			continue
		}
		model.Constants = append(model.Constants, &constantModel{
			GoName: item.Object.Name(), DartName: item.DartName, Docs: item.Docs,
			DartLiteral: literal, IsConst: isConst,
		})
	}
	return result, nil
}

func (b *builder) mapStruct(original types.Type, named *types.Named) (*wireType, error) {
	if existing := b.structModels[named]; existing != nil {
		return existing.Type, nil
	}
	decl := b.api.Types[named.Obj()]
	dartName := names.UpperCamel(named.Obj().Name())
	docs := ""
	if decl != nil {
		dartName, docs = decl.DartName, decl.Docs
	}
	result := b.newType(original, kindStruct, dartName)
	sourceFile := ""
	if decl != nil {
		sourceFile = decl.SourceFile
	}
	structure := &structModel{GoName: named.Obj().Name(), DartName: dartName, Docs: docs, SourceFile: sourceFile, Type: result}
	result.Struct = structure
	b.structModels[named] = structure
	b.unit.Structs = append(b.unit.Structs, structure)

	goStruct := named.Underlying().(*types.Struct)
	usedNames := map[string]int{}
	wireNames := map[string]bool{}
	for i := 0; i < goStruct.NumFields(); i++ {
		field := goStruct.Field(i)
		tag := reflect.StructTag(goStruct.Tag(i))
		jsonName := strings.Split(tag.Get("json"), ",")[0]
		fgbName := strings.Split(tag.Get("flutter_go_bridge"), ",")[0]
		if fgbName == "-" || jsonName == "-" || field.Name() == "_" {
			continue
		}
		if !field.Exported() {
			return nil, fmt.Errorf("value struct %s has unexported field %s; exclude it with `flutter_go_bridge:\"-\"`", named.Obj().Name(), field.Name())
		}
		wireName := fgbName
		if wireName == "" {
			wireName = jsonName
		}
		if wireName == "" {
			wireName = names.LowerCamel(field.Name())
		}
		if wireNames[wireName] {
			return nil, fmt.Errorf("value struct %s has duplicate wire field %q", named.Obj().Name(), wireName)
		}
		wireNames[wireName] = true
		mapped, err := b.mapType(field.Type())
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name(), err)
		}
		dartName := uniqueName(names.LowerCamel(wireName), usedNames)
		structure.Fields = append(structure.Fields, &fieldModel{
			GoName: field.Name(), CName: names.CIdentifier(wireName), DartName: dartName, WireName: wireName,
			Type: mapped, Optional: isPointerType(field.Type()),
		})
	}
	return result, nil
}

func (b *builder) mapOpaque(original types.Type, named *types.Named) (*wireType, error) {
	if existing := b.opaqueModels[named]; existing != nil {
		b.typeCache[original] = existing.Type
		return existing.Type, nil
	}
	decl := b.api.Types[named.Obj()]
	dartName := names.UpperCamel(named.Obj().Name())
	docs := ""
	if decl != nil {
		dartName, docs = decl.DartName, decl.Docs
	}
	result := b.newType(original, kindOpaque, dartName+"?")
	sourceFile := ""
	if decl != nil {
		sourceFile = decl.SourceFile
	}
	opaque := &opaqueModel{GoName: named.Obj().Name(), DartName: dartName, Docs: docs, SourceFile: sourceFile, Type: result}
	result.Opaque = opaque
	b.opaqueModels[named] = opaque
	b.unit.Opaques = append(b.unit.Opaques, opaque)
	return result, nil
}

func (b *builder) newSimpleType(original types.Type, kind typeKind, dart string) *wireType {
	if cached := b.cachedType(original); cached != nil {
		return cached
	}
	return b.newType(original, kind, dart)
}

func (b *builder) newType(original types.Type, kind typeKind, dart string) *wireType {
	if cached := b.cachedType(original); cached != nil {
		return cached
	}
	result := &wireType{ID: b.nextTypeID, Kind: kind, Original: original, DartType: dart}
	b.nextTypeID++
	b.typeCache[original] = result
	b.unit.Types = append(b.unit.Types, result)
	return result
}

func (b *builder) cachedType(original types.Type) *wireType {
	if cached := b.typeCache[original]; cached != nil {
		return cached
	}
	for existing, cached := range b.typeCache {
		if types.Identical(existing, original) {
			b.typeCache[original] = cached
			return cached
		}
	}
	return nil
}

func (b *builder) ensureNamedFromInput(named *types.Named) error {
	if named.Obj() == nil || named.Obj().Pkg() == nil {
		return nil
	}
	if named.Obj().Pkg() != b.api.Package.Types {
		return fmt.Errorf("external named type %s.%s is not supported yet (time.Time and math/big.Int are supported)", named.Obj().Pkg().Path(), named.Obj().Name())
	}
	return nil
}

func (b *builder) qualifyInput(identifier string) string {
	if b.unit.Direct {
		return identifier
	}
	return "api." + identifier
}

func isErrorType(typ types.Type) bool {
	errorObject := types.Universe.Lookup("error")
	return errorObject != nil && types.Identical(types.Unalias(typ), errorObject.Type())
}

func isTime(named *types.Named) bool {
	return isNamed(named, "time", "Time")
}

func isBigInt(named *types.Named) bool {
	return isNamed(named, "math/big", "Int")
}

func isNamed(named *types.Named, packagePath, name string) bool {
	return named != nil && named.Obj() != nil && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}

func validMapKey(typ *wireType) bool {
	switch typ.Kind {
	case kindBool, kindString, kindSigned, kindUnsigned, kindFloat, kindNamed:
		return true
	default:
		return false
	}
}

func uniqueName(base string, used map[string]int) string {
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s%d", base, count+1)
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func dartConstantLiteral(value constant.Value, underlying *wireType) (string, bool, error) {
	switch underlying.Kind {
	case kindString:
		return strconv.Quote(constant.StringVal(value)), true, nil
	case kindBool:
		return strings.ToLower(value.ExactString()), true, nil
	case kindSigned, kindUnsigned:
		text := value.ExactString()
		if underlying.DartType == "BigInt" {
			return "BigInt.parse(" + strconv.Quote(text) + ")", false, nil
		}
		return text, true, nil
	case kindFloat:
		return value.ExactString(), true, nil
	default:
		return "", false, fmt.Errorf("underlying type %s cannot be emitted as a Dart constant", underlying.Kind)
	}
}

func exportedIdentifier(value string) bool {
	for _, r := range value {
		return unicode.IsUpper(r)
	}
	return false
}

func isPointerType(typ types.Type) bool {
	_, ok := types.Unalias(typ).(*types.Pointer)
	return ok
}
