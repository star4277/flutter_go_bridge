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

	"github.com/star4277/flutter-go-bridge-gokit/internal/config"
	"github.com/star4277/flutter-go-bridge-gokit/internal/model"
	"github.com/star4277/flutter-go-bridge-gokit/internal/names"
)

// structClass is the FRB-style bridge classification of a named Go struct:
// fully translatable structs travel by serialized fields, everything else
// degrades to a GoOpaque handle.
type structClass uint8

const (
	classUnknown structClass = iota
	classInProgress
	classValue
	classOpaque
)

type builder struct {
	api           *model.API
	config        config.Resolved
	unit          *unit
	typeCache     map[types.Type]*wireType
	structClasses map[*types.Named]structClass
	namedModels   map[*types.Named]*namedModel
	structModels  map[*types.Named]*structModel
	opaqueModels  map[*types.Named]*opaqueModel
	warnings      []error
	nextTypeID    int
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
		typeCache:     map[types.Type]*wireType{},
		structClasses: map[*types.Named]structClass{},
		namedModels:   map[*types.Named]*namedModel{}, structModels: map[*types.Named]*structModel{},
		opaqueModels: map[*types.Named]*opaqueModel{},
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
		// Value structs travel by serialized fields (pointer-receiver methods
		// then operate on the reconstructed, addressable Go value); GoOpaque
		// receivers keep handle semantics and preserve Go-side state.
		var receiver *wireType
		var err error
		if b.classifyStruct(source.Receiver) == classOpaque {
			receiver, err = b.mapType(types.NewPointer(source.Receiver))
		} else {
			receiver, err = b.mapType(source.Receiver)
		}
		if err != nil {
			return nil, fmt.Errorf("receiver: %w", err)
		}
		call.Receiver = receiver
		call.WireName = source.Receiver.Obj().Name() + "." + source.Func.Name()
		call.GoTarget = source.Func.Name()
	}

	usedParamNames := map[string]int{}
	hasCallbackParam := false
	// `//fgb:nullable` only makes sense for callbacks: every other Go type
	// already expresses optionality through a pointer.
	nullableParams := map[string]bool{}
	for _, name := range source.NullableParams {
		nullableParams[name] = false
	}
	for i := 0; i < sig.Params().Len(); i++ {
		variable := sig.Params().At(i)
		var mapped *wireType
		var err error
		if signature, isFunc := types.Unalias(variable.Type()).(*types.Signature); isFunc {
			hasCallbackParam = true
			mapped, err = b.mapCallback(variable.Type(), signature)
		} else {
			mapped, err = b.mapType(variable.Type())
		}
		if err != nil {
			return nil, fmt.Errorf("parameter %d (%s): %w", i, variable.Name(), err)
		}
		goName := variable.Name()
		if goName == "" || goName == "_" {
			goName = fmt.Sprintf("arg%d", i)
		}
		nullable := false
		if _, listed := nullableParams[variable.Name()]; listed && variable.Name() != "" {
			if !mapped.nilableWithoutPointer() {
				return nil, fmt.Errorf("//fgb:nullable lists parameter %q, but only callback, slice, map and byte/typed-list parameters can be nil without a pointer; use a Go pointer for other optional values", variable.Name())
			}
			nullableParams[variable.Name()] = true
			nullable = true
		}
		dartName := uniqueName(names.LowerCamel(goName), usedParamNames)
		call.Params = append(call.Params, &paramModel{
			GoName: goName, DartName: dartName, CName: names.CIdentifier(dartName),
			Type: mapped, Nullable: nullable,
		})
	}
	for _, name := range source.NullableParams {
		if !nullableParams[name] {
			return nil, fmt.Errorf("//fgb:nullable lists unknown parameter %q", name)
		}
	}
	if hasCallbackParam && call.Mode != model.CallModeAsync {
		return nil, errors.New("parameters of function type require //fgb:async: a synchronous call blocks the Dart event loop, so the callback could never run")
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
			if _, ok := named.Underlying().(*types.Struct); ok && !isDartOpaque(named) && !isTime(named) {
				if err := b.ensureNamedFromInput(named); err != nil {
					return nil, err
				}
				if b.classifyStruct(named) == classOpaque {
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
	case *types.Signature:
		return nil, errors.New("function types are only supported as direct parameters of //fgb:async functions")
	case *types.Named:
		if isTime(typ) {
			b.unit.UsesTime = true
			return b.newSimpleType(original, kindTime, "DateTime"), nil
		}
		if isBigInt(typ) {
			b.unit.UsesBigInt = true
			return b.newSimpleType(original, kindBigInt, "BigInt"), nil
		}
		if isDartOpaque(typ) {
			b.unit.UsesDartOpaque = true
			return b.newSimpleType(original, kindDartOpaque, "Object"), nil
		}
		if err := b.ensureNamedFromInput(typ); err != nil {
			return nil, err
		}
		if _, ok := typ.Underlying().(*types.Struct); ok {
			if b.classifyStruct(typ) == classOpaque {
				return nil, fmt.Errorf("GoOpaque type %s must be passed as *%s", typ.Obj().Name(), typ.Obj().Name())
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

// classifyStruct decides how a named struct bridges. FRB semantics: a struct
// whose (bridged) fields are all translatable becomes a Dart value class;
// anything else - or an explicit fgb(opaque) - becomes a GoOpaque handle.
func (b *builder) classifyStruct(named *types.Named) structClass {
	if named == nil || named.Obj() == nil {
		return classValue
	}
	if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
		return classValue
	}
	if existing := b.structClasses[named]; existing == classValue || existing == classOpaque {
		return existing
	}
	if b.structClasses[named] == classInProgress {
		// A pointer cycle through value structs stays translatable; the
		// outermost frame owns the final verdict.
		return classValue
	}
	if named.Obj().Pkg() == b.api.Package.Types && b.api.OpaqueTypes[named.Obj().Name()] {
		b.structClasses[named] = classOpaque
		return classOpaque
	}
	b.structClasses[named] = classInProgress
	class := classValue
	goStruct := named.Underlying().(*types.Struct)
	for i := 0; i < goStruct.NumFields(); i++ {
		field := goStruct.Field(i)
		tag := reflect.StructTag(goStruct.Tag(i))
		options, err := parseFieldTag(tag.Get("fgb"))
		if err != nil {
			// The tag error surfaces from mapStruct with full context.
			continue
		}
		if skipStructField(field, tag, options) {
			continue
		}
		if reason := b.fieldTranslateBlocker(field.Type(), map[types.Type]bool{}); reason != "" {
			b.warnings = append(b.warnings, fmt.Errorf(
				"struct %s bridges as GoOpaque because field %s %s; mark the type with fgb(opaque) to silence this warning",
				named.Obj().Name(), field.Name(), reason))
			class = classOpaque
			break
		}
	}
	b.structClasses[named] = class
	return class
}

// fieldTranslateBlocker reports why a field type prevents value translation,
// or "" when it is serializable.
func (b *builder) fieldTranslateBlocker(typ types.Type, seen map[types.Type]bool) string {
	typ = types.Unalias(typ)
	if seen[typ] {
		return ""
	}
	seen[typ] = true
	switch typ := typ.(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.Bool, types.String,
			types.Int8, types.Int16, types.Int32, types.Int64, types.Int,
			types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uint, types.Uintptr,
			types.Float32, types.Float64:
			return ""
		default:
			return fmt.Sprintf("has unsupported basic type %s", typ.Name())
		}
	case *types.Pointer:
		elem := types.Unalias(typ.Elem())
		if _, nested := elem.(*types.Pointer); nested {
			return "has a nested pointer type"
		}
		if named, ok := elem.(*types.Named); ok && !isTime(named) && !isBigInt(named) && !isDartOpaque(named) {
			if _, isStruct := named.Underlying().(*types.Struct); isStruct {
				if external := b.externalTypeBlocker(named); external != "" {
					return external
				}
				// *Struct is always bridgeable: either an optional value or a
				// GoOpaque handle.
				return ""
			}
		}
		return b.fieldTranslateBlocker(elem, seen)
	case *types.Slice:
		return b.fieldTranslateBlocker(typ.Elem(), seen)
	case *types.Array:
		return b.fieldTranslateBlocker(typ.Elem(), seen)
	case *types.Map:
		if blocker := b.fieldTranslateBlocker(typ.Key(), seen); blocker != "" {
			return blocker
		}
		return b.fieldTranslateBlocker(typ.Elem(), seen)
	case *types.Interface:
		if typ.Empty() {
			return ""
		}
		return "has a non-empty interface type"
	case *types.Named:
		if isTime(typ) || isBigInt(typ) || isDartOpaque(typ) {
			return ""
		}
		if external := b.externalTypeBlocker(typ); external != "" {
			return external
		}
		if _, isStruct := typ.Underlying().(*types.Struct); isStruct {
			if b.classifyStruct(typ) == classOpaque {
				return fmt.Sprintf("holds GoOpaque struct %s by value (use *%s)", typ.Obj().Name(), typ.Obj().Name())
			}
			return ""
		}
		return b.fieldTranslateBlocker(typ.Underlying(), seen)
	case *types.Signature:
		return "has a function type"
	case *types.Chan:
		return "has a channel type"
	default:
		return fmt.Sprintf("has unsupported type %s", typ.String())
	}
}

func (b *builder) externalTypeBlocker(named *types.Named) string {
	if named.Obj() == nil || named.Obj().Pkg() == nil || named.Obj().Pkg() == b.api.Package.Types {
		return ""
	}
	return fmt.Sprintf("uses external type %s.%s", named.Obj().Pkg().Path(), named.Obj().Name())
}

// skipStructField mirrors the shared exclusion rules: blank/unexported fields
// and fields opted out through fgb/json/flutter_go_bridge tags.
func skipStructField(field *types.Var, tag reflect.StructTag, options fieldTagOptions) bool {
	if options.Ignore || field.Name() == "_" || !field.Exported() {
		return true
	}
	if strings.Split(tag.Get("flutter_go_bridge"), ",")[0] == "-" {
		return true
	}
	return strings.Split(tag.Get("json"), ",")[0] == "-"
}

// fieldTagOptions is the parsed form of an `fgb:"..."` struct tag.
type fieldTagOptions struct {
	Ignore       bool
	NonFinal     bool
	Rename       string
	DefaultValue string
}

// parseFieldTag understands `fgb:"ignore"`, `fgb:"rename:name"`,
// `fgb:"non-final"`, and `fgb:"defaultValue: expr"` - combinable with commas.
// defaultValue consumes the rest of the tag so Dart expressions may contain
// commas; it must therefore be the last option.
func parseFieldTag(raw string) (fieldTagOptions, error) {
	var result fieldTagOptions
	rest := strings.TrimSpace(raw)
	for rest != "" {
		if value, ok := strings.CutPrefix(rest, "defaultValue:"); ok {
			result.DefaultValue = strings.TrimSpace(value)
			if result.DefaultValue == "" {
				return result, errors.New(`fgb:"defaultValue:" needs a Dart expression`)
			}
			break
		}
		token := rest
		if index := strings.Index(rest, ","); index >= 0 {
			token, rest = strings.TrimSpace(rest[:index]), strings.TrimSpace(rest[index+1:])
		} else {
			rest = ""
		}
		switch {
		case token == "":
		case token == "ignore" || token == "-":
			result.Ignore = true
		case token == "non-final":
			result.NonFinal = true
		case strings.HasPrefix(token, "rename:"):
			result.Rename = strings.TrimSpace(strings.TrimPrefix(token, "rename:"))
			if result.Rename == "" {
				return result, errors.New(`fgb:"rename:" needs a field name`)
			}
		default:
			return result, fmt.Errorf("unknown fgb field tag option %q (want ignore, rename:name, non-final, or defaultValue: expr)", token)
		}
	}
	return result, nil
}

// mapCallback bridges a Go function-type parameter. Dart supplies a closure
// (`FutureOr<R> Function(...)`, so plain and async functions both fit); Go
// receives a synthesized func value whose invocation posts the arguments to
// the Dart event loop and parks the goroutine until the reply arrives.
func (b *builder) mapCallback(original types.Type, signature *types.Signature) (*wireType, error) {
	if cached := b.cachedType(original); cached != nil {
		return cached, nil
	}
	if signature.TypeParams() != nil && signature.TypeParams().Len() != 0 {
		return nil, errors.New("generic function types are not supported")
	}
	if signature.Variadic() {
		return nil, errors.New("variadic function types are not supported")
	}

	callback := &callbackModel{}
	for i := 0; i < signature.Params().Len(); i++ {
		paramType := signature.Params().At(i).Type()
		if _, nested := types.Unalias(paramType).(*types.Signature); nested {
			return nil, errors.New("nested function types are not supported")
		}
		mapped, err := b.mapType(paramType)
		if err != nil {
			return nil, fmt.Errorf("callback parameter %d: %w", i, err)
		}
		callback.Params = append(callback.Params, mapped)
	}

	results := signature.Results()
	nonError := results.Len()
	if results.Len() != 0 && isErrorType(results.At(results.Len()-1).Type()) {
		callback.HasError = true
		nonError--
	}
	if nonError > 1 {
		return nil, errors.New("callbacks support at most one non-error result")
	}
	if results.Len() > nonError+btoi(callback.HasError) {
		return nil, errors.New("callback error must be the final result")
	}
	if nonError == 1 {
		mapped, err := b.mapType(results.At(0).Type())
		if err != nil {
			return nil, fmt.Errorf("callback result: %w", err)
		}
		callback.Result = mapped
	}

	resultDart := "void"
	if callback.Result != nil {
		resultDart = callback.Result.DartType
	}
	paramDarts := make([]string, len(callback.Params))
	for i, param := range callback.Params {
		paramDarts[i] = param.DartType
	}
	dartType := fmt.Sprintf("FutureOr<%s> Function(%s)", resultDart, strings.Join(paramDarts, ", "))

	result := b.newType(original, kindCallback, dartType)
	result.Callback = callback
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
		options, err := parseFieldTag(tag.Get("fgb"))
		if err != nil {
			return nil, fmt.Errorf("struct %s field %s: %w", named.Obj().Name(), field.Name(), err)
		}
		if skipStructField(field, tag, options) {
			continue
		}
		wireName := options.Rename
		if wireName == "" {
			wireName = strings.Split(tag.Get("flutter_go_bridge"), ",")[0]
		}
		if wireName == "" {
			wireName = strings.Split(tag.Get("json"), ",")[0]
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
		dartName := options.Rename
		if dartName == "" {
			dartName = names.LowerCamel(wireName)
		}
		dartName = uniqueName(dartName, usedNames)
		structure.Fields = append(structure.Fields, &fieldModel{
			GoName: field.Name(), CName: names.CIdentifier(wireName), DartName: dartName, WireName: wireName,
			Type: mapped, Optional: isPointerType(field.Type()),
			NonFinal: options.NonFinal, DefaultValue: options.DefaultValue,
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
		return fmt.Errorf("external named type %s.%s is not supported yet (time.Time, math/big.Int, and fgb.DartOpaque are supported)", named.Obj().Pkg().Path(), named.Obj().Name())
	}
	if b.api.IgnoredTypes[named.Obj().Name()] {
		return fmt.Errorf("type %s is marked fgb(ignore) but is used by the bridged API", named.Obj().Name())
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

// dartOpaquePackagePath is the runtime module holding fgb.DartOpaque; the
// generated bridge imports it only when the API actually uses DartOpaque.
const dartOpaquePackagePath = "github.com/star4277/flutter-go-bridge-gokit/fgb"

func isDartOpaque(named *types.Named) bool {
	return isNamed(named, dartOpaquePackagePath, "DartOpaque")
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

func isPointerType(typ types.Type) bool {
	_, ok := types.Unalias(typ).(*types.Pointer)
	return ok
}
