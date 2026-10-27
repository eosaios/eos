package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDiagnosticBundle(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	logDir := filepath.Join(tmp, "localappdata")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", logDir)
	t.Setenv("EOS_API_BASE", "https://api.example.com")
	t.Setenv("EOS_API_KEY", "secret-12345678")
	t.Setenv("EOS_MODEL", "gpt-test")

	coreRaw := `{
  "active_model": "primary",
  "language": "zh",
  "trusted_workspaces": ["C:/repo-a", "C:/repo-b"],
  "models": [
    {
      "name": "primary",
      "api_base": "https://api.example.com",
      "api_key": "secret-12345678",
      "model": "gpt-test"
    }
  ]
}`
	if err := os.WriteFile(configPath(), []byte(coreRaw), 0o600); err != nil {
		t.Fatalf("write core config: %v", err)
	}

	logFile := filepath.Join(defaultLogDir(), "server.log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("hello log"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := os.WriteFile(logFile+".1", []byte("older log"), 0o600); err != nil {
		t.Fatalf("write rotated log: %v", err)
	}
	if _, err := WriteCrashReport("boom"); err != nil {
		t.Fatalf("WriteCrashReport() error = %v", err)
	}

	destination := filepath.Join(tmp, "bundle")
	out, err := ExportDiagnosticBundle(DiagnosticBundleOptions{
		DestinationPath: destination,
		LogFile:         logFile,
		Report:          "runtime snapshot",
	})
	if err != nil {
		t.Fatalf("ExportDiagnosticBundle() error = %v", err)
	}
	if !strings.HasSuffix(out, ".zip") {
		t.Fatalf("bundle path = %q, want .zip suffix", out)
	}

	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("zip.OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]string{}
	for _, file := range archive.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		files[file.Name] = string(raw)
	}

	if !strings.Contains(files["report/summary.txt"], "runtime snapshot") {
		t.Fatalf("summary missing runtime snapshot: %q", files["report/summary.txt"])
	}
	if !strings.Contains(files["logs/server.log"], "hello log") {
		t.Fatalf("server log not exported")
	}
	if !strings.Contains(files["crash/last_crash.json"], "\"panic\": \"boom\"") {
		t.Fatalf("crash report not exported")
	}
	if strings.Contains(files["config/core_config_summary.json"], "secret-12345678") {
		t.Fatalf("core config summary leaked raw api key: %s", files["config/core_config_summary.json"])
	}
	if !strings.Contains(files["config/core_config_summary.json"], "****5678") {
		t.Fatalf("core config summary missing masked api key: %s", files["config/core_config_summary.json"])
	}
	if _, ok := files["config/gui_config_summary.json"]; ok {
		t.Fatalf("diagnostic bundle should not export gui config summary")
	}
}

func TestCrashReportLifecycle(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	logDir := filepath.Join(tmp, "localappdata")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", logDir)

	path, err := WriteCrashReport("panic-value")
	if err != nil {
		t.Fatalf("WriteCrashReport() error = %v", err)
	}
	if strings.TrimSpace(path) == "" {
		t.Fatal("expected crash report path")
	}

	pending, err := LoadPendingCrashReport()
	if err != nil {
		t.Fatalf("LoadPendingCrashReport() error = %v", err)
	}
	if pending == nil || pending.Panic != "panic-value" {
		t.Fatalf("unexpected pending crash report: %+v", pending)
	}

	if err := MarkCrashReportAcknowledged(); err != nil {
		t.Fatalf("MarkCrashReportAcknowledged() error = %v", err)
	}
	pending, err = LoadPendingCrashReport()
	if err != nil {
		t.Fatalf("LoadPendingCrashReport() after ack error = %v", err)
	}
	if pending != nil {
		t.Fatalf("expected pending crash report to be cleared, got %+v", pending)
	}
}

func TestExportDiagnosticBundleWithCoreDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	logDir := filepath.Join(tmp, "localappdata")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", logDir)

	logFile := filepath.Join(defaultLogDir(), "server.log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("log"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	coreDiags := map[string]any{
		"binary_path":      "/usr/bin/eos-core",
		"manifest_version": "0.1.0",
		"protocol_version": "v1",
		"store_dir":        "/tmp/store",
		"sandbox_backend":  "WorkspaceWrite",
		"migration_marker": "complete",
		"os":               "linux",
		"arch":             "x86_64",
	}

	destination := filepath.Join(tmp, "bundle_with_core")
	out, err := ExportDiagnosticBundle(DiagnosticBundleOptions{
		DestinationPath:        destination,
		LogFile:                logFile,
		Report:                 "core diag test",
		CoreStartupDiagnostics: coreDiags,
	})
	if err != nil {
		t.Fatalf("ExportDiagnosticBundle() error = %v", err)
	}

	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("zip.OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]string{}
	for _, file := range archive.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		files[file.Name] = string(raw)
	}

	coreDiagJSON, ok := files["core/startup_diagnostics.json"]
	if !ok {
		t.Fatal("core/startup_diagnostics.json missing from bundle")
	}
	if !strings.Contains(coreDiagJSON, "/usr/bin/eos-core") {
		t.Fatalf("core diagnostics missing binary_path: %s", coreDiagJSON)
	}
	if !strings.Contains(coreDiagJSON, "WorkspaceWrite") {
		t.Fatalf("core diagnostics missing sandbox_backend: %s", coreDiagJSON)
	}
	if !strings.Contains(coreDiagJSON, "complete") {
		t.Fatalf("core diagnostics missing migration_marker: %s", coreDiagJSON)
	}
}

func TestExportDiagnosticBundleWithoutCoreDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	logDir := filepath.Join(tmp, "localappdata")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", logDir)

	logFile := filepath.Join(defaultLogDir(), "server.log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("log"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	destination := filepath.Join(tmp, "bundle_no_core")
	out, err := ExportDiagnosticBundle(DiagnosticBundleOptions{
		DestinationPath: destination,
		LogFile:         logFile,
		Report:          "no core",
	})
	if err != nil {
		t.Fatalf("ExportDiagnosticBundle() error = %v", err)
	}

	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("zip.OpenReader() error = %v", err)
	}
	defer archive.Close()

	for _, file := range archive.File {
		if file.Name == "core/startup_diagnostics.json" {
			t.Fatal("core/startup_diagnostics.json should not be present when CoreStartupDiagnostics is nil")
		}
	}
}
