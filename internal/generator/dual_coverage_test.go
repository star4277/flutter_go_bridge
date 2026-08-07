package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/config"
	"github.com/star4277/flutter_go_bridge/internal/model"
	"golang.org/x/tools/go/packages"
)

func TestGenerateAllRejectsInvalidOutputAndPackageLayout(t *testing.T) {
	root := t.TempDir()
	api := &model.API{
		InputDir: root,
		Package:  &packages.Package{Name: "api", PkgPath: "example.com/fallback"},
	}
	if _, err := GenerateAll(api, config.Resolved{GoOutput: filepath.Join(root, "bridge.txt")}); err == nil || !strings.Contains(err.Error(), "go_output must be a .go file") {
		t.Fatalf("invalid Go output was accepted: %v", err)
	}
	if _, err := GenerateAll(api, config.Resolved{GoOutput: filepath.Join(root, "bridge.go")}); err == nil || !strings.Contains(err.Error(), "inside input package") {
		t.Fatalf("non-main direct output was accepted: %v", err)
	}
	api.Package.Name = "main"
	if _, err := GenerateAll(api, config.Resolved{GoOutput: filepath.Join(root, "out", "bridge.go")}); err == nil || !strings.Contains(err.Error(), "package main cannot be imported") {
		t.Fatalf("separate main output was accepted: %v", err)
	}
}

func TestDartDualRuntimeKeepsSpecialDependenciesAndPreamble(t *testing.T) {
	root := t.TempDir()
	u := &unit{
		ClassName: "Bridge", LibraryName: "special", DartPreamble: "typedef UserType = String;",
		UsesUUID: true, UsesDecimal: true, UsesInternetIP: true,
	}
	common, err := renderDartDualCommon(u, filepath.Join(root, "bridge_generated.dart"), filepath.Join(root, "bridge_generated.io.dart"), filepath.Join(root, "bridge_generated.web.dart"), map[string]string{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(common), "typedef UserType = String;") || !strings.Contains(string(common), "package:uuid/uuid.dart") || !strings.Contains(string(common), "package:decimal/decimal.dart") {
		t.Fatalf("common renderer dropped preamble or dependencies:\n%s", common)
	}
	web := dartWebPlatformRuntimeSource(u)
	if !strings.Contains(web, "typedef InternetAddress = String;") {
		t.Fatalf("Web runtime did not provide InternetAddress fallback:\n%s", web)
	}
	if _, err := dartNativePlatformRuntimeSource(u); err != nil {
		t.Fatal(err)
	}
	for name, isWeb := range map[string]bool{"native": false, "web": true} {
		platform, err := renderDartDualPlatform(
			u,
			filepath.Join(root, "bridge_generated."+name+".dart"),
			filepath.Join(root, "bridge_generated.dart"),
			map[string]string{},
			nil,
			isWeb,
		)
		if err != nil {
			t.Fatalf("render %s platform: %v", name, err)
		}
		if !strings.Contains(string(platform), "package:uuid/uuid.dart") || !strings.Contains(string(platform), "package:decimal/decimal.dart") {
			t.Fatalf("%s platform renderer dropped dependencies:\n%s", name, platform)
		}
	}
}

func TestRenderDartDualReportsRendererAndPathErrors(t *testing.T) {
	bad := &unit{ClassName: "Bridge", Types: []*wireType{{ID: 1, Kind: typeKind("unknown"), DartType: "Unknown"}}}
	if _, err := renderDartDualCommon(bad, "bridge.dart", "bridge.io.dart", "bridge.web.dart", map[string]string{}, nil); err == nil || !strings.Contains(err.Error(), "no Dart decoder") {
		t.Fatalf("invalid decoder type was accepted: %v", err)
	}
	root := t.TempDir()
	central := filepath.Join(root, "bridge_generated.dart")
	collision := &unit{
		ClassName: "Bridge", LibraryName: "bridge", MirrorRoot: root,
		SourceFiles: []string{filepath.Join(root, "bridge_generated.go")},
	}
	if _, err := renderDartDual(collision, collision, central); err == nil || !strings.Contains(err.Error(), "collides with") {
		t.Fatalf("Dart source/output collision was accepted: %v", err)
	}
}

func TestGeneratorWriteAndMetadataErrors(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeGeneratedFiles(map[string][]byte{filepath.Join(blocker, "bridge.go"): []byte("broken")}, Result{}); err == nil {
		t.Fatal("a regular file used as an output directory should fail")
	}
	if _, err := writeGeneratedFiles(map[string][]byte{"": []byte("broken")}, Result{}); err == nil {
		t.Fatal("empty output path should fail during rename")
	}
	api := &model.API{SourceFiles: []string{filepath.Join(t.TempDir(), "missing.go")}}
	if _, err := webBridgeMetadata(api, config.Resolved{Target: config.TargetWeb}); err == nil {
		t.Fatal("missing Web bridge source should fail metadata generation")
	}
}

func TestUnsupportedWebCallReasonTraversesWireShapes(t *testing.T) {
	callback := func(id int) *wireType {
		return &wireType{ID: id, Kind: kindCallback}
	}
	tests := []struct {
		name string
		call *callModel
		want string
	}{
		{"cgo scalar", &callModel{Params: []*paramModel{{Type: &wireType{ID: 1, CgoScalar: true}}}}, "cgo scalar types"},
		{"callback", &callModel{Params: []*paramModel{{Type: callback(2)}}}, "Dart callbacks"},
		{"stream", &callModel{Params: []*paramModel{{Type: &wireType{ID: 3, Kind: kindStreamSink}}}}, "streams"},
		{"opaque", &callModel{Params: []*paramModel{{Type: &wireType{ID: 4, Kind: kindOpaque}}}}, "opaque handles"},
		{"interface", &callModel{Params: []*paramModel{{Type: &wireType{ID: 5, Kind: kindInterface}}}}, "opaque handles"},
		{"internet address", &callModel{Params: []*paramModel{{Type: &wireType{ID: 6, Kind: kindInternetIP}}}}, "InternetAddress"},
		{"pointer element", &callModel{Params: []*paramModel{{Type: &wireType{ID: 7, Kind: kindPointer, Elem: callback(8)}}}}, "Dart callbacks"},
		{"map key", &callModel{Params: []*paramModel{{Type: &wireType{ID: 9, Kind: kindMap, Key: callback(10), Elem: &wireType{ID: 11, Kind: kindString}}}}}, "Dart callbacks"},
		{"map value", &callModel{Params: []*paramModel{{Type: &wireType{ID: 12, Kind: kindMap, Key: &wireType{ID: 13, Kind: kindString}, Elem: callback(14)}}}}, "Dart callbacks"},
		{"struct field", &callModel{Params: []*paramModel{{Type: &wireType{ID: 15, Kind: kindStruct, Struct: &structModel{Fields: []*fieldModel{{Type: callback(16)}}}}}}}, "Dart callbacks"},
		{"named underlying", &callModel{Params: []*paramModel{{Type: &wireType{ID: 17, Kind: kindNamed, Named: &namedModel{Underlying: callback(18)}}}}}, "Dart callbacks"},
		{"atomic value", &callModel{Params: []*paramModel{{Type: &wireType{ID: 19, Kind: kindAtomic, Atomic: &atomicModel{Value: callback(20)}}}}}, "Dart callbacks"},
		{"receiver", &callModel{Receiver: callback(21)}, "Dart callbacks"},
		{"result", &callModel{Results: []*resultModel{{Type: callback(22)}}}, "Dart callbacks"},
		{"supported", &callModel{Params: []*paramModel{{Type: &wireType{ID: 23, Kind: kindString}}}}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := unsupportedWebCallReason(test.call)
			if !strings.Contains(got, test.want) {
				t.Fatalf("unsupported reason %q does not contain %q", got, test.want)
			}
		})
	}
}
