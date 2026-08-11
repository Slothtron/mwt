package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML enforces the single-key setup step shape and rejects unknown actions.
func (s *SetupStep) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("setup step must be a mapping with one action key")
	}
	if len(value.Content) != 2 {
		return fmt.Errorf("setup step must have exactly one action key")
	}

	keyNode := value.Content[0]
	valNode := value.Content[1]
	if keyNode.Kind != yaml.ScalarNode {
		return fmt.Errorf("setup step action key must be a string")
	}

	switch keyNode.Value {
	case "copy":
		var action CopyAction
		if err := valNode.Decode(&action); err != nil {
			return fmt.Errorf("copy: %w", err)
		}
		s.Copy = &action
	case "run":
		var action RunAction
		if err := valNode.Decode(&action); err != nil {
			return fmt.Errorf("run: %w", err)
		}
		s.Run = &action
	default:
		return fmt.Errorf("unknown setup action %q", keyNode.Value)
	}
	return nil
}
