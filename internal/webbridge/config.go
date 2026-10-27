package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type modelEntry struct {
	Name    string `json:"name"`
	APIBase string `json:"api_base"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type coreConfig struct {
	Models    []modelEntry `json:"models,omitempty"`
	Active    string       `json:"active_model,omitempty"`
	Language  string       `json:"language,omitempty"`
	LogDir    string       `json:"log_dir,omitempty"`
	TrustedWS []string     `json:"trusted_workspaces,omitempty"`
}

type resolvedCoreConfig struct {
	APIBase           string
	APIKeyMasked      string
	Model             string
	Language          string
	LogDir            string
	TrustedWorkspaces []string
	Path              string
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".eos.json"
	}
	return filepath.Join(home, ".eos.json")
}

func loadCoreConfig() resolvedCoreConfig {
	p := configPath()
	cfg := coreConfig{}
	raw, err := os.ReadFile(p)
	if err == nil {
		_ = json.Unmarshal(raw, &cfg)
	}

	language := strings.TrimSpace(cfg.Language)
	if language == "" {
		language = "zh"
	}
	logDir := resolveLogDir(cfg.LogDir)

	return resolvedCoreConfig{
		APIBase:           strings.TrimSpace(os.Getenv("EOS_API_BASE")),
		APIKeyMasked:      maskKey(strings.TrimSpace(os.Getenv("EOS_API_KEY"))),
		Model:             strings.TrimSpace(os.Getenv("EOS_MODEL")),
		Language:          language,
		LogDir:            logDir,
		TrustedWorkspaces: cfg.TrustedWS,
		Path:              p,
	}
}

func configuredLogDir() string {
	cfg := loadCoreConfig()
	return cfg.LogDir
}

func resolveLogDir(value string) string {
	trimmed := strings.TrimSpace(os.ExpandEnv(value))
	if trimmed == "" {
		return defaultSystemLogDir()
	}
	if strings.HasPrefix(trimmed, "~") {
		home, err := os.UserHomeDir()
		if err == nil && strings.TrimSpace(home) != "" {
			if trimmed == "~" {
				trimmed = home
			} else if strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "~\\") {
				trimmed = filepath.Join(home, trimmed[2:])
			}
		}
	}
	if !filepath.IsAbs(trimmed) {
		if abs, err := filepath.Abs(trimmed); err == nil {
			trimmed = abs
		}
	}
	return filepath.Clean(trimmed)
}

func maskKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "****"
	}
	return "****" + v[len(v)-4:]
}
