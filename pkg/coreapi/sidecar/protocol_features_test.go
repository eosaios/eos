package sidecar

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadProtocolFeaturesPreservesRustExportOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	data := `{"version":1,"methods":[{"name":"turn/start","group":"turn"},{"name":"initialize","group":"core"}],"types":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadProtocolFeatures(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"turn/start", "initialize"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("features = %#v, want %#v", got, want)
	}
}

func TestReadProtocolFeaturesRejectsMalformedExports(t *testing.T) {
	for name, data := range map[string]string{
		"invalid json":        "{",
		"array root":          `[]`,
		"missing methods":     `{}`,
		"null methods":        `{"methods":null}`,
		"empty methods":       `{"methods":[]}`,
		"wrong methods shape": `{"methods":{}}`,
		"missing name":        `{"methods":[{}]}`,
		"blank name":          `{"methods":[{"name":" "}]}`,
		"padded name":         `{"methods":[{"name":" initialize"}]}`,
		"wrong name type":     `{"methods":[{"name":1}]}`,
		"duplicate":           `{"methods":[{"name":"initialize"},{"name":"initialize"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadProtocolFeatures(path)
			if got != nil || err == nil {
				t.Fatalf("malformed schema returned features %#v, error %v", got, err)
			}
		})
	}
}

func TestReadProtocolFeaturesRequiresSourceExport(t *testing.T) {
	got, err := ReadProtocolFeatures(filepath.Join(t.TempDir(), "missing.json"))
	if got != nil || err == nil {
		t.Fatalf("missing source returned features %#v, error %v", got, err)
	}
}
