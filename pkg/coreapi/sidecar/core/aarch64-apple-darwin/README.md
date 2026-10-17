# `aarch64-apple-darwin` vendored sidecar（macOS Apple Silicon）

此目录存放 macOS Apple Silicon（aarch64-apple-darwin，M1/M2/M3）的 eos-core sidecar 二进制 + manifest.json。

## 填充方式

2026-10-17 P0 本机配套：原生 Apple Silicon 构建 core `d65835a`，197 项方法，
SHA256 `8ab4b0d7177ef8cf8aa48de989d0893f2802ffcfab8d1ca408b6b2a1e3c07095`。
本次为 Cargo dev profile 制品，经既有受信 Ed25519 密钥签名，摘要与签名均已核验；
用于本地 P0 默认解析回归，不表示安装更新、正式发布或其他平台通过。
原193项配套及失败证据保留于方案的冻结/历史封存，新配套不改原金样。

由 GitHub Actions workflow `sync-vendored-sidecar.yml` 在 **macos-latest 原生 runner**
（Apple Silicon）上编译 + Ed25519 签名 + 校验 sha256 后自动 vendored 进此目录。

- 触发：手动（Actions → sync-vendored-sidecar → Run workflow），或 eos-core 发版后自动。
- 非 macOS 主机不在本流程中交叉编译 macOS；具备相应 SDK 的原生 macOS 主机可构建对应 target。

## 内容约定

- `eos-core`（无 .exe 后缀，macOS 可执行文件）
- `manifest.json`（含 sha256 + ed25519 signature + features）

勿手动编辑或放置未签名二进制。壳层加载时会校验 sha256 + 签名（`VerifyChecksum: true` + `RequireSignature: true`）。
