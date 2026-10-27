package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const workspaceTrustFileName = "trusted.json"

type WorkspaceTrustState struct {
	Trusted   bool   `json:"trusted"`
	TrustedAt string `json:"trusted_at,omitempty"`
}

func WorkspaceTrustPath(workspace string) string {
	workspace = NormalizeWorkspacePath(workspace)
	if workspace == "" {
		return filepath.Join(".eos", workspaceTrustFileName)
	}
	return filepath.Join(workspace, ".eos", workspaceTrustFileName)
}

func IsWorkspaceTrustedLocal(workspace string) bool {
	state, err := LoadWorkspaceTrust(workspace)
	return err == nil && state.Trusted
}

func LoadWorkspaceTrust(workspace string) (WorkspaceTrustState, error) {
	path := WorkspaceTrustPath(workspace)
	if strings.TrimSpace(path) == "" {
		return WorkspaceTrustState{}, errors.New("workspace trust path empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkspaceTrustState{}, nil
		}
		return WorkspaceTrustState{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return WorkspaceTrustState{}, nil
	}
	var state WorkspaceTrustState
	if err := json.Unmarshal(data, &state); err != nil {
		return WorkspaceTrustState{}, err
	}
	return state, nil
}

func TrustWorkspaceLocal(workspace string) error {
	path := WorkspaceTrustPath(workspace)
	if strings.TrimSpace(path) == "" {
		return errors.New("workspace trust path empty")
	}
	state := WorkspaceTrustState{
		Trusted:   true,
		TrustedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
