package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildDate    = ""
)

type BuildMetadata struct {
	AppName     string `json:"app_name"`
	Version     string `json:"version"`
	Commit      string `json:"commit,omitempty"`
	BuildDate   string `json:"build_date,omitempty"`
	GoVersion   string `json:"go_version"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	MainPath    string `json:"main_path,omitempty"`
	MainVersion string `json:"main_version,omitempty"`
}

func CurrentBuildMetadata() BuildMetadata {
	info := BuildMetadata{
		AppName:   "EOS GUI",
		Version:   strings.TrimSpace(BuildVersion),
		Commit:    strings.TrimSpace(BuildCommit),
		BuildDate: strings.TrimSpace(BuildDate),
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.MainPath = strings.TrimSpace(bi.Main.Path)
		info.MainVersion = strings.TrimSpace(bi.Main.Version)
	}
	return info
}
