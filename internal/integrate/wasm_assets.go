package integrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ensureWebWasmAssets declares the single Web artifact directory owned by the
// existing go_builder/plugin package. Native platforms continue to use the
// FFI plugin entries already present in the generated pubspec.
func ensureWebWasmAssets(dartRoot string, template Template) error {
	if template == TemplateApp {
		return nil
	}
	path := filepath.Join(dartRoot, "pubspec.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read plugin pubspec for Web Wasm assets: %w", err)
	}
	if strings.Contains(string(raw), "assets/wasm/") {
		return nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("parse plugin pubspec for Web Wasm assets: %w", err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("plugin pubspec must contain a YAML mapping")
	}
	root := document.Content[0]
	flutter := mappingValue(root, "flutter")
	if flutter == nil {
		flutter = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendMapping(root, "flutter", flutter)
	}
	assets := mappingValue(flutter, "assets")
	if assets == nil {
		assets = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		appendMapping(flutter, "assets", assets)
	}
	if assets.Kind != yaml.SequenceNode {
		return fmt.Errorf("plugin pubspec flutter.assets must be a list")
	}
	assets.Content = append(assets.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "assets/wasm/"})
	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func appendMapping(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value,
	)
}
