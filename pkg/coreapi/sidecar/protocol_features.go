package sidecar

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ReadProtocolFeatures reads the Rust source export used to build a development
// sidecar. A previous signed manifest is not the new binary's capability source.
func ReadProtocolFeatures(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read core protocol schema: %w", err)
	}
	var schema struct {
		Methods []struct {
			Name string `json:"name"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("decode core protocol schema: %w", err)
	}
	if len(schema.Methods) == 0 {
		return nil, fmt.Errorf("core protocol schema has no methods")
	}
	features := make([]string, 0, len(schema.Methods))
	seen := make(map[string]struct{}, len(schema.Methods))
	for _, method := range schema.Methods {
		name := method.Name
		if name == "" || strings.TrimSpace(name) != name {
			return nil, fmt.Errorf("core protocol schema has an invalid method name")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("core protocol schema has a duplicate method")
		}
		seen[name] = struct{}{}
		features = append(features, name)
	}
	return features, nil
}
