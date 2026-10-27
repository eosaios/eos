package webbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCoreConfigIgnoresModelFieldsFromDotEosJson(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("EOS_API_BASE", "")
	t.Setenv("EOS_API_KEY", "")
	t.Setenv("EOS_MODEL", "")

	configPath := filepath.Join(home, ".eos.json")
	if err := os.WriteFile(configPath, []byte(`{
  "active_model": "MiniMax M3",
  "models": [
    {
      "name": "MiniMax M3",
      "api_base": "https://api.minimaxi.com/v1",
      "api_key": "secret-token",
      "model": "MiniMax-M3"
    }
  ],
  "language": "en",
  "log_dir": "~/logs",
  "trusted_workspaces": ["C:/repo-a"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	cfg := loadCoreConfig()
	if cfg.APIBase != "" {
		t.Fatalf("APIBase=%q, want empty", cfg.APIBase)
	}
	if cfg.APIKeyMasked != "" {
		t.Fatalf("APIKeyMasked=%q, want empty", cfg.APIKeyMasked)
	}
	if cfg.Model != "" {
		t.Fatalf("Model=%q, want empty", cfg.Model)
	}
	if cfg.Language != "en" {
		t.Fatalf("Language=%q, want en", cfg.Language)
	}
	if len(cfg.TrustedWorkspaces) != 1 || cfg.TrustedWorkspaces[0] != "C:/repo-a" {
		t.Fatalf("TrustedWorkspaces=%v", cfg.TrustedWorkspaces)
	}
	if want := filepath.Join(home, "logs"); cfg.LogDir != want {
		t.Fatalf("LogDir=%q, want %q", cfg.LogDir, want)
	}
}
