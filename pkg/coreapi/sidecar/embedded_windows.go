//go:build windows && !eos_noembed

// 内嵌内核是 CLI 二进制的分发形态（go install 兜底）；桌面端构建传
// -tags eos_noembed 排除（自带 core/ 布局，避免归档多 9MB 冗余）。

package sidecar

import _ "embed"

// vendored 内核（dev-rebuild / sync workflow 产出的签名产物），仅内嵌当前
// 平台的一份，避免二进制体积随平台数膨胀。
//
//go:embed core/x86_64-pc-windows-gnu/eos-core.exe
var embeddedCoreBinary []byte

//go:embed core/x86_64-pc-windows-gnu/manifest.json
var embeddedCoreManifestBytes []byte

func init() {
	embeddedCoreSidecar = func() ([]byte, []byte, bool) {
		return embeddedCoreBinary, embeddedCoreManifestBytes, true
	}
}
