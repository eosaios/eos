package adapter

import (
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestSessionMetaFromCoreAPIExtractsSandboxMode(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want string
	}{
		{
			name: "legacy full_access recorded",
			meta: map[string]any{"sandbox_mode": "full_access"},
			want: "danger-full-access",
		},
		{
			name: "legacy workspace recorded",
			meta: map[string]any{"sandbox_mode": "workspace"},
			want: "workspace-write",
		},
		{
			name: "canonical workspace-write recorded",
			meta: map[string]any{"sandbox_mode": "workspace-write"},
			want: "workspace-write",
		},
		{
			name: "canonical read-only recorded",
			meta: map[string]any{"sandbox_mode": "read-only"},
			want: "read-only",
		},
		{
			name: "no sandbox_mode key (new session)",
			meta: map[string]any{"title": "New Chat"},
			want: "",
		},
		{
			name: "nil metadata",
			meta: nil,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionMetaFromCoreAPI(coreapi.Session{ID: "sess-1", Metadata: tc.meta}).SandboxMode
			if got != tc.want {
				t.Fatalf("SandboxMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSessionMetasFromCoreAPIExtractsSandboxModeForEach(t *testing.T) {
	items := []coreapi.Session{
		{ID: "sess-a", Metadata: map[string]any{"sandbox_mode": "full_access"}},
		{ID: "sess-b", Metadata: map[string]any{"sandbox_mode": "workspace"}},
		{ID: "sess-c", Metadata: map[string]any{"title": "default session"}},
	}
	metas := sessionMetasFromCoreAPI(items)
	if len(metas) != 3 {
		t.Fatalf("len = %d, want 3", len(metas))
	}
	want := map[string]string{"sess-a": "danger-full-access", "sess-b": "workspace-write", "sess-c": ""}
	for _, meta := range metas {
		if meta.SandboxMode != want[meta.ID] {
			t.Errorf("session %s SandboxMode = %q, want %q", meta.ID, meta.SandboxMode, want[meta.ID])
		}
	}
}
