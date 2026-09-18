# EOS

[中文](./README.md) | [English](./README.en.md)

EOS is an open-source terminal AI coding assistant with Rust Core as its core runtime, while the Go side provides the CLI entry point, TUI, bridge layer, and distribution integration. It is designed for day-to-day coding, code review, document workflows, local automation, and IDE / platform integration, with an interactive TUI, tool calling, safety controls, workspace-aware context, and extensible MCP support.

Since `v1.0.0-beta.3`, releases ship production packages for three platforms (Windows / macOS / Linux, amd64 + arm64) with SHA256SUMS verification. Windows offers both a setup installer and a portable archive, version-aligned with the EOS App desktop distribution.

- Repository: https://github.com/eosaios/eos
- Issues: https://github.com/eosaios/eos/issues
- Releases: https://github.com/eosaios/eos/releases

## What EOS Is

EOS is more than a chat-style CLI. It is a local AI workbench with three main usage modes:

- For end users: an interactive terminal assistant for coding, debugging, review, search, and document handling
- For power users: headless `--print` execution, document subcommands, workspace management, permission control, and context compaction
- For platforms / IDEs / agent hosts: local `serve` JSON-RPC, `bridge manifest`, and a standard MCP server

## Why EOS

Compared with heavier or more closed terminal assistants, EOS currently focuses on:

- Rust Core handles sessions, runtime, tool orchestration, approvals, and sandboxing, while the Go CLI provides a lightweight entry point, TUI, bridge layer, and cross-platform distribution with no Node.js runtime requirement
- Separating the core runtime from the CLI entry point gives EOS a clearer protocol boundary and allows the desktop app, IDE plugins, MCP hosts, and external platforms to reuse the same core capabilities
- Tool execution, safety approvals, and sandbox policies are centralized in Rust Core, reducing inconsistent behavior and security risk across multiple entry points
- OpenAI-compatible model access instead of a single vendor lock-in
- Full workflows beyond coding chat: documents, MCP, search, Git, remote repositories, subagents, and task orchestration
- Practical integration surfaces for local hosts, IDE bridges, and agent platforms
- Both read-only web fetching and real browser automation through external MCP servers

## Core Capabilities

### 1) Terminal UX

- Interactive TUI with streaming output, Markdown rendering, a help panel, and status hints
- AI / Bash dual-mode workflow in one interface
- Panels for `context`, `memory`, `rules`, `workspace`, `models`, `settings`, `mcp`, `lsp`, `cost`, `versions`, and `tasks`
- Conversation resume, session restore, history navigation, context compaction, and version snapshots

### 2) Execution and Safety

- Two execution modes: `plan` and `auto`
- Approval flow and approval digest checks for higher-risk tools
- Tool allowlist / denylist controls and workspace boundary enforcement
- Sessions, tasks, approvals, and inquiries can be handled by an external host

### 3) Tooling

- Files and code: read, edit, search, project structure, notebooks, and file history
- Shell and tasks: Bash, PowerShell, background jobs, and task control
- Git and remote repos: local Git operations plus remote repository connect / clone / branch / push / PR / MR flows
- Web and external info: `web_search` and `web_fetch`
- Multi-agent and extensibility: subagents, team tools, MCP, Skills, Plugins, and structured output

### 4) Document Workflows

- Built-in `DOCX` / `XLSX` / `PDF` reading
- Built-in `DOCX` / `XLSX` / `PDF` generation
- Format conversion across `DOCX` / `XLSX` / `PDF`
- `DOCX <-> PDF` and `XLSX <-> PDF` prefer `soffice` for high-fidelity conversion, with automatic fallback to content-level conversion when unavailable

### 5) Context and Language Support

- Code indexing, file watching, context assembly, and context compaction
- Persistent sessions with restore support
- Optional LSP support with auto-detection for Go, Python, TypeScript, and JavaScript

## Requirements

- Go `1.25+`
- An accessible OpenAI-compatible endpoint
- Model configuration via one of:
  - Environment variables: `EOS_API_BASE`, `EOS_API_KEY`, `EOS_MODEL`
  - User config file: `~/.eos.json`

## Quick Start

### 1) Install

One-line install (recommended; detects your platform, verifies SHA256, sets up PATH):

macOS / Linux:

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/eosaios/eos/main/scripts/install.ps1 | iex
```

Then run `eos`. Use `eos version` to check the version and `eos update` to self-upgrade (the Rust core sidecar is updated together with the binary).

If you already have the Go toolchain (the Rust core is embedded and self-extracts on first run):

```bash
go install github.com/eosaios/eos@latest
```

Packages (tar.gz / zip plus the Windows installer) remain available at https://github.com/eosaios/eos/releases

### 2) Build from Source

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

### 2) Configure a Model

Option A: environment variables

```bash
export EOS_API_BASE="https://api.openai.com/v1"
export EOS_API_KEY="sk-..."
export EOS_MODEL="gpt-4o-mini"
```

Option B: `~/.eos.json`

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

### 3) Start Using EOS

```bash
eos
```

After launch, use `?` to open the help panel, then inspect your environment with `/status`, `/workspace`, `/model`, or `/mcp`.

## Common Commands

### Interactive Entry Points

```bash
eos
eos --continue
eos --resume <session-id>
eos --model <model-name>
eos --allowed-tools "read,search,bash"
eos --disallowed-tools "bash"
```

### Headless Execution

Useful for scripts, CI, and external automation:

```bash
eos --print "Summarize the current repository structure"
eos --print "review the current changes" --output-format json
```

### Document Commands

```bash
eos doc read ./report.docx
eos doc generate --format pdf --output ./out/report.pdf --title "Weekly Report" --content "Paragraph one\n\nParagraph two"
eos doc convert ./report.docx --to pdf --output ./out/report.pdf --fidelity high
```

### Self Update

```bash
eos update
```

## Interaction Model

### Common Shortcuts

- `?`: open the help panel
- `F2`: switch between AI and Bash mode
- `Tab`: toggle thinking display or accept a suggestion
- `Alt+V`: paste an image from the clipboard
- `→`: accept next-message prediction
- `Ctrl+O`: toggle live verbose display
- `Alt+H`: expand or collapse current thinking content
- `Ctrl+J`: insert a new line
- `Esc`: stop the current flow
- `Ctrl+C`: interrupt or exit

### Common Slash Commands

These are common entry points, not the full list:

- General: `/help`, `/status`, `/clear`, `/exit`, `/lang`
- Workspace and context: `/workspace`, `/context`, `/compact`
- Tasks and planning: `/tasks`, `/plan`, `/permissions`
- Config panels: `/model`, `/config`, `/mcp`, `/lsp`, `/rules`, `/cost`

## Developer Integration

Most users only need `eos`, `eos --print`, `eos doc`, and `eos update`. If you are integrating EOS into an IDE, automation platform, or another agent host, there are three primary paths:

### 1) `eos serve`

Runs EOS as a local tool service over line-delimited JSON-RPC 2.0 on `stdio`.

```bash
eos serve --transport stdio --workspace "/abs/workspace"
```

Docs: [internal/docs/serve/API.md](./internal/docs/serve/API.md)

### 2) `eos bridge manifest`

Generates a bridge manifest containing launch command, protocol version, session defaults, supported methods, and capability metadata for host-side auto-discovery.

```bash
eos bridge manifest --workspace "/abs/workspace"
```

Docs: [internal/docs/serve/IDE_BRIDGE.md](./internal/docs/serve/IDE_BRIDGE.md)

### 3) `eos mcp serve`

Runs EOS as a standard MCP server with `stdio` or `sse` transport.

```bash
eos mcp serve --transport stdio --workspace "/abs/workspace"
eos mcp serve --transport sse --listen 127.0.0.1:8765 --workspace "/abs/workspace"
```

Docs: [internal/docs/mcp/SERVER.md](./internal/docs/mcp/SERVER.md)

## MCP and Browser Automation

EOS can both expose itself as an MCP server and connect to external MCP services as a client.

### Recommended Browser Setup

EOS does not ship with a built-in browser driver. The recommended path is to connect Playwright MCP for real browser automation. Once enabled, the agent can click, type, select, wait for page changes, and capture screenshots in a real session.

Minimum working configuration:

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

How to enable:

- Press `B` in the `/mcp` panel to insert the Playwright preset
- Or edit the `mcp` section in `~/.eos.json` manually

Boundaries:

- `web_fetch` is for read-only page fetching
- Browser MCP is for real interaction, verification, and screenshots
- Use `/status`, the MCP panel, or runtime status to inspect connectivity

## Open-Source Usage Notes

- EOS creates `.eos/` runtime data in the working directory, including sessions, checkpoints, and version snapshots
- Exclude `.eos/`, `.eos.json`, `.env`, logs, and local config from version control
- Check for sensitive data before publishing, including API keys, private keys, certificates, and absolute paths
- If you publish an integration package for others, document the boundaries of `serve`, `bridge manifest`, and `mcp serve`

## License

This project is released under the EOS Non-Commercial License v1.1. See [LICENSE](./LICENSE).

- Free for personal and non-commercial use
- Non-commercial compilation, modification, and redistribution are allowed
- Derivative works must remain open source under the same license
- Commercial use is prohibited, including internal enterprise production, paid services, SaaS, and commercial redistribution
- Commercial use requires separate written authorization from the copyright holder

## Contact

- Issues: https://github.com/eosaios/eos/issues
- Commercial licensing: legal@eosaios.com
