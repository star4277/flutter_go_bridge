// Package template embeds the Flutter project templates that
// `flutter_go_bridge_codegen integrate` overlays onto a Flutter project.
package template

import "embed"

// FS holds the app/plugin/shared integration templates.
//
// The `all:` prefix keeps dotfiles such as .gitignore. Go's embed silently
// skips `.git` entries and any directory containing its own go.mod, which is
// why the Go module template files carry a `.template` suffix; the integrator
// renames them back while overlaying.
//
//go:embed all:app all:plugin all:shared
var FS embed.FS
