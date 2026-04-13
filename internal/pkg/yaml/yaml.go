// Package yaml provides YAML marshalling with bomb protection, backed by gopkg.in/yaml.v3.
package yaml

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

const (
	maxYAMLDepth = 100
	maxYAMLNodes = 100_000
)

// Unmarshal decodes YAML bytes into the supplied destination pointer.
// Enforces maximum depth (100) and node count (100,000) limits.
func Unmarshal(data []byte, v any) error {
	if v == nil {
		return fmt.Errorf("unmarshal target cannot be nil")
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("unmarshal target must be a non-nil pointer")
	}

	// Parse to check for YAML bombs (depth and node count)
	var rawNode yaml.Node
	if err := yaml.Unmarshal(data, &rawNode); err != nil {
		return err
	}
	if err := checkNodeDepth(&rawNode, 0); err != nil {
		return err
	}
	if count := countNodes(&rawNode); count > maxYAMLNodes {
		return fmt.Errorf("yaml document exceeds maximum node count (%d > %d)", count, maxYAMLNodes)
	}

	// Decode into the target struct
	return yaml.Unmarshal(data, v)
}

// Marshal encodes a value to YAML bytes.
func Marshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

// countNodes recursively counts all nodes in the YAML tree.
func countNodes(n *yaml.Node) int {
	count := 1
	for _, child := range n.Content {
		count += countNodes(child)
	}
	return count
}

// checkNodeDepth recursively checks that no nesting exceeds maxYAMLDepth.
func checkNodeDepth(n *yaml.Node, depth int) error {
	if depth > maxYAMLDepth {
		return fmt.Errorf("yaml document exceeds maximum depth (%d > %d)", depth, maxYAMLDepth)
	}
	for _, child := range n.Content {
		if err := checkNodeDepth(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}
