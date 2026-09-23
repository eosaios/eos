# eos-cli scripts

本目录的开发辅助脚本与 CI 配置。

## sidecar 构建（内核二进制分发）

eos-cli / eos-app 各自 vendored 一份 eos-core sidecar 二进制，两个壳层独立。
sidecar 构建分两条路径，职责严格分工：

### 本机开发迭代（仅 host target）

**`dev-rebuild.ps1`** — 改 eos-core-rs 内核后，一键重编当前 host target
（Windows 上 = `x86_64-pc-windows-gnu`）+ Ed25519 签名 + 分发到 eos-cli 和
eos-app 的 windows-gnu 目录，用于本地快速验证。

```powershell
powershell -File scripts/dev-rebuild.ps1            # 完整 build+clippy+test+签名分发
powershell -File scripts/dev-rebuild.ps1 -SkipBuild # 用已编译二进制重签分发
powershell -File scripts/dev-rebuild.ps1 -SkipApp   # 只分发 eos-cli，不碰 eos-app
```

⚠️ **只产 host target**。本机无法交叉编译 macOS（缺 macOS SDK，法律+技术双重
不可行），Linux 需 zig + 改 reqwest 为 rustls（工具链成本高）。跨平台走 CI。

### 跨平台生产构建（5 target，CI 全自动）

**`.github/workflows/sync-vendored-sidecar.yml`** — 在各平台原生 runner 上
构建 + 签名 + 自动 vendored 进仓库，覆盖全部 5 target：

| target | runner |
|---|---|
| `x86_64-pc-windows-gnu` | windows-latest |
| `x86_64-apple-darwin` | macos-13 |
| `aarch64-apple-darwin` | macos-latest |
| `x86_64-unknown-linux-gnu` | ubuntu-latest |
| `aarch64-unknown-linux-gnu` | ubuntu-24.04-arm |

触发：GitHub Actions → sync-vendored-sidecar.yml → Run workflow（手动）。
build job 产出签名 artifact，sync job 分发到 `pkg/coreapi/sidecar/core/<triple>/`
并 commit 回仓库。可选 `sync_app=true` 同步到下游 eos-app 仓库（需 deploy key secret）。

需要的 secrets（在 eos-cli 仓库 Settings → Secrets 配置）：
- `EOS_SIGNING_KEY`：Ed25519 签名私钥（PKCS#8 PEM 内容，与 eos-core-rs/scripts/eos-signing-key.pem 一致）
- `EOS_CORE_REPO`：eos-core-rs 的 GitHub 仓库（owner/name，默认 eosaios/eos-core-rs）
- `EOS_CORE_PAT`：若 eos-core-rs 是私有仓，需 PAT 访问（公开仓留空）
- `EOS_APP_GIT_URL` + `EOS_APP_DEPLOY_KEY`（可选）：同步到下游 eos-app 仓库

## 其它脚本

- `regen-protocol.ps1` — 重新生成 protocol schema（schema.json 变更后跑）
- `generate_eos_icon.ps1` — 生成应用图标
