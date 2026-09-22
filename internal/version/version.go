package version

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

var (
	// AppVersion 是应用程序的版本号
	AppVersion = "v1.0.0-beta.28"

	// BuildCommit 是构建时的 git commit hash（通过 -ldflags 注入）
	BuildCommit = "unknown"

	// BuildDate 是构建时间（通过 -ldflags 注入，RFC3339 格式）
	BuildDate = ""
)

const (
	// AppName 是应用程序名称
	AppName = "EOS"
)
