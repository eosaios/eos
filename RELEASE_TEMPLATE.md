# EOS 发布模板（GitHub Release）

> 与 `.github/workflows/release.yml` 产出对齐。
> 产物为 5 平台归档 + `SHA256SUMS.txt`，**不含** Inno Setup 安装器。
> 文件名中的版本号含 `v` 前缀（与 git tag 一致，如 `v1.0.0-beta.29`）。

## 标题

`EOS CLI <版本号>`

## 概要

<一句话本版本重点：面向什么场景、解决什么问题。>

## 下载

| 平台 | 资产 |
|---|---|
| Windows x64 | `eos-cli_<版本号>_windows-amd64.zip` |
| macOS Intel | `eos-cli_<版本号>_darwin-amd64.tar.gz` |
| macOS Apple Silicon | `eos-cli_<版本号>_darwin-arm64.tar.gz` |
| Linux x64 | `eos-cli_<版本号>_linux-amd64.tar.gz` |
| Linux ARM64 | `eos-cli_<版本号>_linux-arm64.tar.gz` |
| 校验文件 | `SHA256SUMS.txt` |

## 校验（SHA256）

```text
<sha256>  eos-cli_<版本号>_windows-amd64.zip
<sha256>  eos-cli_<版本号>_darwin-amd64.tar.gz
<sha256>  eos-cli_<版本号>_darwin-arm64.tar.gz
<sha256>  eos-cli_<版本号>_linux-amd64.tar.gz
<sha256>  eos-cli_<版本号>_linux-arm64.tar.gz
```

完整清单以资产 `SHA256SUMS.txt` 为准。

## 更新内容

### 新增

- <新能力 / 新命令 / 新协议。>

### 变更

- <行为或接口变化；无则删除本节。>

### 修复

- <缺陷修复；无则删除本节。>

## 升级说明

- 推荐用一行安装脚本覆盖升级（自动校验 SHA256 并配置 PATH）：
  - macOS / Linux：`curl -fsSL https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.sh | bash`
  - Windows（PowerShell）：`irm https://raw.githubusercontent.com/eosaios/eos/main/scripts/install.ps1 | iex`
- 已安装用户也可直接 `eos update` 自升级（含 Rust 内核 core/ 同步更新）。
- 或从 Releases 手动下载对应平台归档，解压后覆盖安装目录。
- 首次启动前请配置模型参数：`EOS_API_BASE`、`EOS_API_KEY`、`EOS_MODEL`（或写入 `~/.eos.json`）。

## 已知事项

- <平台限制 / 已知告警；无则删除本节。>
- Windows 终端可能出现 CRLF 提示，不影响运行。

## 许可证

- 个人/非商业用途免费。
- 商业用途需单独书面授权。
- 详见仓库中的 `LICENSE` 文件。
