package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DiagnosticBundleOptions struct {
	DestinationPath        string
	LogFile                string
	Report                 string
	CoreStartupDiagnostics map[string]any
}

type diagnosticMetadata struct {
	ExportedAt      time.Time     `json:"exported_at"`
	Build           BuildMetadata `json:"build"`
	LogFile         string        `json:"log_file,omitempty"`
	LogFiles        []string      `json:"log_files,omitempty"`
	CoreConfigPath  string        `json:"core_config_path,omitempty"`
	CrashReportPath string        `json:"crash_report_path,omitempty"`
}

type coreConfigSummary struct {
	Path                  string `json:"path"`
	ActiveProfile         string `json:"active_profile,omitempty"`
	ModelCount            int    `json:"model_count"`
	SelectedModel         string `json:"selected_model,omitempty"`
	Language              string `json:"language,omitempty"`
	TrustedWorkspaceCount int    `json:"trusted_workspace_count"`
	APIBase               string `json:"api_base,omitempty"`
	APIKeyMasked          string `json:"api_key_masked,omitempty"`
}

func ExportDiagnosticBundle(opts DiagnosticBundleOptions) (string, error) {
	destination := normalizeDiagnosticBundlePath(opts.DestinationPath)
	if destination == "" {
		return "", errors.New("destination path required")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}

	file, err := os.Create(destination)
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = os.Remove(destination)
		}
	}()

	zipWriter := zip.NewWriter(file)
	if err := writeZipText(zipWriter, "report/summary.txt", buildDiagnosticReportText(opts.Report)); err != nil {
		return "", err
	}
	if err := writeZipJSON(zipWriter, "app/build.json", CurrentBuildMetadata()); err != nil {
		return "", err
	}

	metadata := diagnosticMetadata{
		ExportedAt:     time.Now().UTC(),
		Build:          CurrentBuildMetadata(),
		LogFile:        strings.TrimSpace(opts.LogFile),
		LogFiles:       collectLogFiles(opts.LogFile),
		CoreConfigPath: configPath(),
	}
	if report, err := LatestCrashReport(); err == nil && report != nil {
		metadata.CrashReportPath = report.Path
	}
	if err := writeZipJSON(zipWriter, "app/metadata.json", metadata); err != nil {
		return "", err
	}
	if err := writeZipJSON(zipWriter, "config/core_config_summary.json", buildCoreConfigSummary()); err != nil {
		return "", err
	}
	if opts.CoreStartupDiagnostics != nil {
		if err := writeZipJSON(zipWriter, "core/startup_diagnostics.json", opts.CoreStartupDiagnostics); err != nil {
			return "", err
		}
	}

	for _, path := range metadata.LogFiles {
		if err := addFileToZip(zipWriter, path, filepath.ToSlash(filepath.Join("logs", filepath.Base(path)))); err != nil {
			return "", err
		}
	}
	if report, err := LatestCrashReport(); err == nil && report != nil && strings.TrimSpace(report.Path) != "" {
		if err := addFileToZip(zipWriter, report.Path, "crash/last_crash.json"); err != nil {
			return "", err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	success = true
	return destination, nil
}

func normalizeDiagnosticBundlePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		return path
	}
	return path + ".zip"
}

func buildDiagnosticReportText(report string) string {
	report = strings.TrimSpace(report)
	if report == "" {
		return "当前没有可导出的运行时摘要。"
	}
	return report
}

func buildCoreConfigSummary() coreConfigSummary {
	summary := coreConfigSummary{
		Path: configPath(),
	}
	resolved := loadCoreConfig()
	summary.APIBase = strings.TrimSpace(resolved.APIBase)
	summary.APIKeyMasked = strings.TrimSpace(resolved.APIKeyMasked)
	summary.SelectedModel = strings.TrimSpace(resolved.Model)
	summary.Language = strings.TrimSpace(resolved.Language)
	summary.TrustedWorkspaceCount = len(resolved.TrustedWorkspaces)

	raw, err := os.ReadFile(summary.Path)
	if err != nil {
		return summary
	}
	var cfg coreConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return summary
	}
	summary.ActiveProfile = strings.TrimSpace(cfg.Active)
	summary.ModelCount = len(cfg.Models)
	if summary.Language == "" {
		summary.Language = strings.TrimSpace(cfg.Language)
	}
	if summary.TrustedWorkspaceCount == 0 {
		summary.TrustedWorkspaceCount = len(cfg.TrustedWS)
	}
	return summary
}

func collectLogFiles(logFile string) []string {
	logFile = strings.TrimSpace(logFile)
	if logFile == "" {
		return nil
	}
	dir := filepath.Dir(logFile)
	base := filepath.Base(logFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if _, statErr := os.Stat(logFile); statErr == nil {
			return []string{logFile}
		}
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name != base && !strings.HasPrefix(name, base+".") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files
}

func writeZipJSON(zipWriter *zip.Writer, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeZipText(zipWriter, name, string(raw))
}

func writeZipText(zipWriter *zip.Writer, name, content string) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(content))
	return err
}

func addFileToZip(zipWriter *zip.Writer, path, name string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(raw)
	return err
}
