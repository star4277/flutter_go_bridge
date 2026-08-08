package generator

import (
	"go/types"
	"strings"
	"testing"

	bridgemodel "github.com/star4277/flutter_go_bridge/internal/model"
)

func TestRenderGoWebIncludesConditionalRuntimeDependencies(t *testing.T) {
	unit := &unit{
		PackagePath: "example.com/api",
		LibraryName: "go_lib_web_coverage",
		GoPreamble:  "// user preamble",
		Calls: []*callModel{{
			Mode: bridgemodel.CallModeAsync, WireName: "Async", GoTarget: "Async", ContextIndex: -1,
		}},
		UsesTime:              true,
		UsesInternetIP:        true,
		UsesIPPrefix:          true,
		UsesURL:               true,
		UsesUUID:              true,
		UsesShopspringDecimal: true,
		UsesAPDDecimal:        true,
		UsesRuntimePackage:    true,
		SupportPackagePath:    "example.com/internal/fgb",
		ExternalImports:       []goImportModel{{Alias: "fgbext0", Path: "example.com/models"}},
	}

	source, err := renderGoWeb(unit)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"// user preamble",
		`"context"`,
		`"time"`,
		`"net/netip"`,
		`"net/url"`,
		`"github.com/gofrs/uuid/v5"`,
		`"github.com/shopspring/decimal"`,
		`"github.com/cockroachdb/apd/v3"`,
		`fgbrt "example.com/internal/fgb"`,
		`fgbext0 "example.com/models"`,
		"func fgbTimeUnixMicro(value time.Time)",
	} {
		if !strings.Contains(string(source), expected) {
			t.Fatalf("Web renderer output does not contain %q:\n%s", expected, source)
		}
	}
}

func TestRenderGoWebReportsRuntimeAndTypeErrors(t *testing.T) {
	if _, err := webGoRuntimeBase("package main"); err == nil || !strings.Contains(err.Error(), "split marker") {
		t.Fatalf("missing runtime marker error = %v", err)
	}
	_, err := renderGoWeb(&unit{
		LibraryName: "bad",
		Types:       []*wireType{{ID: 1, Kind: typeKind("unknown"), Original: types.Typ[types.Int]}},
	})
	if err == nil || !strings.Contains(err.Error(), "no Go decoder") {
		t.Fatalf("unknown Web type error = %v", err)
	}
}

func TestDartRuntimeSplitErrorsPropagateThroughRenderers(t *testing.T) {
	renderUnit := &unit{ClassName: "Bridge", LibraryName: "bridge"}
	tests := []struct {
		name   string
		source string
		call   func(*unit, string) (string, error)
		want   string
	}{
		{name: "Web missing opaque", source: "", call: dartWebRuntimeSourceFrom, want: "GoOpaque marker"},
		{name: "Native missing absent", source: "abstract base class GoOpaque", call: dartNativeRuntimeSourceFrom, want: "GoAbsent marker"},
		{name: "Native platform missing FFI", source: "abstract base class GoOpaque abstract interface class GoAbsent", call: dartNativePlatformRuntimeSourceFrom, want: "FFI marker"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.call(renderUnit, test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	if _, err := dartCommonRuntimeSourceFrom(""); err == nil || !strings.Contains(err.Error(), "GoOpaque marker") {
		t.Fatalf("common runtime error = %v", err)
	}
}
