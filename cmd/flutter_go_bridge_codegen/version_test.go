package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestResolvedVersion(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	t.Run("runtime environment wins", func(t *testing.T) {
		version = "v1.2.3"
		t.Setenv(versionEnvironment, " v2.0.0-beta1 ")
		if got := resolvedVersion(); got != "v2.0.0-beta1" {
			t.Fatalf("resolvedVersion() = %q, want %q", got, "v2.0.0-beta1")
		}
	})

	t.Run("build version is used without environment", func(t *testing.T) {
		version = "v1.2.3"
		t.Setenv(versionEnvironment, "")
		if got := resolvedVersion(); got != "v1.2.3" {
			t.Fatalf("resolvedVersion() = %q, want %q", got, "v1.2.3")
		}
	})

	t.Run("development is the final fallback", func(t *testing.T) {
		version = ""
		t.Setenv(versionEnvironment, "   ")
		if got := resolvedVersion(); got != defaultVersion {
			t.Fatalf("resolvedVersion() = %q, want %q", got, defaultVersion)
		}
	})
}

func TestDefaultVersionIsDevelopment(t *testing.T) {
	if defaultVersion != "development" {
		t.Fatalf("defaultVersion = %q, want %q", defaultVersion, "development")
	}
}

func TestVersionFlagsIncludeGoToolchain(t *testing.T) {
	original := version
	version = "v1.2.3"
	t.Cleanup(func() { version = original })
	t.Setenv(versionEnvironment, "")

	for _, flag := range []string{"-v", "--version"} {
		t.Run(flag, func(t *testing.T) {
			command := newRootCommand()
			output := &bytes.Buffer{}
			command.SetOut(output)
			command.SetErr(output)
			command.SetArgs([]string{flag})

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			text := output.String()
			for _, expected := range []string{
				"flutter_go_bridge_codegen version v1.2.3",
				"Build with " + runtime.Version(),
			} {
				if !strings.Contains(text, expected) {
					t.Fatalf("%s output %q does not contain %q", flag, text, expected)
				}
			}
		})
	}
}
