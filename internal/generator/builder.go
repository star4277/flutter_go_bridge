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

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/model"
	"github.com/star4277/flutter_go_bridge/internal/names"
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
	api             *model.API
	config          config.Resolved
	unit            *unit
	typeCache       map[types.Type]*wireType
	structClasses   map[*types.Named]structClass
	namedModels     map[*types.Named]*namedModel
	structModels    map[*types.Named]*structModel
	opaqueModels    map[*types.Named]*opaqueModel
	interfaceModels map[*types.Named]*interfaceModel
	// Dart has one library namespace across the mutually importing generated
	// source files. Reserve input-package names up front, then disambiguate
	// reachable external declarations without renaming the user's own types.
	dartTypeNames map[string]*types.Named
	dartNames     map[*types.Named]string
	// supportImportPath is the import path of the generated support package
	// for this project; empty disables DartOpaque/StreamSink detection.
	supportImportPath string
	warnings          []error
	nextTypeID        int
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
		PackagePath:      api.Package.PkgPath,
		PackageName:      api.Package.Name,
		InputDir:         api.InputDir,
		MirrorRoot:       api.InputDir,
		SourceFiles:      append([]string(nil), api.SourceFiles...),
		Direct:           direct,
		NeedsMain:        needsMain,
		LibraryName:      resolved.LibraryName,
		ClassName:        names.UpperCamel(resolved.DartEntrypointClassName),
		GoPreamble:       resolved.GoPreamble,
		DartPreamble:     resolved.DartPreamble,
		GoPackageAliases: map[string]string{},
		codecSupport:     map[codecCacheKey]bool{},
	}
	// Mirror the Dart tree from the Go module root so a package directory such
	// as api/ shows up as api/ on the Dart side too.
	if module := api.Package.Module; module != nil && module.Dir != "" {
		result.MirrorRoot = module.Dir
	}
	b := &builder{
		api: api, config: resolved, unit: result,
		typeCache:     map[types.Type]*wireType{},
		structClasses: map[*types.Named]structClass{},
		namedModels:   map[*types.Named]*namedModel{}, structModels: map[*types.Named]*structModel{},
		opaqueModels:    map[*types.Named]*opaqueModel{},
		interfaceModels: map[*types.Named]*interfaceModel{},
		dartTypeNames:   map[string]*types.Named{},
		dartNames:       map[*types.Named]string{},
	}
	for _, declaration := range api.Types {
		if declaration != nil && declaration.Named != nil {
			b.dartTypeNames[declaration.DartName] = declaration.Named
		}
	}
	usedTopCallNames := map[string]int{}
	if module := api.Package.Module; module != nil {
		b.supportImportPath = supportPackageImportPath(module.Path, module.Dir, SupportPackageDir(resolved))
	}
	result.SupportPackagePath = b.supportImportPath
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
		call.Codec = preferredCodecForCall(call, b.unit.codecSupport)
		if call.Receiver == nil {
			original := call.DartName
			call.DartName = uniqueName(original, usedTopCallNames)
			if call.DartName != original {
				b.warnings = append(b.warnings, fmt.Errorf("top-level Dart name %q collides after sanitization; renamed %s to %q", original, call.GoName, call.DartName))
			}
			b.unit.TopCalls = append(b.unit.TopCalls, call)
		} else {
			switch call.Receiver.Kind {
			case kindOpaque:
				b.disambiguateMethod(call, call.Receiver.Opaque.Methods)
				call.Receiver.Opaque.Methods = append(call.Receiver.Opaque.Methods, call)
			case kindStruct:
				b.disambiguateMethod(call, call.Receiver.Struct.Methods)
				call.Receiver.Struct.Methods = append(call.Receiver.Struct.Methods, call)
			case kindNamed:
				b.disambiguateMethod(call, call.Receiver.Named.Methods)
				call.Receiver.Named.Methods = append(call.Receiver.Named.Methods, call)
			default:
				return nil, b.warnings, fmt.Errorf("method receiver %s maps to unsupported Dart receiver %s", callable.Receiver.Obj().Name(), call.Receiver.Kind)
			}
		}
	}

	sort.SliceStable(b.unit.Types, func(i, j int) bool { return b.unit.Types[i].ID < b.unit.Types[j].ID })
	if err := b.checkMethodOverrides(); err != nil {
		return nil, b.warnings, err
	}
	return b.unit, b.warnings, nil
}

func (b *builder) disambiguateMethod(call *callModel, existing []*callModel) {
	base := call.DartName
	candidate := base
	for suffix := 2; ; suffix++ {
		collision := false
		for _, other := range existing {
			if other.DartName == candidate {
				collision = true
				break
			}
		}
		if !collision {
			break
		}
		candidate = fmt.Sprintf("%s%d", base, suffix)
	}
	if candidate != base {
		b.warnings = append(b.warnings, fmt.Errorf("method Dart name %q collides after sanitization; renamed %s to %q", base, call.GoName, candidate))
		call.DartName = candidate
	}
}

// checkMethodOverrides reconciles Go method shadowing with Dart's override
// rules. Go lets an embedded method be shadowed by any signature at all; Dart
// only accepts a compatible one, so a mismatch has to be reported here rather
// than as an error in the generated code.
func (b *builder) checkMethodOverrides() error {
	for _, structure := range b.unit.Structs {
		if structure.Super == nil {
			continue
		}
		inherited := map[string]*callModel{}
		inheritedOwner := map[string]*structModel{}
		for super := structure.Super; super != nil; super = super.Super {
			for _, call := range super.Methods {
				if _, seen := inherited[call.DartName]; !seen {
					inherited[call.DartName] = call
					inheritedOwner[call.DartName] = super
				}
			}
		}
		for _, call := range structure.Methods {
			promoted, shadows := inherited[call.DartName]
			if !shadows {
				continue
			}
			if dartMethodSignature(call) != dartMethodSignature(promoted) {
				return fmt.Errorf(
					"%s.%s shadows %s.%s with a different signature; Dart cannot express that on a subclass, so rename one of them with //fgb:rename",
					structure.GoName, call.GoName, inheritedOwner[call.DartName].GoName, promoted.GoName)
			}
			call.Overrides = true
		}
	}
	return nil
}

// dartMethodSignature is the shape Dart checks when validating an override.
func dartMethodSignature(call *callModel) string {
	prefix := ""
	if isAsyncCall(call) {
		prefix = "async "
	}
	return prefix + dartResultType(call) + " (" + dartParams(call) + ")"
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
		PointerRecv: source.PointerRecv, ContextIndex: -1,
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
		unaliased := types.Unalias(variable.Type())
		switch typed := unaliased.(type) {
		case *types.Signature:
			hasCallbackParam = true
			mapped, err = b.mapCallback(variable.Type(), typed)
		case *types.Chan:
			mapped, err = b.mapChannelStream(variable.Type(), typed)
		default:
			if isContextType(unaliased) {
				// The bridge owns the context: it never reaches Dart, so it
				// stays out of call.Params and is spliced back in when the Go
				// call expression is rendered.
				if call.ContextIndex >= 0 {
					return nil, errors.New("only one context.Context parameter is supported")
				}
				call.ContextIndex = i
				continue
			}
			mapped, err = b.mapType(variable.Type())
		}
		if err != nil {
			return nil, fmt.Errorf("parameter %d (%s): %w", i, variable.Name(), err)
		}
		if mapped.containsAtomic(map[int]bool{}) {
			switch mapped.Kind {
			case kindSlice, kindArray, kindMap:
				return nil, fmt.Errorf("parameter %d (%s) contains atomic values inside a collection; this would copy sync/atomic state", i, variable.Name())
			}
		}
		if mapped.usesPointerCodec(map[int]bool{}) {
			if _, pointer := types.Unalias(variable.Type()).(*types.Pointer); !pointer {
				return nil, fmt.Errorf("parameter %d (%s) contains an atomic value and must be passed by pointer to avoid copying sync/atomic state", i, variable.Name())
			}
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

	// Every `error` result is collected - non-nil ones fail the call and are
	// reported together - and the rest keep their Go order. Two or more of
	// those become a Dart record, so `error` no longer has to come last.
	results := sig.Results()
	usedResultNames := map[string]int{}
	named := results.Len() != 0
	for i := 0; i < results.Len(); i++ {
		variable := results.At(i)
		if isErrorType(variable.Type()) {
			call.ErrorCount++
			continue
		}
		mapped, err := b.mapType(variable.Type())
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", i, err)
		}
		if mapped.containsAtomic(map[int]bool{}) {
			switch mapped.Kind {
			case kindSlice, kindArray, kindMap:
				return nil, fmt.Errorf("result %d contains atomic values inside a collection; this would copy sync/atomic state", i)
			}
		}
		if containsStreamSink(mapped, map[int]bool{}) {
			return nil, errors.New("a stream sink cannot be returned to Dart; take it as a parameter instead")
		}
		if variable.Name() == "" || variable.Name() == "_" {
			named = false
		}
		call.Results = append(call.Results, &resultModel{
			GoName: variable.Name(), DartName: uniqueName(names.LowerCamel(variable.Name()), usedResultNames),
			GoIndex: i, Type: mapped,
		})
	}
	// A record reads much better with the Go result names, and Go requires
	// results to be either all named or none.
	call.NamedResults = named && len(call.Results) > 1

	// A call that only produces a stream owns it: exactly one sink and no
	// value to return means the Dart function can hand back Stream<T>
	// directly. Everything else keeps the sink as a parameter, so the Dart
	// side creates - and disposes - the StreamController itself.
	var sinkParams []*paramModel
	for _, param := range call.Params {
		if param.Type.Kind == kindStreamSink {
			sinkParams = append(sinkParams, param)
		}
	}
	if len(sinkParams) != 0 {
		if call.Mode != model.CallModeAsync {
			return nil, errors.New("stream sink parameters require //fgb:async: the Dart side must receive the stream while Go keeps producing")
		}
		for _, param := range sinkParams {
			if param.Nullable {
				return nil, fmt.Errorf("//fgb:nullable cannot be applied to the stream sink parameter %q", param.GoName)
			}
		}
		if len(sinkParams) == 1 && len(call.Results) == 0 {
			call.StreamParam = sinkParams[0]
		}
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
			if _, ok := named.Underlying().(*types.Struct); ok && !b.hasDedicatedMapping(named) {
				if err := b.ensureNamedSupported(named); err != nil {
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
		return nil, errors.New("an unnamed non-empty interface cannot be bridged; declare a named interface type")
	case *types.Signature:
		return nil, errors.New("function types are only supported as direct parameters of //fgb:async functions")
	case *types.Named:
		if isTime(typ) {
			b.unit.UsesTime = true
			return b.newSimpleType(original, kindTime, "DateTime"), nil
		}
		if isDuration(typ) {
			b.unit.UsesTime = true
			return b.newSimpleType(original, kindDuration, "Duration"), nil
		}
		if isBigInt(typ) {
			b.unit.UsesBigInt = true
			return b.newSimpleType(original, kindBigInt, "BigInt"), nil
		}
		if isInternetIP(typ) {
			b.unit.UsesInternetIP = true
			return b.newSimpleType(original, kindInternetIP, "InternetAddress"), nil
		}
		if isIPPrefix(typ) {
			b.unit.UsesIPPrefix = true
			return b.newSimpleType(original, kindIPPrefix, "String"), nil
		}
		if isURL(typ) {
			b.unit.UsesURL = true
			return b.newSimpleType(original, kindURL, "Uri"), nil
		}
		if isUUID(typ) {
			b.unit.UsesUUID = true
			return b.newSimpleType(original, kindUUID, "UuidValue"), nil
		}
		if b.isDartOpaque(typ) {
			b.unit.UsesDartOpaque = true
			b.unit.UsesRuntimePackage = true
			return b.newSimpleType(original, kindDartOpaque, "Object"), nil
		}
		if b.isStreamSink(typ) {
			return b.mapStreamSink(original, typ)
		}
		if valueType, ok := atomicValueType(typ); ok {
			return b.mapAtomic(original, typ, valueType)
		}
		if err := b.ensureNamedSupported(typ); err != nil {
			return nil, err
		}
		if interfaceType, isInterface := typ.Underlying().(*types.Interface); isInterface {
			if interfaceType.Empty() {
				return b.newSimpleType(original, kindAny, "Object?"), nil
			}
			return b.mapInterface(original, typ, interfaceType)
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

func (b *builder) mapAtomic(original types.Type, named *types.Named, valueType types.Type) (*wireType, error) {
	if named.Obj() != nil && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() != "sync/atomic" {
		b.registerExternalPackage(named.Obj().Pkg())
	}
	value, err := b.mapType(valueType)
	if err != nil {
		return nil, fmt.Errorf("atomic %s value: %w", named.Obj().Name(), err)
	}
	result := b.newType(original, kindAtomic, value.DartType)
	result.Atomic = &atomicModel{Value: value}
	return result, nil
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
	dartName = b.dartNameFor(named, dartName)
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
	bridged := 0
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
		bridged++
		if reason := b.fieldTranslateBlocker(field.Type(), map[types.Type]bool{}); reason != "" {
			b.warnings = append(b.warnings, fmt.Errorf(
				"struct %s bridges as GoOpaque because field %s %s; mark the type with fgb(opaque) to silence this warning",
				named.Obj().Name(), field.Name(), reason))
			class = classOpaque
			break
		}
	}
	// A struct that carries state but exposes none of it to the wire - the usual
	// shape of a third-party type built on unexported fields, which cannot be
	// annotated with fgb(opaque) - has to bridge as a handle. A Dart value class
	// would be empty, so it would both drop the payload and fail to compile.
	if class == classValue && bridged == 0 && goStruct.NumFields() > 0 {
		b.warnings = append(b.warnings, fmt.Errorf(
			"struct %s bridges as GoOpaque because none of its %d fields can travel to Dart (all unexported or excluded)",
			named.Obj().Name(), goStruct.NumFields()))
		class = classOpaque
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
		if named, ok := elem.(*types.Named); ok && !b.hasDedicatedMapping(named) {
			if _, isStruct := named.Underlying().(*types.Struct); isStruct {
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
		if b.hasDedicatedMapping(typ) {
			return ""
		}
		if declared, isInterface := typ.Underlying().(*types.Interface); isInterface {
			if declared.Empty() {
				return ""
			}
			if typ.Obj() != nil && typ.Obj().Pkg() != nil && typ.Obj().Pkg() != b.api.Package.Types {
				return fmt.Sprintf("uses external interface %s.%s", typ.Obj().Pkg().Path(), typ.Obj().Name())
			}
			// A named interface from the input package is bridged as a Dart
			// interface, so a field of that type is translatable.
			return ""
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
	Nullable     bool
	Rename       string
	DefaultValue string
}

// parseFieldTag understands `fgb:"ignore"`, `fgb:"rename:name"`,
// `fgb:"non-final"`, `fgb:"nullable"`, and `fgb:"defaultValue: expr"` -
// combinable with commas. defaultValue consumes the rest of the tag so Dart
// expressions may contain commas; it must therefore be the last option.
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
		case token == "nullable":
			result.Nullable = true
		case strings.HasPrefix(token, "rename:"):
			result.Rename = strings.TrimSpace(strings.TrimPrefix(token, "rename:"))
			if result.Rename == "" {
				return result, errors.New(`fgb:"rename:" needs a field name`)
			}
		default:
			return result, fmt.Errorf("unknown fgb field tag option %q (want ignore, rename:name, non-final, nullable, or defaultValue: expr)", token)
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

// mapChannelStream bridges a `chan<- T` parameter - the simple stream form.
// The bridge owns the channel: it creates it, drains it into the Dart stream,
// and closes it once the call returns. User code only ever sends, so there is
// no sink API, no error channel and nothing to close by hand.
func (b *builder) mapChannelStream(original types.Type, channel *types.Chan) (*wireType, error) {
	if cached := b.cachedType(original); cached != nil {
		return cached, nil
	}
	if channel.Dir() != types.SendOnly {
		return nil, errors.New("only send-only channels (chan<- T) can be bridged as a stream")
	}
	elem, err := b.mapType(channel.Elem())
	if err != nil {
		return nil, fmt.Errorf("stream element: %w", err)
	}
	if containsStreamSink(elem, map[int]bool{}) {
		return nil, errors.New("a stream of stream sinks is not supported")
	}
	b.unit.UsesStreamSink = true
	result := b.newType(original, kindStreamSink, "StreamSink<"+elem.DartType+">")
	result.Stream = elem
	result.ChannelStream = true
	return result, nil
}

// mapStreamSink bridges fgb.StreamSink[T]: Dart passes a StreamSink<T> (the
// sink of a StreamController it owns and disposes) and Go pushes values into
// it. Only the handle crosses the wire; items travel over the shared stream
// port using the standard codec.
func (b *builder) mapStreamSink(original types.Type, named *types.Named) (*wireType, error) {
	if cached := b.cachedType(original); cached != nil {
		return cached, nil
	}
	elem, err := b.mapType(named.TypeArgs().At(0))
	if err != nil {
		return nil, fmt.Errorf("stream element: %w", err)
	}
	if containsStreamSink(elem, map[int]bool{}) {
		return nil, errors.New("a stream of stream sinks is not supported")
	}
	b.unit.UsesStreamSink = true
	b.unit.UsesRuntimePackage = true
	result := b.newType(original, kindStreamSink, "StreamSink<"+elem.DartType+">")
	result.Stream = elem
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
	dartName = b.dartNameFor(named, dartName)
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
		// An embedded struct becomes the Dart superclass: Go promotes its
		// fields, so they travel flattened and both sides reach them through
		// promotion / inheritance.
		if field.Embedded() {
			super, err := b.mapEmbedded(named, field)
			if err != nil {
				return nil, err
			}
			if super != nil {
				if structure.Super != nil {
					return nil, fmt.Errorf("struct %s embeds %s and %s, but a Dart class can only extend one type", named.Obj().Name(), structure.Super.GoName, super.GoName)
				}
				structure.Super = super
				super.Subclassed = true
				for _, inherited := range super.allFields() {
					if wireNames[inherited.WireName] {
						return nil, fmt.Errorf("struct %s has duplicate wire field %q after promoting %s", named.Obj().Name(), inherited.WireName, super.GoName)
					}
					wireNames[inherited.WireName] = true
					usedNames[inherited.DartName]++
				}
				continue
			}
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
		if mapped.containsAtomic(map[int]bool{}) {
			switch mapped.Kind {
			case kindSlice, kindArray, kindMap:
				return nil, fmt.Errorf("field %s contains atomic values inside a collection; this would copy sync/atomic state", field.Name())
			case kindStruct:
				return nil, fmt.Errorf("field %s contains a value struct with atomic state; make the field a pointer to avoid copying it", field.Name())
			}
		}
		dartName := options.Rename
		if dartName == "" {
			dartName = names.LowerCamel(wireName)
		}
		dartName = uniqueName(dartName, usedNames)
		if options.Nullable {
			if isPointerType(field.Type()) {
				return nil, fmt.Errorf(`struct %s field %s is a pointer, so it is already nullable; drop fgb:"nullable"`, named.Obj().Name(), field.Name())
			}
			if !mapped.nilableWithoutPointer() {
				return nil, fmt.Errorf(`struct %s field %s cannot be marked fgb:"nullable": only callback, slice, map, byte/typed-list and interface fields can be nil without a pointer, so use a Go pointer for other optional values`, named.Obj().Name(), field.Name())
			}
		}
		structure.Fields = append(structure.Fields, &fieldModel{
			GoName: field.Name(), CName: names.CIdentifier(wireName), DartName: dartName, WireName: wireName,
			Type: mapped, Optional: isPointerType(field.Type()), Nullable: options.Nullable,
			NonFinal: options.NonFinal, DefaultValue: options.DefaultValue,
		})
	}
	return result, nil
}

// mapEmbedded resolves an embedded struct field into the Dart superclass, or
// returns nil when the embedded type is not a plain value struct and should
// stay an ordinary field.
// mapInterface bridges a named Go interface as a Dart
// `abstract interface class`. Its methods are declaration-only - a call on an
// interface value dispatches to the concrete Dart class - and values travel
// tagged with the index of their concrete type.
func (b *builder) mapInterface(original types.Type, named *types.Named, declared *types.Interface) (*wireType, error) {
	if existing := b.interfaceModels[named]; existing != nil {
		b.typeCache[original] = existing.Type
		return existing.Type, nil
	}
	decl := b.api.Types[named.Obj()]
	dartName := names.UpperCamel(named.Obj().Name())
	docs, sourceFile := "", ""
	if decl != nil {
		dartName, docs, sourceFile = decl.DartName, decl.Docs, decl.SourceFile
	}
	dartName = b.dartNameFor(named, dartName)
	result := b.newType(original, kindInterface, dartName)
	declaration := &interfaceModel{
		GoName: named.Obj().Name(), DartName: dartName, Docs: docs, SourceFile: sourceFile, Type: result,
	}
	result.Interface = declaration
	b.interfaceModels[named] = declaration
	b.unit.Interfaces = append(b.unit.Interfaces, declaration)

	for i := 0; i < declared.NumMethods(); i++ {
		method := declared.Method(i)
		if !method.Exported() {
			return nil, fmt.Errorf("interface %s declares unexported method %s, which cannot be bridged", named.Obj().Name(), method.Name())
		}
		var directive *model.InterfaceMethod
		if decl != nil {
			directive = decl.Methods[method.Name()]
		}
		if directive != nil && directive.Ignore {
			continue
		}
		bridged, err := b.mapInterfaceMethod(named, method, directive)
		if err != nil {
			return nil, err
		}
		declaration.Methods = append(declaration.Methods, bridged)
	}

	if err := b.collectImplementors(named, declaration); err != nil {
		return nil, err
	}
	if len(declaration.Implementors) == 0 {
		return nil, fmt.Errorf("no bridged type implements interface %s; declare at least one so its values can cross the bridge", named.Obj().Name())
	}
	return result, nil
}

// mapInterfaceMethod builds the Dart declaration of one interface method. It
// reuses callModel so the interface and the implementations are rendered by
// exactly the same rules.
func (b *builder) mapInterfaceMethod(owner *types.Named, method *types.Func, directive *model.InterfaceMethod) (*callModel, error) {
	signature, _ := method.Type().(*types.Signature)
	if signature == nil {
		return nil, fmt.Errorf("interface %s method %s has no signature", owner.Obj().Name(), method.Name())
	}
	source := &model.Callable{
		Func: method, Signature: signature, DartName: names.LowerCamel(method.Name()),
		Mode: model.CallModeSync,
	}
	if directive != nil {
		source.DartName, source.Mode, source.Docs = directive.DartName, directive.Mode, directive.Docs
	}
	declaration, err := b.mapCallable(source)
	if err != nil {
		return nil, fmt.Errorf("interface %s method %s: %w", owner.Obj().Name(), method.Name(), err)
	}
	return declaration, nil
}

// collectImplementors finds every bridged type in the input package that
// satisfies the interface. The order follows the declaration order so the
// wire tags stay stable across runs.
func (b *builder) collectImplementors(iface *types.Named, declaration *interfaceModel) error {
	declared := iface.Underlying().(*types.Interface)
	candidates := make([]*types.TypeName, 0, len(b.api.Types))
	for object := range b.api.Types {
		candidates = append(candidates, object)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Pos() < candidates[j].Pos() })
	for _, object := range candidates {
		named, ok := object.Type().(*types.Named)
		if !ok || named == iface {
			continue
		}
		if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
			continue
		}
		// A value struct exposes its pointer-receiver methods on the Dart
		// class too, so either form counts as an implementation.
		if !types.Implements(named, declared) && !types.Implements(types.NewPointer(named), declared) {
			continue
		}
		var mapped *wireType
		var err error
		if b.classifyStruct(named) == classOpaque {
			mapped, err = b.mapType(types.NewPointer(named))
		} else {
			mapped, err = b.mapType(named)
		}
		if err != nil {
			return fmt.Errorf("interface %s implementation %s: %w", iface.Obj().Name(), named.Obj().Name(), err)
		}
		dartName := strings.TrimSuffix(mapped.DartType, "?")
		declaration.Implementors = append(declaration.Implementors, &implementorModel{
			DartName: dartName, Type: mapped,
		})
		switch mapped.Kind {
		case kindStruct:
			mapped.Struct.Interfaces = appendUnique(mapped.Struct.Interfaces, declaration)
		case kindOpaque:
			mapped.Opaque.Interfaces = appendUnique(mapped.Opaque.Interfaces, declaration)
		}
	}
	return nil
}

func appendUnique(list []*interfaceModel, item *interfaceModel) []*interfaceModel {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}

func (b *builder) mapEmbedded(owner *types.Named, field *types.Var) (*structModel, error) {
	embedded := types.Unalias(field.Type())
	if _, isPointer := embedded.(*types.Pointer); isPointer {
		return nil, fmt.Errorf("struct %s embeds a pointer type; Dart cannot extend a nullable class, embed the value instead", owner.Obj().Name())
	}
	named, ok := embedded.(*types.Named)
	if !ok {
		return nil, nil
	}
	if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
		return nil, nil
	}
	if b.hasDedicatedMapping(named) {
		return nil, nil
	}
	if b.classifyStruct(named) == classOpaque {
		return nil, fmt.Errorf("struct %s embeds GoOpaque struct %s, which has no fields to inherit", owner.Obj().Name(), named.Obj().Name())
	}
	mapped, err := b.mapType(named)
	if err != nil {
		return nil, fmt.Errorf("embedded %s: %w", named.Obj().Name(), err)
	}
	return mapped.Struct, nil
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
	dartName = b.dartNameFor(named, dartName)
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

func (b *builder) ensureNamedSupported(named *types.Named) error {
	if named.Obj() == nil || named.Obj().Pkg() == nil {
		return nil
	}
	if named.Obj().Pkg() != b.api.Package.Types {
		if declared, ok := named.Underlying().(*types.Interface); ok && !declared.Empty() {
			return fmt.Errorf("external named interface %s.%s cannot be bridged automatically", named.Obj().Pkg().Path(), named.Obj().Name())
		}
		b.registerExternalPackage(named.Obj().Pkg())
		return nil
	}
	if b.api.IgnoredTypes[named.Obj().Name()] {
		return fmt.Errorf("type %s is marked fgb(ignore) but is used by the bridged API", named.Obj().Name())
	}
	return nil
}

// registerExternalPackage gives every dependency package a generated alias.
// Generated aliases avoid collisions with the runtime imports and with two Go
// packages that happen to share the same declared package name.
func (b *builder) registerExternalPackage(pkg *types.Package) {
	if pkg == nil || pkg.Path() == "" || pkg == b.api.Package.Types {
		return
	}
	if _, exists := b.unit.GoPackageAliases[pkg.Path()]; exists {
		return
	}
	alias := fmt.Sprintf("fgbext%d", len(b.unit.ExternalImports))
	b.unit.GoPackageAliases[pkg.Path()] = alias
	b.unit.ExternalImports = append(b.unit.ExternalImports, goImportModel{Alias: alias, Path: pkg.Path()})
}

// dartNameFor keeps the Go declaration name when it is unambiguous. Input
// package declarations are reserved before mapping starts, so an external
// collision receives a package-prefixed name such as models.User -> ModelsUser.
func (b *builder) dartNameFor(named *types.Named, preferred string) string {
	if cached := b.dartNames[named]; cached != "" {
		return cached
	}
	owner := b.dartTypeNames[preferred]
	if owner == nil || owner == named {
		b.dartTypeNames[preferred] = named
		b.dartNames[named] = preferred
		return preferred
	}
	prefix := "External"
	if named != nil && named.Obj() != nil && named.Obj().Pkg() != nil {
		prefix = names.UpperCamel(named.Obj().Pkg().Name())
	}
	base := prefix + preferred
	candidate := base
	for suffix := 2; b.dartTypeNames[candidate] != nil && b.dartTypeNames[candidate] != named; suffix++ {
		candidate = fmt.Sprintf("%s%d", base, suffix)
	}
	b.dartTypeNames[candidate] = named
	b.dartNames[named] = candidate
	return candidate
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

// isContextType matches context.Context, which the bridge supplies rather than
// taking from Dart.
func isContextType(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	return isNamed(named, "context", "Context")
}

func isTime(named *types.Named) bool {
	return isNamed(named, "time", "Time")
}

func isDuration(named *types.Named) bool {
	return isNamed(named, "time", "Duration")
}

func isBigInt(named *types.Named) bool {
	return isNamed(named, "math/big", "Int")
}

func isInternetIP(named *types.Named) bool { return isNamed(named, "net/netip", "Addr") }

// isIPPrefix matches net/netip.Prefix, which bridges as its CIDR text form.
func isIPPrefix(named *types.Named) bool { return isNamed(named, "net/netip", "Prefix") }

// isURL matches net/url.URL, which bridges as its text form.
func isURL(named *types.Named) bool { return isNamed(named, "net/url", "URL") }

// hasDedicatedMapping reports whether mapType translates a named type through a
// built-in rule instead of bridging it as a declared struct. Such a type must
// never reach classifyStruct: several of them (netip.Addr, netip.Prefix,
// url.URL, the atomic wrappers) keep all of their state unexported and would
// otherwise be mistaken for an untranslatable struct.
func (b *builder) hasDedicatedMapping(named *types.Named) bool {
	if named == nil {
		return false
	}
	if isTime(named) || isDuration(named) || isBigInt(named) ||
		isInternetIP(named) || isIPPrefix(named) || isURL(named) || isUUID(named) {
		return true
	}
	if b.isDartOpaque(named) || b.isStreamSink(named) {
		return true
	}
	_, isAtomic := atomicValueType(named)
	return isAtomic
}

func isUUID(named *types.Named) bool { return isNamed(named, "github.com/gofrs/uuid/v5", "UUID") }

// isSupportType matches a type from the generated support package, whose
// import path depends on the module that owns it.
func (b *builder) isSupportType(named *types.Named, name string) bool {
	return b.supportImportPath != "" && isNamed(named, b.supportImportPath, name)
}

func (b *builder) isDartOpaque(named *types.Named) bool {
	return b.isSupportType(named, "DartOpaque")
}

func (b *builder) isStreamSink(named *types.Named) bool {
	return b.isSupportType(named, "StreamSink") &&
		named.TypeArgs() != nil && named.TypeArgs().Len() == 1
}

// atomicValueType recognizes atomic wrapper structs by behavior rather than a
// hard-coded dependency list. Both sync/atomic.Int64 and wrappers such as
// mihomo/common/atomic.Int64 expose Load() T and Store(T), and Dart should see
// T directly instead of a synthetic empty AtomicInt64 class.
func atomicValueType(named *types.Named) (types.Type, bool) {
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil || named.Obj().Pkg().Name() != "atomic" {
		return nil, false
	}
	methods := types.NewMethodSet(types.NewPointer(named))
	loadSelection := methods.Lookup(nil, "Load")
	storeSelection := methods.Lookup(nil, "Store")
	if loadSelection == nil || storeSelection == nil {
		return nil, false
	}
	load, _ := loadSelection.Obj().Type().(*types.Signature)
	store, _ := storeSelection.Obj().Type().(*types.Signature)
	if load == nil || store == nil || load.Params().Len() != 0 || load.Results().Len() != 1 ||
		store.Params().Len() != 1 || store.Results().Len() != 0 {
		return nil, false
	}
	valueType := types.Unalias(load.Results().At(0).Type())
	if _, ok := valueType.(*types.Basic); !ok || !types.Identical(valueType, types.Unalias(store.Params().At(0).Type())) {
		return nil, false
	}
	return valueType, true
}

// containsStreamSink reports whether a stream sink is reachable from typ.
// Sinks only travel Dart -> Go, so a result carrying one is a generation
// error rather than a runtime surprise.
func containsStreamSink(typ *wireType, seen map[int]bool) bool {
	if typ == nil || seen[typ.ID] {
		return false
	}
	seen[typ.ID] = true
	switch typ.Kind {
	case kindStreamSink:
		return true
	case kindPointer, kindSlice, kindArray, kindBytes, kindInt32List, kindInt64List, kindFloat64List:
		return containsStreamSink(typ.Elem, seen)
	case kindMap:
		return containsStreamSink(typ.Key, seen) || containsStreamSink(typ.Elem, seen)
	case kindStruct:
		for _, field := range typ.Struct.allFields() {
			if containsStreamSink(field.Type, seen) {
				return true
			}
		}
		return false
	case kindNamed:
		return containsStreamSink(typ.Named.Underlying, seen)
	case kindAtomic:
		return containsStreamSink(typ.Atomic.Value, seen)
	default:
		return false
	}
}

func isNamed(named *types.Named, packagePath, name string) bool {
	return named != nil && named.Obj() != nil && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}

func validMapKey(typ *wireType) bool {
	switch typ.Kind {
	case kindBool, kindString, kindSigned, kindUnsigned, kindFloat, kindDuration, kindIPPrefix, kindURL, kindNamed:
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
