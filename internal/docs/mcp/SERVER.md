# eos mcp serve — 标准 MCP Server（agent 后端）

`eos mcp serve` 把 EOS 作为**标准 MCP（Model Context Protocol）Server** 暴露给外部
agent 或宿主。任何符合 MCP 规范的客户端（Claude Desktop、Cursor、其它编码 agent 等）
都能：

1. **委托完整编码任务**：`eos_chat` 把一条消息交给 EOS agent 跑完整一轮（读文件、
   编辑、执行命令、子 agent 等全部能力），阻塞至收尾并返回结构化结果。
2. **直通原子工具**：EOS 工具目录（`tool/catalog`）逐个映射为 MCP tool。
3. **管理会话与审批闭环**：会话增删查、审批/问询的列出与回应、任务等待与打断。

## 与 eos serve 的区别

| | `eos serve` | `eos mcp serve` |
|---|---|---|
| 协议 | EOS 私有 JSON-RPC（`session/create` 等） | 标准 MCP（`tools/list`、`tools/call`） |
| 适合 | 深度集成、需要 turn 编排和事件流的宿主 | 任意 MCP 客户端接入 |
| transport | stdio | stdio + sse |
| 能力范围 | 全部 ~135 个 method | 工具直通 + eos_* 委托/控制工具集 |

## Transport

```bash
# stdio（本地宿主）
eos mcp serve --transport stdio --workspace "/abs/workspace"

# sse（远程/网络宿主）
eos mcp serve --transport sse --listen 127.0.0.1:8765 --workspace "/abs/workspace"
```

基于 [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) 实现。stdio transport
以 5 worker 并发处理 `tools/call`——阻塞中的 `eos_chat` 不会卡住并发的
`eos_approval_respond` / `eos_task_status`。

## 工具总览

### 1. 任务委托（核心）

| 工具 | 关键参数 | 行为 |
|---|---|---|
| `eos_chat` | `message`（必填）、`session_id?`、`timeout_secs?`（默认 600，上限 3600） | 委托一轮完整任务；返回 JSON（见下） |
| `eos_task_wait` | `session_id`（必填）、`timeout_secs?` | 等待会话当前 turn 收尾（审批回应后的续跑、超时后的任务）；会话空闲立即返回最近一轮结果 |
| `eos_task_status` | `session_id` | 运行中/待审批/待输入状态 + 最近回复摘要 |
| `eos_task_cancel` | `session_id` | 打断当前 turn（异步生效；已落盘变更不回滚） |

`eos_chat` / `eos_task_wait` 的 JSON 结果：

```jsonc
{
  "status": "completed | waiting_approval | request_user_input | timeout | error",
  "session_id": "…", "turn_id": "…",
  "reply": "最终 assistant 回复",
  "files_changed": [{ "path": "main.go", "status": "M", "additions": 3, "deletions": 1 }],
  "approval": { "approval_id": "…", "tool_name": "edit", "reason": "…" },  // waiting_approval 时
  "questions": { … },                                                      // request_user_input 时透传
  "error": "…", "note": "下一步指引"
}
```

- **遇审批/问询立即快速返回**（`waiting_approval` / `request_user_input`），不干等超时。
- **超时 ≠ 失败**：turn 继续在后台运行，用 `eos_task_status` / `eos_task_wait` 续查。
- 续聊：携带上一次返回的 `session_id` 再次 `eos_chat`。

### 2. 会话管理

| 工具 | 行为 |
|---|---|
| `eos_session_create` | 显式建会话（`workspace?` / `title?`），多任务隔离用 |
| `eos_sessions_list` | 会话列表（运行状态、待审批计数，来自 state/snapshot） |
| `eos_session_history` | 尾部 N 条消息（`session_id` + `limit?` 默认 20） |

### 3. 审批/问询闭环

| 工具 | 行为 |
|---|---|
| `eos_approvals_list` | 待处理审批（`session_id?` 过滤） |
| `eos_approval_respond` | `approval_id` + `decision`（`accept` / `accept_for_session` / `decline` / `cancel`）+ `reason?` |
| `eos_inquiry_respond` | `inquiry_id` + `option?` + `text?`（Plan 模式问询） |

**闭环时序**（宿主视角）：

```
宿主                         eos mcp serve
 │ eos_chat {message}            │
 │──────────────────────────────▶│ turn/start → 事件流
 │◀────── waiting_approval ──────│ （快速返回，含 approval_id）
 │ （问用户 / 自行决策）
 │ eos_approval_respond {accept} │
 │──────────────────────────────▶│ approval/respond → turn 续跑
 │ eos_task_wait {session_id}    │
 │──────────────────────────────▶│ 轮询至收尾
 │◀────── completed + reply ─────│
```

内核的 prompt timeout 自动拒绝兜底仍然生效：宿主始终不回应时，按设置自动
decline/推荐项，turn 不会永久挂起。

### 4. 原子工具直通（MVP 行为保留）

`tools/list` 映射 EOS 工具目录（仅 `Invocable == true`），每个工具保留原名，
schema 从 EOS 参数定义构造；`tools/call` 注入会话后调 `tool/execute`。高风险
调用返回 `isError=true` + 结构化提示——宿主可改用 `eos_chat`（走完整审批流）
而非直通。

## 会话语义

- 每个 MCP server 进程持有**一个懒创建的默认会话**（`session/current` 复用，否则
  `session/create`，metadata `source=mcp`）。
- 三种定向方式（优先级从高到低）：工具参数 `session_id` → 请求 `_meta.session_id`
  → 默认会话。
- `eos_chat` 返回的 `session_id` 用于续聊；多任务隔离用 `eos_session_create`。

## 启动选项

| flag | 说明 |
|---|---|
| `--transport` | `stdio`（默认）或 `sse` |
| `--workspace` | 工作区根目录（默认当前目录） |
| `--listen` | SSE 监听地址（默认 `127.0.0.1:8765`，仅 sse） |
| `--access-mode` | `read-only` / `workspace-write` / `danger-full-access` |
| `--approval-mode` | `untrusted` / `on-failure` / `on-request` / `never` |
| `--sandbox-mode` | `workspace` / `full_access`（legacy 别名） |
| `--dangerously-skip-permissions` | 等价 `--access-mode danger-full-access --approval-mode never` |
| `--model` | 模型覆盖（条目名/模型 ID/套餐 label），对每个会话首次 `eos_chat` 应用 |
| `--chat-timeout` | `eos_chat` / `eos_task_wait` 默认阻塞/等待秒数（默认 600；按次 `timeout_secs` 覆盖） |

模型鉴权沿用 `~/.eos.json` 的模型条目（含 API key），MCP 协议面不暴露任何密钥。

## 客户端配置示例

### stdio（Claude Desktop 风格）

```jsonc
{
  "mcpServers": {
    "eos": {
      "command": "eos",
      "args": ["mcp", "serve", "--transport", "stdio", "--workspace", "/abs/workspace"]
    }
  }
}
```

### sse

客户端指向 `http://127.0.0.1:8765`（由 `--listen` 指定），按 MCP SSE transport 连接。

## 设计原则

（AGENTS.md「充分信任但不做无谓限制」）不替用户自动批准高风险操作，但也不把
agent 关进笼子——能力边界交给模型和 prompt，审批只防低级事故。宿主对
`waiting_approval` 拥有完全决策权；`--dangerously-skip-permissions` 留给显式
选择全放行的场景。

## 后续迭代（当前不做）

- MCP `resources/list` / `resources/read`（`eos://sessions` 转写资源）
- MCP `prompts/list` / `prompts/get`
- SSE 连接级会话隔离精细化
- `eos mcp list/add`（管理外部 MCP server 的客户端命令）
- 与 `eos-core --mcp-serve`（Rust 内核自带的 2 工具最小实现）合并统一
