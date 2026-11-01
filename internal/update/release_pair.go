package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

const releasePairFile = "release-pair.json"

type releasePair struct {
	CLIVersion       string `json:"cli_version"`
	CLICommit        string `json:"cli_commit"`
	CoreSourceCommit string `json:"core_source_commit"`
	CoreTarget       string `json:"core_target"`
	CoreSHA256       string `json:"core_sha256"`
}

func validateReleasePair(root, version string) error {
	data, err := os.ReadFile(filepath.Join(root, releasePairFile))
	if err != nil {
		return fmt.Errorf("read release pair: %w", err)
	}
	var pair releasePair
	if err := json.Unmarshal(data, &pair); err != nil {
		return fmt.Errorf("invalid release pair: %w", err)
	}
	if pair.CLIVersion != version || !validHex(pair.CLICommit, 40) || !validHex(pair.CoreSourceCommit, 40) || !validHex(pair.CoreSHA256, 64) {
		return fmt.Errorf("release pair has invalid version, commit or digest for %s", version)
	}
	allowedTarget := false
	for _, target := range sidecar.TargetTriples(runtime.GOOS, runtime.GOARCH) {
		if pair.CoreTarget == target {
			allowedTarget = true
			break
		}
	}
	if !allowedTarget {
		return fmt.Errorf("release pair targets unsupported platform %q", pair.CoreTarget)
	}
	manifestPath := filepath.Join(root, "core", pair.CoreTarget, sidecar.DefaultManifestName)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read release core manifest: %w", err)
	}
	manifest, err := sidecar.LoadManifestBytes(manifestData)
	if err != nil {
		return err
	}
	var provenance struct {
		SourceCommit string `json:"source_commit"`
	}
	if err := json.Unmarshal(manifestData, &provenance); err != nil {
		return err
	}
	if provenance.SourceCommit != pair.CoreSourceCommit || manifest.Target != pair.CoreTarget || strings.TrimPrefix(manifest.SHA256, "sha256:") != pair.CoreSHA256 {
		return fmt.Errorf("release pair differs from the core manifest")
	}
	// 发布包的 Core 必须位于目标目录内，不接受绝对路径或越界路径。
	if filepath.Base(manifest.Binary) != manifest.Binary || manifest.Binary == "." {
		return fmt.Errorf("release core binary must be inside its target directory")
	}
	if err := manifest.VerifyBinary(filepath.Join(filepath.Dir(manifestPath), manifest.Binary)); err != nil {
		return fmt.Errorf("verify release core: %w", err)
	}
	return nil
}

func validHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
