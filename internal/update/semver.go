package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strconv"
	"strings"
)

// compareVersions 按 semver 语义比较两个版本号（v 前缀可选），返回 -1 / 0 / 1。
//
// 为什么不用字符串比较：`1.0.0-beta.10 > 1.0.0-beta.3` 在字典序下不成立
// （"1" < "3"），beta.10 会被判为旧版导致升级检测失灵。这里按 semver 规则：
//   - X.Y.Z 主段按数值逐段比较；
//   - 带 prerelease 的版本小于不带 prerelease 的版本；
//   - prerelease 按点分段：纯数字段之间数值比较，数字段 < 字母段，
//     字母段之间字典序；段数多的大（有 prerelease 前提下）。
func compareVersions(a, b string) int {
	pa, preA := splitVersion(a)
	pb, preB := splitVersion(b)

	for i := 0; i < 3; i++ {
		da, db := pa[i], pb[i]
		if da != db {
			if da < db {
				return -1
			}
			return 1
		}
	}

	// 主版本相同：不带 prerelease > 带 prerelease
	if preA == "" && preB == "" {
		return 0
	}
	if preA == "" {
		return 1
	}
	if preB == "" {
		return -1
	}

	return comparePrerelease(preA, preB)
}

// splitVersion 拆出 [3]uint64 主版本与 prerelease 串。build metadata（+ 之后）
// 按 semver 规范不参与比较；解析失败的段按 0 处理，保证宽容输入不 panic。
func splitVersion(v string) ([3]uint64, string) {
	var out [3]uint64
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")

	// 先剥 build metadata（+），再取 prerelease（- 之后）
	if plus := strings.Index(v, "+"); plus >= 0 {
		v = v[:plus]
	}
	if idx := strings.Index(v, "-"); idx >= 0 {
		out[0], out[1], out[2] = parseNumericTriple(v[:idx])
		return out, v[idx+1:]
	}
	out[0], out[1], out[2] = parseNumericTriple(v)
	return out, ""
}

func parseNumericTriple(s string) (uint64, uint64, uint64) {
	parts := strings.SplitN(s, ".", 3)
	var nums [3]uint64
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.ParseUint(parts[i], 10, 64)
		nums[i] = n
	}
	return nums[0], nums[1], nums[2]
}

func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.ParseUint(as[i], 10, 64)
		bn, bErr := strconv.ParseUint(bs[i], 10, 64)
		switch {
		case aErr == nil && bErr == nil:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aErr == nil:
			return -1 // 数字标识 < 字母标识
		case bErr == nil:
			return 1
		default:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

func isNewer(current, latest string) bool {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	latest = strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if latest == "" || current == "" {
		return false
	}
	// latest 与 current 比较，latest 更大才算有更新
	return compareVersions(latest, current) > 0
}
