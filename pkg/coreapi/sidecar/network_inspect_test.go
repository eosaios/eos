package sidecar

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDisableNetworkInspectOverridesEverySpelling(t *testing.T) {
	env := []string{"EOS_NETWORK_INSPECT=1", "eos_network_inspect=true", "OTHER=1"}
	want := []string{"OTHER=1", "EOS_NETWORK_INSPECT=0"}
	if got := DisableNetworkInspect(env); !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %q, want %q", got, want)
	}
}

func TestNetworkInspectMetadataDistinguishesLegacyBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core")
	for _, marker := range []string{"EOS_NETWORK_INSPECT", "dev_network_inspect_metadata_v1"} {
		if err := os.WriteFile(path, []byte(strings.Repeat("x", (64<<10)-2)+marker), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := HasNetworkInspectMetadata(path)
		if err != nil || got != (marker != "EOS_NETWORK_INSPECT") {
			t.Fatalf("%s: got %t, %v", marker, got, err)
		}
	}
}

func TestAgentReviewModeCannotBeOverriddenByLocalEnvironment(t *testing.T) {
	env := []string{"EOS_DEV_AGENT_REVIEW=1", "eos_dev_agent_review=1", "OTHER=1"}
	for _, tt := range []struct {
		mode AgentReviewMode
		flag string
	}{{AgentReviewDisabled, "0"}, {AgentReviewDev, "1"}} {
		want := []string{"OTHER=1", "EOS_DEV_AGENT_REVIEW=" + tt.flag}
		if got := SetAgentReviewMode(env, tt.mode); !reflect.DeepEqual(got, want) {
			t.Fatalf("environment = %q, want %q", got, want)
		}
	}
}
