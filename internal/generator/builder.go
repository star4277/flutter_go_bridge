package generator

import (
	"errors"
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/model"
	"github.com/star4277/flutter_go_bridge/internal/names"
	"golang.org/x/tools/go/packages"
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
	api               *model.API
	config            config.Resolved
	unit              *unit
	typeCache         map[types.Type]*wireType
	structClasses     map[*types.Named]structClass
	namedModels       map[*types.Named]*namedModel
	structModels      map[*types.Named]*structModel
	opaqueModels      map[*types.Named]*opaqueModel
	syntheticOpaques  map[string]*opaqueModel
	interfaceModels   map[*types.Named]*interfaceModel
	dependencyMethods map[*types.Named]bool
	// Dart has one library namespace across the mutually importing generated
	// source files. Reserve input-package names up front, then disambiguate
	// reachable external declarations without renaming the user's own types.
	dartTypeNames      map[string]*types.Named
	dartNames          map[*types.Named]string
	syntheticDartNames map[string]bool
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
		opaqueModels:       map[*types.Named]*opaqueModel{},
		syntheticOpaques:   map[string]*opaqueModel{},
		interfaceModels:    map[*types.Named]*interfaceModel{},
		dependencyMethods:  map[*types.Named]bool{},
		dartTypeNames:      map[string]*types.Named{},
		dartNames:          map[*types.Named]string{},
		syntheticDartNames: map[string]bool{},
	}
	for _, declaration := range api.Types {
		if declaration != nil && declaration.Named != nil {
			// Ambient names are never handed out as-is, so reserving one here
			// would only keep dartNameFor from noticing the collision.
			if names.IsDartAmbientType(declaration.DartName) {
				continue
			}
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
	if err := b.mapDependencyImplementorMethods(); err != nil {
		return nil, b.warnings, err
	}
	sort.SliceStable(b.unit.Types, func(i, j int) bool { return b.unit.Types[i].ID < b.unit.Types[j].ID })
	b.selectInterfaceToStringMethods()
	if err := b.propagateInterfaceMethodShapes(); err != nil {
		return nil, b.warnings, err
	}
	b.selectValueToStringMethods()
	if err := b.checkMethodOverrides(); err != nil {
		return nil, b.warnings, err
	}
	if err := b.checkInterfaceImplementations(); err != nil {
		return nil, b.warnings, err
	}
	return b.unit, b.warnings, nil
}

func (b *builder) selectInterfaceToStringMethods() {
	for _, declaration := range b.unit.Interfaces {
		b.selectToStringForMethods(declaration.GoName, declaration.Methods)
		b.disambiguateSelectedMethods(declaration.GoName, declaration.Methods, false)
	}
}

// selectValueToStringMethods reserves Dart's Object.toString member for the
// strongest eligible Go representation after interface directives have shaped
// the concrete calls. Promoted methods participate in a child's selection.
func (b *builder) selectValueToStringMethods() {
	for _, structure := range b.unit.Structs {
		methods := structureAndPromotedMethods(structure)
		b.selectToStringForMethods(structure.GoName, methods)
		structure.LocalToString = !hasToStringMethod(methods)
		b.disambiguateSelectedMethods(structure.GoName, structure.Methods, hasToStringMethod(methods) || structure.LocalToString)
	}
	for _, named := range b.unit.Named {
		b.selectToStringForMethods(named.GoName, named.Methods)
		if named.Enum {
			named.LocalToString = false
			b.disambiguateSelectedMethods(named.GoName, named.Methods, true)
			b.disambiguateEnumMethods(named)
		} else {
			b.normalizeNamedExtensionMethods(named)
		}
	}
	for _, opaque := range b.unit.Opaques {
		b.selectToStringForMethods(opaque.GoName, opaque.Methods)
		b.disambiguateSelectedMethods(opaque.GoName, opaque.Methods, true)
	}
}

// mapDependencyImplementorMethods exposes the small set of Go methods that
// can provide a concrete object's string representation. Dependency packages
// do not carry parser directives, so their methods use the ordinary sync call
// mode and the existing eligibility checks decide the final Dart override.
func (b *builder) mapDependencyImplementorMethods() error {
	namedTypes := make([]*types.Named, 0, len(b.dependencyMethods))
	for named := range b.dependencyMethods {
		namedTypes = append(namedTypes, named)
	}
	sort.Slice(namedTypes, func(i, j int) bool {
		left, right := namedTypes[i].Obj(), namedTypes[j].Obj()
		if left.Pkg().Path() != right.Pkg().Path() {
			return left.Pkg().Path() < right.Pkg().Path()
		}
		return left.Name() < right.Name()
	})
	for _, named := range namedTypes {
		methodSet := types.NewMethodSet(types.NewPointer(named))
		for index := 0; index < methodSet.Len(); index++ {
			method, ok := methodSet.At(index).Obj().(*types.Func)
			if !ok || method == nil || !method.Exported() || !isStringRepresentationMethod(method.Name()) {
				continue
			}
			signature, ok := method.Type().(*types.Signature)
			if !ok || signature.TypeParams() != nil && signature.TypeParams().Len() != 0 {
				continue
			}
			source := &model.Callable{
				Func: method, Signature: signature, Receiver: named,
				PointerRecv: methodReceiverIsPointer(method),
				DartName:    names.LowerCamel(method.Name()), Mode: model.CallModeSync,
			}
			call, err := b.mapCallable(source)
			if err != nil {
				b.warnings = append(b.warnings, fmt.Errorf("dependency method %s.%s was not bridged: %w", named.Obj().Name(), method.Name(), err))
				continue
			}
			call.WireName = dependencyMethodWireName(named, method.Name())
			call.ID = len(b.unit.Calls)
			call.Codec = preferredCodecForCall(call, b.unit.codecSupport)
			b.unit.Calls = append(b.unit.Calls, call)
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
			}
		}
	}
	return nil
}

func isStringRepresentationMethod(name string) bool {
	switch name {
	case "ToString", "String", "MarshalJSON":
		return true
	default:
		return false
	}
}

func methodReceiverIsPointer(method *types.Func) bool {
	if method == nil {
		return false
	}
	signature, _ := method.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	_, pointer := types.Unalias(signature.Recv().Type()).(*types.Pointer)
	return pointer
}

func dependencyMethodWireName(named *types.Named, method string) string {
	return fmt.Sprintf("dependency:%s:%s.%s", named.Obj().Pkg().Path(), named.Obj().Name(), method)
}

// Extension types cannot declare members inherited from Object, including
// toString. Keep Go-backed string representations callable under ordinary
// names and leave Object.toString to Dart's extension-type implementation.
func (b *builder) normalizeNamedExtensionMethods(named *namedModel) {
	named.LocalToString = false
	for _, method := range named.Methods {
		if !method.ToString {
			continue
		}
		method.ToString = false
		method.ToStringFormat = toStringNone
		switch method.GoName {
		case "String":
			method.DartName = "asString"
		case "ToString":
			method.DartName = "toStringValue"
		case "MarshalJSON":
			method.DartName = names.LowerCamel(method.GoName)
		default:
			method.DartName = names.LowerCamel(method.GoName)
		}
		b.warnings = append(b.warnings, fmt.Errorf("%s.%s cannot override Dart Object.toString on an extension type; exposed as %s", named.GoName, method.GoName, method.DartName))
	}
	b.disambiguateSelectedMethods(named.GoName, named.Methods, true)
}

func (b *builder) disambiguateEnumMethods(named *namedModel) {
	used := map[string]bool{"values": true, "value": true, "name": true, "index": true}
	for _, method := range named.Methods {
		if method.Operator != "" || method.ToString {
			continue
		}
		base := method.DartName
		candidate := base
		for suffix := 2; used[candidate]; suffix++ {
			candidate = fmt.Sprintf("%s%d", base, suffix)
		}
		if candidate != base {
			b.warnings = append(b.warnings, fmt.Errorf("enum %s method Dart name %q is reserved by Dart enum behavior; renamed %s to %q", named.GoName, base, method.GoName, candidate))
			method.DartName = candidate
		}
		used[candidate] = true
	}
}

func structureAndPromotedMethods(structure *structModel) []*callModel {
	var methods []*callModel
	for current := structure; current != nil; current = current.Super {
		methods = append(methods, current.Methods...)
	}
	return methods
}

func hasToStringMethod(methods []*callModel) bool {
	for _, method := range methods {
		if method.Operator != "" {
			continue
		}
		if method.ToString {
			return true
		}
	}
	return false
}

func (b *builder) selectToStringForMethods(owner string, methods []*callModel) {
	var explicit, stringer, marshal *callModel
	for _, method := range methods {
		switch method.GoName {
		case "ToString":
			if explicit == nil {
				if b.eligibleToString(method) {
					explicit = method
				} else {
					b.warnings = append(b.warnings, fmt.Errorf("%s.%s cannot be Dart toString because it has required parameters; falling back to the next representation", owner, method.GoName))
					method.DartName = "toStringWithArgs"
				}
			}
		case "String":
			if stringer == nil {
				if b.eligibleStringer(method) {
					stringer = method
				} else if method.ErrorCount == 0 && len(method.Results) == 1 && method.Results[0].Type.Kind == kindString {
					b.warnings = append(b.warnings, fmt.Errorf("%s.%s cannot be Dart toString because it has required parameters; falling back to the next representation", owner, method.GoName))
				}
			}
			method.DartName = "asString"
		case "MarshalJSON":
			if marshal == nil {
				if b.eligibleMarshalJSON(method) {
					marshal = method
				} else if len(method.Results) == 1 && method.Results[0].Type.Kind == kindBytes {
					b.warnings = append(b.warnings, fmt.Errorf("%s.%s cannot be Dart toString because it has required parameters; falling back to local fields", owner, method.GoName))
				}
			}
		}
	}
	selected := explicit
	format := toStringText
	if selected == nil {
		selected = stringer
	}
	if selected == nil {
		selected = marshal
		format = toStringJSON
	}
	if selected == nil {
		return
	}
	selected.ToString = true
	selected.ToStringFormat = format
	selected.DartName = "toString"
}

// disambiguateSelectedMethods runs after special names have been assigned.
// Special `toString`/`asString` members keep their specified API names; any
// ordinary method occupying one of those names receives the numeric suffix.
func (b *builder) disambiguateSelectedMethods(owner string, methods []*callModel, reserveToString bool) {
	reserved := map[string]*callModel{}
	if reserveToString {
		reserved["toString"] = nil
	}
	for _, method := range methods {
		if method.ToString {
			reserved["toString"] = method
		}
		if method.GoName == "String" && !method.ToString {
			reserved["asString"] = method
		}
	}
	used := map[string]bool{}
	for name := range reserved {
		used[name] = true
	}
	for _, method := range methods {
		if method.Operator != "" {
			continue
		}
		name := method.DartName
		if holder, special := reserved[name]; special && holder == method {
			continue
		}
		candidate := name
		for suffix := 2; used[candidate]; suffix++ {
			candidate = fmt.Sprintf("%s%d", name, suffix)
		}
		if candidate != name {
			b.warnings = append(b.warnings, fmt.Errorf("%s method Dart name %q is reserved by generated toString behavior; renamed %s to %q", owner, name, method.GoName, candidate))
			method.DartName = candidate
		}
		used[candidate] = true
	}
}

func (b *builder) eligibleToString(call *callModel) bool {
	if call == nil || call.Mode != model.CallModeSync || call.ErrorCount != 0 || len(call.Results) != 1 || call.Results[0].Type.Kind != kindString {
		return false
	}
	return paramsOptional(call.Params)
}

func (b *builder) eligibleStringer(call *callModel) bool {
	return call != nil && len(call.Params) == 0 && call.Mode == model.CallModeSync && call.ErrorCount == 0 && len(call.Results) == 1 && call.Results[0].Type.Kind == kindString
}

func (b *builder) eligibleMarshalJSON(call *callModel) bool {
	if call == nil || call.Mode != model.CallModeSync || len(call.Params) != 0 || call.ErrorCount != 1 || len(call.Results) != 1 || call.Results[0].GoIndex != 0 || call.Results[0].Type.Kind != kindBytes {
		return false
	}
	return paramsOptional(call.Params)
}

func paramsOptional(params []*paramModel) bool {
	for _, param := range params {
		if !param.Nullable && !isPointerType(param.Type.Original) {
			return false
		}
	}
	return true
}

// propagateInterfaceMethodShapes copies the Dart name and call mode of an
// interface method onto the implementations rendered under it.
//
// A directive on an interface method only shapes the declaration. Dart matches
// an implementation by name and signature, so `//fgb:rename` on the interface
// alone produced a class that claimed to implement it while exposing the
// original name, and `//fgb:async` on the interface alone produced an invalid
// override. Repeating the directive on every implementation would be silent
// busywork, so the interface -- which owns the Dart contract -- wins.
func (b *builder) propagateInterfaceMethodShapes() error {
	// claimed remembers which interface already shaped a Go method, so two
	// interfaces asking for different Dart shapes are reported instead of
	// letting the last one win.
	type claim struct {
		iface     *interfaceModel
		method    *callModel
		signature string
	}
	claimed := map[*callModel]claim{}
	for _, declaration := range b.unit.Interfaces {
		for _, declared := range declaration.Methods {
			for _, implementor := range declaration.Implementors {
				if implementor.DecodeOnly {
					continue
				}
				for _, concrete := range implementorMethods(implementor.Type) {
					if concrete.GoName != declared.GoName {
						continue
					}
					shape := declared.DartName + " " + string(declared.Mode)
					if previous, seen := claimed[concrete]; seen && previous.signature != shape {
						return fmt.Errorf(
							"%s.%s implements both %s.%s and %s.%s, which ask for different Dart shapes (%s vs %s); make the two interface method directives agree",
							implementor.DartName, concrete.GoName,
							previous.iface.GoName, previous.method.GoName,
							declaration.GoName, declared.GoName,
							previous.signature, shape)
					}
					claimed[concrete] = claim{iface: declaration, method: declared, signature: shape}
					concrete.DartName = declared.DartName
					concrete.Mode = declared.Mode
					// The same principle applies to an operator, and more
					// sharply: an operator provides no named member at all, so
					// the `implements` clause would be left without the one the
					// interface promises and the generated Dart would not
					// compile. An interface declaration never becomes an
					// operator itself - mapInterfaceMethod builds it without a
					// receiver, and the operand type of a method on an
					// interface is the interface, never the implementation.
					if concrete.Operator != "" {
						b.warnings = append(b.warnings, fmt.Errorf(
							"%s.%s matches Dart operator %s, but it implements %s.%s, which declares a method; the interface owns the Dart shape, so it stays %s()",
							implementor.DartName, concrete.GoName, concrete.Operator,
							declaration.GoName, declared.GoName, declared.DartName))
						concrete.Operator = ""
					}
				}
			}
		}
	}
	return nil
}

// implementorMethods collects the bridged methods a Dart implementation class
// exposes, including the ones promoted from an embedded struct, because Dart
// accepts an inherited member as the implementation of an interface method.
//
// collectImplementors only ever registers value structs and GoOpaque handles,
// so anything else is a future shape that owns no methods yet.
func implementorMethods(mapped *wireType) []*callModel {
	if mapped.Kind == kindOpaque {
		return mapped.Opaque.Methods
	}
	// Anything else registered as an implementor is a value struct. A kind
	// without a struct model simply owns no methods, so the walk ends at once.
	var methods []*callModel
	for structure := mapped.Struct; structure != nil; structure = structure.Super {
		methods = append(methods, structure.Methods...)
	}
	return methods
}

// checkInterfaceImplementations rejects an implementation whose Dart method
// shape does not match the interface it is rendered with. Go is satisfied as
// long as the Go signatures line up, but the bridge adds a per-declaration
// call mode: an interface method marked //fgb:async becomes Future<T> while an
// unmarked implementation stays synchronous, and Dart rejects that override.
// Reporting it here names both declarations, where dart analyze would only
// point at generated code.
func (b *builder) checkInterfaceImplementations() error {
	for _, declaration := range b.unit.Interfaces {
		// Dependency interfaces are marker-only, so they promise nothing.
		if len(declaration.Methods) == 0 {
			continue
		}
		for _, implementor := range declaration.Implementors {
			// A decode-only entry is another Go spelling of a Dart class that
			// is already checked through its canonical implementor.
			if implementor.DecodeOnly {
				continue
			}
			available := map[string]*callModel{}
			for _, method := range implementorMethods(implementor.Type) {
				// An operator provides no named member, so it can never satisfy
				// an interface method declaration.
				if method.Operator != "" {
					continue
				}
				if _, seen := available[method.DartName]; !seen {
					available[method.DartName] = method
				}
			}
			for _, declared := range declaration.Methods {
				concrete, found := available[declared.DartName]
				if !found {
					return fmt.Errorf(
						"%s is rendered as a Dart implementation of %s but bridges no method for %s.%s; drop the //fgb:ignore on that method, or keep the type out of the interface",
						implementor.DartName, declaration.DartName, declaration.GoName, declared.GoName)
				}
				if dartMethodSignature(concrete) != dartMethodSignature(declared) {
					return fmt.Errorf(
						"%s.%s implements %s.%s with a different Dart signature (%s vs %s); the call mode is taken from the interface, so the difference comes from another directive such as //fgb:nullable",
						implementor.DartName, concrete.GoName, declaration.GoName, declared.GoName,
						dartMethodSignature(concrete), dartMethodSignature(declared))
				}
			}
		}
	}
	return nil
}

func (b *builder) disambiguateMethod(call *callModel, existing []*callModel) {
	// An operator occupies no ordinary name, so it neither needs renaming nor
	// blocks a method that happens to share its Go name's Dart spelling.
	if call.Operator != "" {
		return
	}
	base := call.DartName
	candidate := base
	for suffix := 2; ; suffix++ {
		collision := false
		for _, other := range existing {
			if other.Operator != "" {
				continue
			}
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
				if _, seen := inherited[call.dartMember()]; !seen {
					inherited[call.dartMember()] = call
					inheritedOwner[call.dartMember()] = super
				}
			}
		}
		for _, call := range structure.Methods {
			promoted, shadows := inherited[call.dartMember()]
			if !shadows {
				continue
			}
			if dartMethodSignature(call) != dartMethodSignature(promoted) {
				return fmt.Errorf(
					"%s.%s shadows %s.%s with a different signature; Dart cannot express that on a subclass, so rename one of them with //fgb:rename",
					structure.GoName, call.GoName, inheritedOwner[call.dartMember()].GoName, promoted.GoName)
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
	if call.Operator != "" {
		return prefix + dartResultType(call) + " operator " + call.Operator + " (" + dartOperatorParams(call) + ")"
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
		if mapped.usesPointerCodec(map[int]bool{}) {
			if _, pointer := types.Unalias(variable.Type()).(*types.Pointer); !pointer {
				return nil, fmt.Errorf("result %d contains atomic state and must be returned by pointer to avoid copying sync/atomic state", i)
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
	// A method whose name and signature match one of the Dart operators is
	// rendered as that operator instead of an ordinary method.
	call.Operator = dartOperatorFor(source)
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
			switch b.atomicCollectionShape(named) {
			case atomicCollectionOpaque:
				return b.mapSyntheticOpaque(original, named)
			case atomicCollectionReject:
				return nil, fmt.Errorf("type %s contains atomic array values and cannot cross the bridge without copying sync/atomic state", named.Obj().Name())
			}
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
		if b.atomicCollectionShape(typ) == atomicCollectionOpaque {
			return b.mapSyntheticOpaque(original, typ)
		}
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
		if b.atomicCollectionShape(typ) == atomicCollectionReject {
			return nil, errors.New("arrays containing atomic values cannot cross the bridge without copying sync/atomic state; use a slice or an opaque named wrapper")
		}
		elem, err := b.mapType(typ.Elem())
		if err != nil {
			return nil, err
		}
		result := b.newType(original, kindArray, "List<"+elem.DartType+">")
		result.Elem = elem
		result.Length = typ.Len()
		return result, nil
	case *types.Map:
		if b.atomicCollectionShape(typ) == atomicCollectionOpaque {
			return b.mapSyntheticOpaque(original, typ)
		}
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
		if isErrorType(typ) {
			// Go's predeclared error is ubiquitous, but dependency implementor
			// discovery cannot expose its methods. Carry the useful error text
			// across the bridge instead of emitting an empty marker interface.
			return b.newSimpleType(original, kindError, "String?"), nil
		}
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
		if isShopspringDecimal(typ) {
			b.unit.UsesDecimal = true
			b.unit.UsesShopspringDecimal = true
			return b.newSimpleType(original, kindDecimal, "Decimal"), nil
		}
		if isAPDDecimal(typ) {
			b.unit.UsesDecimal = true
			b.unit.UsesAPDDecimal = true
			return b.newSimpleType(original, kindDecimal, "Decimal"), nil
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
		switch b.atomicCollectionShape(typ) {
		case atomicCollectionOpaque:
			return b.mapSyntheticOpaque(original, typ)
		case atomicCollectionReject:
			return nil, fmt.Errorf("type %s contains atomic array values and cannot cross the bridge without copying sync/atomic state", typ.Obj().Name())
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
	if decl != nil {
		model.Enum = decl.Enum
	}
	result.Named = model
	b.namedModels[named] = model
	b.unit.Named = append(b.unit.Named, model)
	underlying, err := b.mapType(named.Underlying())
	if err != nil {
		return nil, err
	}
	model.Underlying = underlying
	constants := b.api.Constants[named]
	if model.Enum {
		if underlying.Kind != kindString && underlying.Kind != kindSigned && underlying.Kind != kindUnsigned {
			return nil, fmt.Errorf("enum %s requires a string or integer underlying type, got %s", model.GoName, underlying.Kind)
		}
		if len(constants) == 0 {
			return nil, fmt.Errorf("enum %s must declare at least one exported typed constant", model.GoName)
		}
		for index := 0; index < len(constants); index++ {
			for other := index + 1; other < len(constants); other++ {
				if constant.Compare(constants[index].Object.Val(), token.EQL, constants[other].Object.Val()) {
					return nil, fmt.Errorf("enum %s has duplicate underlying value %s in %s and %s", model.GoName, constants[index].Object.Val().ExactString(), constants[index].Object.Name(), constants[other].Object.Name())
				}
			}
		}
	} else if decl != nil && likelyEnum(named, constants, underlying) {
		b.warnings = append(b.warnings, fmt.Errorf("type %s looks like an enum; add //fgb:enum to opt in", model.GoName))
	}
	usedCases := map[string]int{}
	for _, item := range constants {
		literal, isConst, err := dartConstantLiteral(item.Object.Val(), underlying)
		if err != nil {
			b.warnings = append(b.warnings, fmt.Errorf("constant %s.%s: %w", item.Object.Pkg().Path(), item.Object.Name(), err))
			continue
		}
		dartCase := item.DartName
		if model.Enum {
			dartCase = enumCaseName(model.GoName, item)
			dartCase = uniqueEnumCaseName(dartCase, usedCases, model.GoName, item.Object.Name(), &b.warnings)
			if underlying.DartType == "BigInt" {
				literal = strconv.Quote(item.Object.Val().ExactString())
				isConst = true
			}
		}
		model.Constants = append(model.Constants, &constantModel{
			GoName: item.Object.Name(), DartName: dartCase, Docs: item.Docs,
			DartLiteral: literal, IsConst: isConst,
		})
	}
	return result, nil
}

func likelyEnum(named *types.Named, constants []*model.Constant, underlying *wireType) bool {
	if named == nil || len(constants) == 0 || underlying == nil {
		return false
	}
	if underlying.Kind != kindString && underlying.Kind != kindSigned && underlying.Kind != kindUnsigned {
		return false
	}
	prefix := named.Obj().Name()
	for _, item := range constants {
		if !strings.HasPrefix(item.Object.Name(), prefix) {
			return false
		}
	}
	return true
}

func enumCaseName(typeName string, item *model.Constant) string {
	name := item.DartName
	if item.Renamed {
		return name
	}
	defaultName := names.LowerCamel(item.Object.Name())
	if name == defaultName {
		if remainder := strings.TrimPrefix(item.Object.Name(), typeName); remainder != item.Object.Name() && remainder != "" {
			return names.LowerCamel(remainder)
		}
	}
	return name
}

func uniqueEnumCaseName(base string, used map[string]int, typeName, goName string, warnings *[]error) string {
	reserved := map[string]bool{"values": true, "index": true, "name": true, "value": true}
	candidate := base
	if reserved[candidate] || used[candidate] != 0 {
		count := used[base]
		if count == 0 {
			count = 1
		}
		for {
			count++
			candidate = fmt.Sprintf("%s%d", base, count)
			if !reserved[candidate] && used[candidate] == 0 {
				break
			}
		}
		*warnings = append(*warnings, fmt.Errorf("enum %s case %s has reserved or duplicate Dart name %q; renamed to %q", typeName, goName, base, candidate))
	}
	used[base]++
	if candidate != base {
		used[candidate]++
	}
	return candidate
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
		if field.Embedded() {
			if _, pointer := types.Unalias(field.Type()).(*types.Pointer); pointer && named.Obj().Pkg() != b.api.Package.Types {
				b.warnings = append(b.warnings, fmt.Errorf(
					"struct %s bridges as GoOpaque because it embeds pointer field %s, which Dart cannot represent as inheritance",
					named.Obj().Name(), field.Name()))
				class = classOpaque
				break
			}
		}
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
		if _, directAtomic := atomicValueType(typ); !directAtomic {
			if _, isStruct := typ.Underlying().(*types.Struct); isStruct && atomicValueByCopy(typ, map[types.Type]bool{}) {
				return fmt.Sprintf("holds value struct %s with atomic state (use *%s to keep a Dart value class)", typ.Obj().Name(), typ.Obj().Name())
			}
		}
		if b.hasDedicatedMapping(typ) {
			return ""
		}
		if declared, isInterface := typ.Underlying().(*types.Interface); isInterface {
			if declared.Empty() {
				return ""
			}
			// Named interfaces are bridged as tagged unions. Dependency
			// interfaces discover their concrete types from the loaded package
			// graph, so they are translatable in fields as well as signatures.
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

	// Methods are callable only for interfaces declared in the input package:
	// those declarations have source directives and their implementations can
	// own generated method entrypoints. A dependency interface is an open-world
	// serialization union instead; emitting its methods would make the Dart
	// implementations promise calls for which no bridge entrypoint exists.
	if decl != nil {
		for i := 0; i < declared.NumMethods(); i++ {
			method := declared.Method(i)
			if !method.Exported() {
				return nil, fmt.Errorf("interface %s declares unexported method %s, which cannot be bridged", named.Obj().Name(), method.Name())
			}
			directive := decl.Methods[method.Name()]
			if directive != nil && directive.Ignore {
				continue
			}
			bridged, err := b.mapInterfaceMethod(named, method, directive)
			if err != nil {
				return nil, err
			}
			declaration.Methods = append(declaration.Methods, bridged)
		}
	}
	if decl == nil && declared.NumMethods() > 0 && !isErrorType(named) {
		b.warnings = append(b.warnings, fmt.Errorf(
			"interface %s.%s crosses the bridge as a marker-only Dart interface: its %d methods are not callable from Dart because only interfaces declared in the input package get generated entrypoints",
			named.Obj().Pkg().Path(), named.Obj().Name(), declared.NumMethods()))
	}

	if err := b.collectImplementors(named, declaration); err != nil {
		return nil, err
	}
	if named.Obj().Pkg() != b.api.Package.Types {
		fallback := b.mapInterfaceOpaqueFallback(named, declaration)
		implementation := b.registerInterfaceImplementor(declaration, fallback, false)
		implementation.WireTag = stableWireTag(named) + "#opaque"
	}
	// Decide the wire-tag scheme once, after every implementor - including the
	// opaque fallback above - has been collected. Both the decoder branch
	// selector and the per-case tag literals read this field so they can never
	// disagree about whether an interface uses string content tags or integer
	// indices.
	declaration.UsesContentTags = named.Obj().Pkg() != b.api.Package.Types
	if named.Obj().Pkg() != b.api.Package.Types && len(declaration.Implementors) > 8 {
		b.warnings = append(b.warnings, fmt.Errorf(
			"interface %s.%s has %d generated wire members; prefer a narrower bridge surface or an opaque wrapper",
			named.Obj().Pkg().Path(), named.Obj().Name(), len(declaration.Implementors)))
	}
	if len(declaration.Implementors) == 0 {
		return nil, fmt.Errorf("no bridged type implements interface %s; declare at least one so its values can cross the bridge", named.Obj().Name())
	}
	return result, nil
}

// mapInterfaceOpaqueFallback creates the final union member for a dependency
// interface. Go dependencies may hide concrete implementations behind
// unexported, internal, generic, or runtime-registered types that generated Go
// code cannot name in a type switch. Boxing the interface value itself in the
// opaque registry preserves identity and lets Dart pass that same value back.
func (b *builder) mapInterfaceOpaqueFallback(named *types.Named, declaration *interfaceModel) *wireType {
	base := declaration.DartName + "Opaque"
	dartName := base
	for suffix := 2; b.dartTypeNames[dartName] != nil || b.syntheticDartNames[dartName]; suffix++ {
		dartName = fmt.Sprintf("%s%d", base, suffix)
	}
	b.syntheticDartNames[dartName] = true
	b.dartTypeNames[dartName] = named
	result := &wireType{
		ID: b.nextTypeID, Kind: kindOpaque, Original: named, DartType: dartName + "?",
	}
	b.nextTypeID++
	opaque := &opaqueModel{
		GoName: named.Obj().Name(), DartName: dartName, Type: result, Synthetic: true,
	}
	result.Opaque = opaque
	b.unit.Types = append(b.unit.Types, result)
	b.unit.Opaques = append(b.unit.Opaques, opaque)
	return result
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

// collectImplementors finds every bridged concrete type that satisfies the
// interface. Input interfaces preserve source declaration order for wire
// compatibility. Dependency interfaces use public-API reachability plus
// module-local discovery and stable package path plus type name tags.
func (b *builder) collectImplementors(iface *types.Named, declaration *interfaceModel) error {
	declared := iface.Underlying().(*types.Interface)
	candidates := b.interfaceImplementorCandidates(iface)
	for _, object := range candidates {
		named, ok := object.Type().(*types.Named)
		if !ok || named == iface {
			continue
		}
		if params := named.TypeParams(); params != nil && params.Len() != 0 {
			// An uninstantiated generic declaration cannot be named in a Go
			// type switch. Runtime instantiations use the interface fallback.
			continue
		}
		if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
			continue
		}
		// A value struct exposes its pointer-receiver methods on the Dart
		// class too, so either form counts as an implementation.
		valueImplements := types.Implements(named, declared)
		pointerImplements := types.Implements(types.NewPointer(named), declared)
		if !valueImplements && !pointerImplements {
			continue
		}
		if iface.Obj().Pkg() != b.api.Package.Types && named.Obj().Pkg() != b.api.Package.Types {
			b.dependencyMethods[named] = true
		}
		class := b.classifyStruct(named)
		var mapped *wireType
		var err error
		if class == classOpaque {
			mapped, err = b.mapType(types.NewPointer(named))
		} else if !valueImplements {
			// Pointer-receiver-only implementations must decode back to a
			// pointer in Go. Dart still sees the underlying value class.
			mapped, err = b.mapType(types.NewPointer(named))
		} else {
			mapped, err = b.mapType(named)
		}
		if err != nil {
			return fmt.Errorf("interface %s implementation %s: %w", iface.Obj().Name(), named.Obj().Name(), err)
		}
		implementor := b.registerInterfaceImplementor(declaration, mapped, false)
		if class == classOpaque && valueImplements {
			implementor.GoTypes = append(implementor.GoTypes, implementorGoType{
				Type: named, AddressValue: true,
			})
		}

		// Value-receiver methods make both T and *T valid Go dynamic types.
		// Keep a second decode tag so Go results accept either representation,
		// while Dart encodes the canonical value tag only.
		if class == classValue && valueImplements && pointerImplements {
			pointer, pointerErr := b.mapType(types.NewPointer(named))
			if pointerErr != nil {
				return fmt.Errorf("interface %s pointer implementation %s: %w", iface.Obj().Name(), named.Obj().Name(), pointerErr)
			}
			b.registerInterfaceImplementor(declaration, pointer, true)
		}
	}
	return nil
}

func (b *builder) registerInterfaceImplementor(declaration *interfaceModel, mapped *wireType, decodeOnly bool) *implementorModel {
	dartName := strings.TrimSuffix(mapped.DartType, "?")
	implementation := &implementorModel{
		DartName: dartName, Type: mapped, DecodeOnly: decodeOnly,
		GoTypes: []implementorGoType{{Type: mapped.Original}},
	}
	if named, ok := types.Unalias(declaration.Type.Original).(*types.Named); ok && named.Obj().Pkg() != b.api.Package.Types {
		implementation.WireTag = stableWireTag(mapped.Original)
	}
	declaration.Implementors = append(declaration.Implementors, implementation)
	switch mapped.Kind {
	case kindStruct:
		mapped.Struct.Interfaces = appendUnique(mapped.Struct.Interfaces, declaration)
	case kindOpaque:
		mapped.Opaque.Interfaces = appendUnique(mapped.Opaque.Interfaces, declaration)
	case kindPointer:
		if mapped.Elem.Kind == kindStruct {
			mapped.Elem.Struct.Interfaces = appendUnique(mapped.Elem.Struct.Interfaces, declaration)
		}
	}
	return implementation
}

func (b *builder) interfaceImplementorCandidates(iface *types.Named) []*types.TypeName {
	// Keep the existing declaration-order ABI for interfaces owned by the
	// input package.
	if iface.Obj().Pkg() == b.api.Package.Types {
		candidates := make([]*types.TypeName, 0, len(b.api.Types))
		for object := range b.api.Types {
			candidates = append(candidates, object)
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Pos() < candidates[j].Pos() })
		return candidates
	}

	// Dependency interfaces are open-world. Preserve explicitly reachable
	// implementations, then add declarations from loaded packages in the same
	// third-party module as the interface. The module boundary keeps unrelated
	// imports and standard-library implementations out of the union.
	candidateSet := map[*types.TypeName]bool{}
	for _, object := range b.reachableInterfaceImplementorCandidates() {
		candidateSet[object] = true
	}
	for _, object := range b.moduleInterfaceImplementorCandidates(iface) {
		candidateSet[object] = true
	}

	candidates := make([]*types.TypeName, 0, len(candidateSet))
	for object := range candidateSet {
		candidates = append(candidates, object)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.Pkg().Path() != right.Pkg().Path() {
			return left.Pkg().Path() < right.Pkg().Path()
		}
		return left.Name() < right.Name()
	})
	return candidates
}

func (b *builder) reachableInterfaceImplementorCandidates() []*types.TypeName {
	seen := map[*types.Named]bool{}
	var visit func(types.Type)
	visit = func(typ types.Type) {
		typ = types.Unalias(typ)
		switch typed := typ.(type) {
		case *types.Named:
			if typed.Obj() == nil || seen[typed] {
				return
			}
			seen[typed] = true
			if b.hasDedicatedMapping(typed) {
				return
			}
			if _, isInterface := typed.Underlying().(*types.Interface); isInterface {
				return
			}
			visit(typed.Underlying())
		case *types.Pointer:
			visit(typed.Elem())
		case *types.Slice:
			visit(typed.Elem())
		case *types.Array:
			visit(typed.Elem())
		case *types.Map:
			visit(typed.Key())
			visit(typed.Elem())
		case *types.Signature:
			if typed.Recv() != nil {
				visit(typed.Recv().Type())
			}
			for i := 0; i < typed.Params().Len(); i++ {
				visit(typed.Params().At(i).Type())
			}
			for i := 0; i < typed.Results().Len(); i++ {
				visit(typed.Results().At(i).Type())
			}
		case *types.Struct:
			for i := 0; i < typed.NumFields(); i++ {
				visit(typed.Field(i).Type())
			}
		case *types.Interface:
			for i := 0; i < typed.NumMethods(); i++ {
				visit(typed.Method(i).Type())
			}
		}
	}
	for _, callable := range b.api.Callables {
		if callable.Receiver != nil {
			visit(callable.Receiver)
		}
		visit(callable.Signature)
	}

	var candidates []*types.TypeName
	for named := range seen {
		if named.Obj() == nil || !named.Obj().Exported() || named.Obj().Pkg() == nil ||
			!b.canReferenceImplementationType(named) || b.hasDedicatedMapping(named) {
			continue
		}
		candidates = append(candidates, named.Obj())
	}
	return candidates
}

func (b *builder) moduleInterfaceImplementorCandidates(iface *types.Named) []*types.TypeName {
	if iface == nil || iface.Obj() == nil || iface.Obj().Pkg() == nil {
		return nil
	}
	packagesByPath := b.loadedPackagesByPath()
	declarationPackage := packagesByPath[iface.Obj().Pkg().Path()]
	if declarationPackage == nil || declarationPackage.Module == nil || declarationPackage.Module.Path == "" {
		return nil
	}
	inputModule := b.api.Package.Module
	if inputModule != nil && inputModule.Path == declarationPackage.Module.Path {
		return nil
	}

	var candidates []*types.TypeName
	for _, pkg := range packagesByPath {
		if pkg.Module == nil || pkg.Module.Path != declarationPackage.Module.Path ||
			!b.canReferenceImplementationPackage(pkg) {
			continue
		}
		for _, name := range pkg.Types.Scope().Names() {
			object, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok || !object.Exported() {
				continue
			}
			candidates = append(candidates, object)
		}
	}
	return candidates
}

func (b *builder) loadedPackagesByPath() map[string]*packages.Package {
	result := map[string]*packages.Package{}
	var visit func(*packages.Package)
	visit = func(pkg *packages.Package) {
		if pkg == nil || result[pkg.PkgPath] != nil {
			return
		}
		result[pkg.PkgPath] = pkg
		for _, imported := range pkg.Imports {
			visit(imported)
		}
	}
	visit(b.api.Package)
	return result
}

func (b *builder) canReferenceImplementationPackage(pkg *packages.Package) bool {
	if pkg == nil || pkg.Types == nil || pkg.Name == "main" {
		return false
	}
	return b.canReferenceImplementationPackagePath(pkg.PkgPath)
}

func (b *builder) canReferenceImplementationType(named *types.Named) bool {
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	if named.Obj().Pkg() == b.api.Package.Types {
		return true
	}
	if named.Obj().Pkg().Name() == "main" {
		return false
	}
	return b.canReferenceImplementationPackagePath(named.Obj().Pkg().Path())
}

func (b *builder) canReferenceImplementationPackagePath(path string) bool {
	marker := "/internal/"
	index := strings.LastIndex(path, marker)
	if index < 0 {
		if strings.HasPrefix(path, "internal/") || path == "internal" {
			return false
		}
		if parent, found := strings.CutSuffix(path, "/internal"); found {
			return b.unit.PackagePath == parent || strings.HasPrefix(b.unit.PackagePath, parent+"/")
		}
		return true
	}
	parent := path[:index]
	return b.unit.PackagePath == parent || strings.HasPrefix(b.unit.PackagePath, parent+"/")
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

type atomicCollectionClass uint8

const (
	atomicCollectionNone atomicCollectionClass = iota
	atomicCollectionOpaque
	atomicCollectionReject
)

func (b *builder) atomicCollectionShape(typ types.Type) atomicCollectionClass {
	return atomicCollectionShape(types.Unalias(typ), map[types.Type]bool{})
}

func atomicCollectionShape(typ types.Type, seen map[types.Type]bool) atomicCollectionClass {
	typ = types.Unalias(typ)
	if seen[typ] {
		return atomicCollectionNone
	}
	seen[typ] = true
	defer delete(seen, typ)
	switch typed := typ.(type) {
	case *types.Named:
		if _, atomic := atomicValueType(typed); atomic {
			return atomicCollectionNone
		}
		return atomicCollectionShape(typed.Underlying(), seen)
	case *types.Slice:
		if atomicValueByCopy(typed.Elem(), seen) {
			return atomicCollectionOpaque
		}
	case *types.Map:
		if atomicValueByCopy(typed.Key(), seen) || atomicValueByCopy(typed.Elem(), seen) {
			return atomicCollectionOpaque
		}
	case *types.Array:
		if atomicValueByCopy(typed.Elem(), seen) {
			return atomicCollectionReject
		}
	}
	return atomicCollectionNone
}

func atomicValueByCopy(typ types.Type, seen map[types.Type]bool) bool {
	typ = types.Unalias(typ)
	if seen[typ] {
		return false
	}
	seen[typ] = true
	defer delete(seen, typ)
	switch typed := typ.(type) {
	case *types.Pointer:
		return false
	case *types.Named:
		if _, atomic := atomicValueType(typed); atomic {
			return true
		}
		return atomicValueByCopy(typed.Underlying(), seen)
	case *types.Slice:
		return atomicValueByCopy(typed.Elem(), seen)
	case *types.Array:
		return atomicValueByCopy(typed.Elem(), seen)
	case *types.Map:
		return atomicValueByCopy(typed.Key(), seen) || atomicValueByCopy(typed.Elem(), seen)
	case *types.Struct:
		for i := 0; i < typed.NumFields(); i++ {
			if atomicValueByCopy(typed.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func (b *builder) mapSyntheticOpaque(original types.Type, identity types.Type) (*wireType, error) {
	key := types.TypeString(types.Unalias(identity), func(pkg *types.Package) string {
		return pkg.Path()
	})
	opaque := b.syntheticOpaques[key]
	if opaque == nil {
		base := b.syntheticOpaqueName(types.Unalias(identity))
		base = names.AvoidDartAmbientType(base)
		dartName := base
		owner, namedIdentity := types.Unalias(identity).(*types.Named)
		nameAvailable := func(candidate string) bool {
			declared := b.dartTypeNames[candidate]
			return !b.syntheticDartNames[candidate] && (declared == nil || namedIdentity && declared == owner)
		}
		for suffix := 2; !nameAvailable(dartName); suffix++ {
			dartName = fmt.Sprintf("%s%d", base, suffix)
		}
		b.syntheticDartNames[dartName] = true
		opaque = &opaqueModel{GoName: key, DartName: dartName, Synthetic: true}
		b.syntheticOpaques[key] = opaque
		b.unit.Opaques = append(b.unit.Opaques, opaque)
		b.warnings = append(b.warnings, fmt.Errorf(
			"type %s bridges as synthetic GoOpaque %s because copying its atomic collection elements is invalid; Dart can only pass this token back to Go",
			key, dartName))
	}
	result := b.newType(original, kindOpaque, opaque.DartName+"?")
	result.Opaque = opaque
	if opaque.Type == nil {
		opaque.Type = result
	}
	return result, nil
}

func (b *builder) syntheticOpaqueName(typ types.Type) string {
	typ = types.Unalias(typ)
	switch typed := typ.(type) {
	case *types.Pointer:
		return b.syntheticOpaqueName(typed.Elem())
	case *types.Named:
		name := names.UpperCamel(typed.Obj().Name())
		if typed.Obj().Pkg() != nil && typed.Obj().Pkg() != b.api.Package.Types {
			name = names.UpperCamel(typed.Obj().Pkg().Name()) + name
		}
		return name
	case *types.Slice:
		return b.syntheticOpaqueName(typed.Elem()) + "Slice"
	case *types.Map:
		return b.syntheticOpaqueName(typed.Key()) + b.syntheticOpaqueName(typed.Elem()) + "Map"
	case *types.Basic:
		return names.UpperCamel(typed.Name())
	default:
		return "AtomicOpaque"
	}
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
	if b.isFixedGoImport(pkg.Path()) {
		return
	}
	alias := fmt.Sprintf("fgbext%d", len(b.unit.ExternalImports))
	b.unit.GoPackageAliases[pkg.Path()] = alias
	b.unit.ExternalImports = append(b.unit.ExternalImports, goImportModel{Alias: alias, Path: pkg.Path()})
}

func (b *builder) isFixedGoImport(path string) bool {
	if path == b.unit.SupportPackagePath {
		return true
	}
	switch path {
	case "bytes", "context", "encoding/binary", "fmt", "io", "math/big", "os", "reflect", "runtime", "runtime/debug", "sync", "sync/atomic", "unsafe", "time", "net/netip", "net/url", "github.com/gofrs/uuid/v5", "github.com/shopspring/decimal", "github.com/cockroachdb/apd/v3":
		return true
	default:
		return false
	}
}

func stableWireTag(typ types.Type) string {
	typ = types.Unalias(typ)
	if pointer, ok := typ.(*types.Pointer); ok {
		return "*" + stableWireTag(pointer.Elem())
	}
	named, ok := typ.(*types.Named)
	if !ok || named.Obj() == nil {
		return types.TypeString(typ, func(pkg *types.Package) string {
			if pkg == nil {
				return ""
			}
			return pkg.Path()
		})
	}
	if pkg := named.Obj().Pkg(); pkg != nil {
		return pkg.Path() + "." + named.Obj().Name()
	}
	return named.Obj().Name()
}

// dartNameFor keeps the Go declaration name when it is unambiguous. Input
// package declarations are reserved before mapping starts, so an external
// collision receives a package-prefixed name such as models.User -> ModelsUser.
func (b *builder) dartNameFor(named *types.Named, preferred string) string {
	if cached := b.dartNames[named]; cached != "" {
		return cached
	}
	// dart:core needs no import, so a class reusing one of its names shadows it
	// for the whole generated library -- including the generator's own uses of
	// List<T>, Duration and friends. Rename before reserving anything.
	if names.IsDartAmbientType(preferred) {
		renamed := names.AvoidDartAmbientType(preferred)
		b.warnings = append(b.warnings, fmt.Errorf(
			"Dart type name %q would shadow the ambient Dart type of the same name; the generated class is named %q instead",
			preferred, renamed))
		preferred = renamed
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
	if isErrorType(named) || isTime(named) || isDuration(named) || isBigInt(named) ||
		isInternetIP(named) || isIPPrefix(named) || isURL(named) || isUUID(named) ||
		isShopspringDecimal(named) || isAPDDecimal(named) {
		return true
	}
	if b.isDartOpaque(named) || b.isStreamSink(named) {
		return true
	}
	_, isAtomic := atomicValueType(named)
	return isAtomic
}

func isUUID(named *types.Named) bool { return isNamed(named, "github.com/gofrs/uuid/v5", "UUID") }

func isShopspringDecimal(named *types.Named) bool {
	return isNamed(named, "github.com/shopspring/decimal", "Decimal")
}

func isAPDDecimal(named *types.Named) bool {
	return isNamed(named, "github.com/cockroachdb/apd/v3", "Decimal")
}

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
