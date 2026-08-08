package generator

import (
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	bridgemodel "github.com/star4277/flutter_go_bridge/internal/model"
	"golang.org/x/tools/go/packages"
)

func TestPatchCoverageAtomicResultArrayAndMapBranches(t *testing.T) {
	t.Run("atomic result must be a pointer", func(t *testing.T) {
		_, _, _, _, err := generateFixture(t, `package api

import "sync/atomic"

func Value() atomic.Int64 { return atomic.Int64{} }
`)
		if err == nil || !strings.Contains(err.Error(), "must be returned by pointer") {
			t.Fatalf("atomic result should be rejected, got %v", err)
		}
	})

	t.Run("atomic array is rejected", func(t *testing.T) {
		_, _, _, _, err := generateFixture(t, `package api

import "sync/atomic"

func Value(values [2]atomic.Int64) {}
`)
		if err == nil || !strings.Contains(err.Error(), "arrays containing atomic values") {
			t.Fatalf("atomic array should be rejected, got %v", err)
		}
	})

	t.Run("unnamed atomic map is boxed", func(t *testing.T) {
		apiDart, _, _, warnings, err := generateFixture(t, `package api

import "sync/atomic"

func Value(values map[string]atomic.Int64) {}
`)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(apiDart, "AtomicInt64Map?") || len(warnings) == 0 {
			t.Fatalf("unnamed atomic map should use a synthetic opaque: %s warnings=%v", apiDart, warnings)
		}
	})
}

func TestPatchCoverageBuilderNamingAndInterfaceBranches(t *testing.T) {
	b := &builder{}
	named := &namedModel{GoName: "Value", Methods: []*callModel{{
		GoName: "Describe", DartName: "toString", ToString: true,
	}}}
	b.normalizeNamedExtensionMethods(named)
	if named.Methods[0].DartName != "describe" || named.Methods[0].ToString {
		t.Fatalf("extension fallback method = %#v", named.Methods[0])
	}

	iface := &interfaceModel{
		GoName: "Thing", DartName: "Thing",
		Methods: []*callModel{{GoName: "Do", DartName: "do"}},
	}
	operator := &callModel{GoName: "Do", DartName: "do", Operator: "+"}
	iface.Implementors = []*implementorModel{{
		DartName: "Implementation",
		Type:     &wireType{Kind: kindStruct, Struct: &structModel{Methods: []*callModel{operator}}},
	}}
	b.unit = &unit{Interfaces: []*interfaceModel{iface}}
	if err := b.checkInterfaceImplementations(); err == nil || !strings.Contains(err.Error(), "bridges no method") {
		t.Fatalf("operator-only implementation should fail interface validation, got %v", err)
	}

	if (&callModel{}).dartMember() != "" || (*callModel)(nil).dartMember() != "" {
		t.Fatal("nil call members should have an empty Dart member name")
	}
}

func TestPatchCoverageReachableCandidatesVisitContainers(t *testing.T) {
	pkg := types.NewPackage("example.com/candidates", "candidates")
	name := types.NewTypeName(0, pkg, "Candidate", nil)
	candidate := types.NewNamed(name, types.NewStruct(nil, nil), nil)
	signature := types.NewSignatureType(nil, nil, nil, types.NewTuple(
		types.NewVar(0, pkg, "slice", types.NewSlice(candidate)),
		types.NewVar(0, pkg, "array", types.NewArray(candidate, 2)),
	), types.NewTuple(), false)
	b := &builder{
		api:  &bridgemodel.API{Package: &packages.Package{Types: pkg}, Callables: []*bridgemodel.Callable{{Signature: signature}}},
		unit: &unit{PackagePath: pkg.Path()},
	}
	if candidates := b.reachableInterfaceImplementorCandidates(); len(candidates) != 1 || candidates[0] != name {
		t.Fatalf("reachable candidate discovery = %#v, want Candidate", candidates)
	}
}

func TestPatchCoverageStableWireTagCompoundUniverseType(t *testing.T) {
	errorSlice := types.NewSlice(types.Universe.Lookup("error").Type())
	if got := stableWireTag(errorSlice); got != "[]error" {
		t.Fatalf("stableWireTag() = %q, want []error", got)
	}
}

func TestPatchCoverageRejectsVariadicDartOperator(t *testing.T) {
	pkg := types.NewPackage("example.com/operators", "operators")
	name := types.NewTypeName(0, pkg, "Value", nil)
	receiver := types.NewNamed(name, types.NewStruct(nil, nil), nil)
	receiverVar := types.NewVar(0, pkg, "value", receiver)
	signature := types.NewSignatureType(
		receiverVar, nil, nil,
		types.NewTuple(types.NewVar(0, pkg, "other", types.NewSlice(receiver))),
		types.NewTuple(types.NewVar(0, pkg, "result", receiver)), true,
	)
	function := types.NewFunc(0, pkg, "Add", signature)
	if got := dartOperatorFor(&bridgemodel.Callable{Func: function, Signature: signature, Receiver: receiver}); got != "" {
		t.Fatalf("variadic operator should keep an ordinary method shape, got %q", got)
	}
}

func TestPatchCoverageDartSplitWebLoaderAndNamedFallback(t *testing.T) {
	root := t.TempDir()
	central := filepath.Join(root, "bridge_generated.dart")
	files, err := renderDartSplit(&unit{
		Target:              "web",
		ClassName:           "Bridge",
		LibraryName:         "bridge",
		UseFlutterWebLoader: true,
	}, central)
	if err != nil {
		t.Fatal(err)
	}
	if source := string(files[central]); !strings.Contains(source, "package:flutter/services.dart") || !strings.Contains(source, "package:flutter/widgets.dart") {
		t.Fatalf("Web Flutter loader imports are missing:\n%s", files[central])
	}

	renderer := &splitDartRenderer{}
	renderer.renderNamed(&namedModel{
		DartName:      "Value",
		Underlying:    &wireType{Kind: kindString, DartType: "String"},
		LocalToString: true,
	})
	if !strings.Contains(renderer.buffer.String(), "String toString() => value.toString();") {
		t.Fatalf("named local toString fallback was not rendered:\n%s", renderer.buffer.String())
	}
	if !unitNeedsJSONToString(&unit{Named: []*namedModel{{Methods: []*callModel{{ToString: true, ToStringFormat: toStringJSON}}}}}) {
		t.Fatal("named JSON toString should require dart:convert")
	}
}

func TestPatchCoverageDartDualCollisionAndInterfacePaths(t *testing.T) {
	commonUnit := &unit{ClassName: "Bridge", Interfaces: []*interfaceModel{{DartName: "Thing"}}}
	if _, err := renderDartDualCommon(commonUnit, "bridge.dart", "bridge.io.dart", "bridge.web.dart", map[string]string{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := renderDartDual(&unit{ClassName: "Bridge", Types: []*wireType{{ID: 1, Kind: typeKind("unknown"), DartType: "Unknown"}}, MirrorRoot: "."}, &unit{ClassName: "Bridge"}, "bridge.dart"); err == nil || !strings.Contains(err.Error(), "no Dart decoder") {
		t.Fatalf("dual renderer should surface common renderer errors, got %v", err)
	}

	root := t.TempDir()
	central := filepath.Join(root, "bridge_generated.dart")
	collision := &unit{
		ClassName: "Bridge", LibraryName: "bridge", MirrorRoot: root,
		SourceFiles: []string{filepath.Join(root, "bridge_generated.io.go")},
	}
	if _, err := renderDartDual(collision, collision, central); err == nil || !strings.Contains(err.Error(), "collides with generated bridge output") {
		t.Fatalf("platform output collision should be rejected, got %v", err)
	}
}

func TestPatchCoverageGenerateReportsMetadataAndDartCollision(t *testing.T) {
	api, resolved, sourcePath := parseGenerateAllFixture(t, "func Add(left, right int) int { return left + right }\n")
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(api, config.Resolved{
		Target: config.TargetWeb, BaseDir: resolved.BaseDir, GoInput: resolved.GoInput,
		GoOutput: resolved.GoOutput, DartOutput: resolved.DartOutput, LibraryName: "bridge",
		DartEntrypointClassName: "Bridge", StopOnError: true,
	}); err == nil || !strings.Contains(err.Error(), "read Web bridge source") {
		t.Fatalf("single-target metadata error was not returned, got %v", err)
	}

	api, resolved, _ = parseGenerateAllFixture(t, "func Add(left, right int) int { return left + right }\n")
	api.Package.Module = nil
	resolved.DartOutput = filepath.Join(resolved.BaseDir, "dart", "api.dart")
	if _, err := GenerateAll(api, resolved); err == nil || !strings.Contains(err.Error(), "Dart source") {
		t.Fatalf("GenerateAll Dart collision was not returned, got %v", err)
	}
}
