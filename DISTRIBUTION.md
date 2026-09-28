# EOS CLI 发行说明（DISTRIBUTION）

> 记录 CLI 的对外发行配置，供接手者（含 AI 会话）直接引用。最后更新：2026-09-28。
> 账号与凭证统一记录在私有仓 `eos-app-src` 的 `RELEASE_CREDENTIALS.md`，本文件不重复。

## winget 包

- 包 ID：**`EOSAIOS.EOSCLI`**（portable zip）
- 首次提交：microsoft/winget-pkgs PR #442425（2026-09-28 已过自动校验）
- fork：`eosaios/winget-pkgs`
- beta 策略：单包 ID 走到底，版本号如实写 `1.0.0-beta.N`；将来发 `1.0.0` 正式版时 beta 用户经 `winget upgrade` 无缝升级
- 安装命令：`winget install EOSAIOS.EOSCLI`

## 归档结构红线（改打包逻辑前必读）

- tar.gz：有顶层目录 `eos-cli_<ver>_<os>-<arch>/`，内含 `eos` + `core/<triple>/`
- zip：**文件在归档根**（`eos.exe` + `core/`），winget manifest 的 `RelativeFilePath: eos.exe` 依赖此结构
- `install.sh` 按"解压后顶层目录中存在 eos"定位，不依赖 find -maxdepth（macOS BSD find 不支持）

## 安装脚本

- mac/Linux 入口：`curl -fsSL https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.sh | bash`
- Windows 入口：`irm https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.ps1 | iex`
- 入口必须走 jsdelivr CDN（`raw.githubusercontent.com` 国内不可达）
- jsdelivr 对 `@main` 缓存 12 小时：改脚本后需 purge 或等过期
- `install.ps1` 含 `$ProgressPreference = "SilentlyContinue"`（PS5.1 提速必须）

## 发版自动化

`.github/workflows/release.yml` 末尾的 `winget` job：Release 创建后自动跑 wingetcreate `update --submit` 提 manifest PR。

- 前置：本仓库 Settings → Secrets 已配 `WINGET_TOKEN`（classic PAT，见私有仓凭证文档）
- 开关：仓库 Variables 设 `WINGET_SUBMIT=off` 停用
- 注意：`wingetcreate update` 要求包已存在于 winget，首次手动 PR 合并前自动 job 失败属预期

## 签名

CLI 走命令行下载，**无 Mark-of-the-Web，天然不弹 SmartScreen**，无需购买代码签名证书。

## 文案红线

许可证为非商用（保留所有权利），**对外文案禁止"开源 / open-source"字样**，正确表述是"源码可见"。
