# EOS

[中文](./README.md) | [English](./README.en.md)

EOS 是一个开源的终端 AI 编码助手，当前以 Rust Core 作为核心运行时，Go 侧负责 CLI 入口、TUI、桥接与分发集成。它面向日常编码、代码审查、文档处理、本地自动化，以及 IDE / 平台集成场景，提供交互式 TUI、工具调用、安全门禁、工作区上下文和可扩展的 MCP 能力。

当前 `v1.0.0-beta.3` 起提供三端（Windows / macOS / Linux × amd64+arm64）生产包与 SHA256SUMS 校验，Windows 提供安装器与便携压缩包双通道，并与 EOS App 的桌面分发版本保持同版号。

- 项目仓库：https://github.com/eosaios/eos
- 问题反馈：https://github.com/eosaios/eos/issues
- 版本发布：https://github.com/eosaios/eos/releases

## 项目定位

EOS 不是单一的“问答 CLI”，而是一个完整的本地 AI 工作台：

- 对普通用户：提供开箱即用的终端交互体验，适合编码、排障、审查、检索和文档处理
- 对高级用户：支持 `--print` 无头调用、文档子命令、工作区管理、权限控制和上下文压缩
- 对平台 / IDE / Agent 宿主：提供本地 `serve` JSON-RPC、`bridge manifest` 桥接清单，以及标准 MCP Server

## 为什么选择 EOS

相对于依赖更重、接入范围更窄的同类终端助手，EOS 当前更强调以下几点：

- Rust Core 承接会话、运行时、工具调度、审批与沙箱等核心能力，Go CLI 负责轻量入口、TUI、桥接与跨平台分发，不依赖 Node.js 运行时
- 核心运行时与 CLI 入口分层后，EOS 的协议边界更清晰，也便于桌面端、IDE 插件、MCP Host 和外部平台复用同一套核心能力
- 工具执行、安全审批和沙箱策略集中在 Rust Core 中，减少多入口重复实现带来的行为不一致和安全风险
- 模型接入更开放，支持 OpenAI 兼容接口，不限制单一模型提供商
- 不只做代码补全，还覆盖文档读写转换、MCP、搜索、Git、远程仓库、子代理等完整工作流
- 对平台集成更友好，既能作为本地工具服务，也能直接暴露为标准 MCP Server
- 支持网页只读抓取和外接浏览器自动化，适合真实任务链路

## 核心能力

### 1) 终端交互体验

- 交互式 TUI，支持流式输出、Markdown 渲染、帮助面板和状态栏提示
- AI / Bash 双模式切换，适合在同一界面里完成问答、执行命令和结果回看
- 面板系统覆盖 `context`、`memory`、`rules`、`workspace`、`models`、`settings`、`mcp`、`lsp`、`cost`、`versions`、`tasks`
- 支持续聊、恢复指定会话、历史导航、上下文压缩和版本快照

### 2) 执行与安全

- 支持 `plan` / `auto` 两种执行模式
- 高风险工具调用支持审批与摘要校验，适合本地交互和平台托管
- 工具权限支持白名单 / 黑名单和工作区边界限制
- 会话、任务、审批、提问等状态可被外部宿主接管

### 3) 工具体系

- 文件与代码：读取、编辑、搜索、目录分析、Notebook 编辑、文件历史
- Shell 与任务：Bash、PowerShell、后台任务、长任务控制
- Git 与远程仓库：本地 Git 操作、远程仓库连接、克隆、分支、提交、推送、PR / MR 流程
- Web 与外部信息：`web_search`、`web_fetch`
- 多代理与扩展：子代理、团队协作、MCP、Skills、Plugins、结构化输出

### 4) 文档能力

- 内置 `DOCX` / `XLSX` / `PDF` 读取
- 支持生成 `DOCX` / `XLSX` / `PDF`
- 支持 `DOCX` / `XLSX` / `PDF` 之间的格式转换
- `DOCX <-> PDF`、`XLSX <-> PDF` 默认优先使用 `soffice` 做高保真转换；不可用时自动回退到内容级转换并返回告警

### 5) 上下文与语言能力

- 代码索引、文件监听、上下文构建与压缩
- 会话持久化与恢复
- 可选 LSP 能力，支持 Go、Python、TypeScript、JavaScript 自动检测

## 环境要求

- Go `1.25+`
- 可访问的 OpenAI 兼容接口
- 至少配置以下模型参数之一组：
  - 环境变量：`EOS_API_BASE`、`EOS_API_KEY`、`EOS_MODEL`
  - 用户配置文件：`~/.eos.json`

## 快速开始

### 1) 安装

一行命令安装（推荐，自动识别平台、SHA256 校验、配置 PATH）：

macOS / Linux：

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://raw.githubusercontent.com/eosaios/eos/main/scripts/install.ps1 | iex
```

安装后运行 `eos` 即可；`eos version` 查看版本，`eos update` 一键自升级（含 Rust 内核 core/ 同步更新）。

已安装 Go 工具链的用户也可以（内核内嵌，首次运行自动释放）：

```bash
go install github.com/eosaios/eos@latest
```

安装包（tar.gz / zip 与 Windows 安装器）仍从 Releases 获取：https://github.com/eosaios/eos/releases

从源码编译：

```bash
git clone https://github.com/eosaios/eos.git
cd eos
go mod tidy
go build -o eos
```

Windows:

```powershell
.\eos.exe
```

macOS / Linux:

```bash
./eos
```

### 2) 配置模型

方式 A：环境变量

```bash
export EOS_API_BASE="https://api.openai.com/v1"
export EOS_API_KEY="sk-..."
export EOS_MODEL="gpt-4o-mini"
```

方式 B：`~/.eos.json`

```json
{
  "models": [
    {
      "name": "default",
      "api_base": "https://api.openai.com/v1",
      "api_key": "sk-...",
      "model": "gpt-4o-mini"
    }
  ],
  "active_model": "default"
}
```

### 3) 直接开始使用

```bash
eos
```

首次进入后，可以先用 `?` 打开帮助面板，再通过 `/status`、`/workspace`、`/model`、`/mcp` 等命令查看当前状态。

## 常用命令

### 交互入口

```bash
eos
eos --continue
eos --resume <session-id>
eos --model <model-name>
eos --allowed-tools "read,search,bash"
eos --disallowed-tools "bash"
```

### 无头调用

适合脚本、CI 或外部调度：

```bash
eos --print "请总结当前仓库结构"
eos --print "review 当前改动" --output-format json
```

### 文档命令

```bash
eos doc read ./report.docx
eos doc generate --format pdf --output ./out/report.pdf --title "周报" --content "第一段\n\n第二段"
eos doc convert ./report.docx --to pdf --output ./out/report.pdf --fidelity high
```

### 更新

```bash
eos update
```

## 交互方式

### 常用快捷键

- `?`：打开帮助面板
- `F2`：切换 AI / Bash 模式
- `Tab`：切换思考显示或接受建议
- `Alt+V`：从剪贴板粘贴图片
- `→`：接受下一条预测内容
- `Ctrl+O`：切换实时详细显示
- `Alt+H`：展开或折叠当前思考内容
- `Ctrl+J`：输入换行
- `Esc`：停止当前流程
- `Ctrl+C`：中断或退出

### 常用斜杠命令

以下只是常见入口，不是完整列表：

- 通用：`/help`、`/status`、`/clear`、`/exit`、`/lang`
- 工作区与上下文：`/workspace`、`/context`、`/compact`
- 任务与计划：`/tasks`、`/plan`、`/permissions`
- 配置面板：`/model`、`/config`、`/mcp`、`/lsp`、`/rules`、`/cost`

## 开发者集成

普通用户通常只需要 `eos`、`eos --print`、`eos doc` 和 `eos update`。如果你要把 EOS 接入 IDE、自动化平台或其他 agent 宿主，当前有三条主线：

### 1) `eos serve`

把 EOS 作为本地工具服务运行，当前基于 `stdio` 按行输出 JSON-RPC 2.0，适合本地宿主、IDE bridge 和平台侧 agent。

```bash
eos serve --transport stdio --workspace "/abs/workspace"
```

文档见：[internal/docs/serve/API.md](./internal/docs/serve/API.md)

### 2) `eos bridge manifest`

生成桥接清单，输出启动命令、协议版本、默认会话参数、方法列表和能力声明，适合宿主侧自动发现 EOS 接入信息。

```bash
eos bridge manifest --workspace "/abs/workspace" --access-mode workspace-write --approval-mode on-request
```

文档见：[internal/docs/serve/IDE_BRIDGE.md](./internal/docs/serve/IDE_BRIDGE.md)

### 3) `eos mcp serve`

把 EOS 作为标准 MCP Server 暴露给外部 agent 或宿主，当前支持 `stdio` 和 `sse` 两种 transport。

```bash
eos mcp serve --transport stdio --workspace "/abs/workspace" --access-mode workspace-write --approval-mode on-request
eos mcp serve --transport sse --listen 127.0.0.1:8765 --workspace "/abs/workspace" --access-mode workspace-write --approval-mode on-request
```

文档见：[internal/docs/mcp/SERVER.md](./internal/docs/mcp/SERVER.md)

### 4) `eos web`

在浏览器里运行 EOS 桌面端工作台 UI：本地 HTTP 服务静态前端，通过 WebSocket 把
`BridgeService` RPC 与事件流桥接到 eos-core sidecar。仅监听 127.0.0.1。

```bash
eos web                                   # 默认 127.0.0.1:8788，自动打开浏览器
eos web --listen 127.0.0.1:9000 --workspace "/abs/workspace" --no-open
```

前端产物目录解析顺序：`--ui-dir` → `EOS_WEB_UI_DIR` → 工作目录/可执行文件目录
相邻的 `eos-app-src/frontend/dist`（或 `frontend/dist`）。构建发布版需先构建
eos-app 前端（`cd eos-app-src/frontend && npm run build`），或显式传 `--ui-dir`。


## MCP 与浏览器自动化

EOS 既可以作为 MCP Server 对外提供工具，也可以作为 MCP 客户端连接外部 MCP 服务。

### 推荐浏览器接入方式

EOS 当前不内置浏览器驱动，推荐通过 Playwright MCP 接入真实浏览器自动化能力。接入后，agent 可以执行点击、输入、选择、等待页面变化和截图等操作。

最小可用配置：

```json
[
  {
    "name": "playwright",
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "@playwright/mcp@latest"],
    "envs": {},
    "enabled": true
  }
]
```

启用方式：

- 在 `/mcp` 面板中按 `B` 插入 Playwright 预设
- 或手动编辑 `~/.eos.json` 中的 `mcp` 配置

能力边界：

- `web_fetch` 适合只读抓取网页内容
- 浏览器 MCP 适合真实页面交互、行为验证和截图
- 可通过 `/status`、MCP 面板或运行态信息检查连接状态

## 开源使用与发布建议

- 运行时会在工作目录生成 `.eos/` 数据，例如会话、检查点和版本快照
- 建议将 `.eos/`、`.eos.json`、`.env`、日志和本地配置加入忽略列表
- 对外发布前请检查 API Key、私钥、证书、绝对路径等敏感信息
- 如果你要给外部平台分发集成方案，建议同时提供 `serve` / `bridge manifest` / `mcp serve` 的使用边界说明

## 许可证

本项目采用 EOS 非商用许可证 v1.1，详见 [LICENSE](./LICENSE)。

- 个人和非商业用途可免费使用
- 允许编译、修改和分发非商业版本
- 衍生作品需在相同许可证下开源
- 禁止企业内部生产、收费服务、SaaS、商业再分发等商业使用
- 商业使用需获得版权人单独书面授权

## 联系方式

- 问题反馈：https://github.com/eosaios/eos/issues
- 商业合作 / 授权咨询：legal@eosaios.com
