# EOS 工程规范（AGENTS.md）

> 本文档适用于 EOS 全部三个仓库（eos-core-rs / eos-cli / eos-app）。
> 所有 AI agent 和人类贡献者在修改代码前必须阅读并遵守。

## 核心原则

### 1. 不留兼容兜底

- **不要写兼容层、回退路径、兜底逻辑来掩盖问题。** 每条代码路径必须是明确且可预测的。
- 行为不符合预期 = 有 bug，要**修 bug**，不能用"如果失败了就 fallback 到 XX"来掩盖。
- 禁止 `if ... else { /* 兜底放行/默认值 */ }` 模式——要么明确处理所有分支，要么在编译期/启动时 fail-fast。
- 例外：用户输入解析（如环境变量）允许合理的默认值，但必须有显式注释说明为什么这个默认值是安全的。

### 2. 模块化、高内聚、低耦合

- 每个 crate / package / module 只负责一个明确的职责。
- 依赖方向必须单向，禁止循环依赖。
- 跨 crate 调用走 trait 抽象（定义在 protocol / base 层），不直接 import 上层实现。
- 新功能优先考虑放进已有的合适 crate，而非创建新 crate（除非职责确实独立）。

### 3. 封装复用、归一化、统一接口

- 相似功能必须归一化：如果有 3 个地方在做"文件路径解析"，应该有一个公共函数。
- 统一接口：同类操作（如"所有工具执行"、"所有 provider 请求"）走同一个 trait / dispatch 模式。
- 能复用的要封装：重复出现的逻辑提取为公共函数/结构体，而非复制粘贴。

### 4. 函数单一职责

- 一个函数只做一件事。如果函数名需要 "and" 来描述，就该拆分。
- 函数参数不超过 5 个；超过的用结构体封装。
- 禁止 bool 参数——用枚举（Codex 同款规则），`foo(true, false)` 不可读。

### 5. 文件大小限制

- 单文件超过 **500 行（不含测试）** 时，必须考虑拆分。
- 超过 **800 行** 时，新功能必须放到新文件，不允许继续往大文件里加。
- 大文件本身是设计缺陷的信号——说明职责没有拆分清楚。

## Rust 编码规范（eos-core-rs）

### 命名

- Crate 名前缀 `eos-core-`。
- 类型名 PascalCase，函数/变量 snake_case，常量 UPPER_SNAKE。
- 文件名与模块名一致（snake_case）。

### 架构

```
eos-core-bin → app-server → runtime → {protocol, agent, context, model, tools, sandbox, store}
                                     ↘ orchestrator → {react_loop, agent_spawn}
                                     ↘ mcp → {client, server}
                                     ↘ browser
```

- `protocol`：纯 DTO，不含逻辑，所有 crate 可依赖。
- `runtime`：核心引擎，持有 ToolRuntime / TurnRunner / Engine trait 实现。
- `tools`：工具定义 + 执行器 + 审批策略。
- `sandbox`：沙箱策略 + 命令规则引擎（TOML 规则）。
- `model`：模型 provider 工厂（OpenAI 兼容 / Claude / Responses）。
- `bin`：二进制入口，只做组装（注入依赖），不含业务逻辑。

### 错误处理

- 用 `Result<T, E>` 显式传递错误，不 panic（除非 invariant 被破坏）。
- 错误类型实现 `Display`（或至少有 `message()` 方法）。
- `?` 操作符优先于 match。
- 禁止 `unwrap()` 在生产代码里（测试可以用）。

### 测试

- 单元测试跟代码在同一文件（`#[cfg(test)] mod tests`）。
- 集成测试在 `tests/` 目录。
- 测试不得依赖全局状态或执行顺序（并行安全）。
- 不为静态值写测试。
- 测试断言比较整个对象，而非逐字段比较。

### Tracing（日志）

- 关键路径加 `tracing::info!` / `warn!` / `debug!`。
- 日志走 stderr（父进程捕获到 `eos-core.log`），纯本地无网络。
- `EOS_LOG_LEVEL` 环境变量控制级别。

### 构建

- `cargo build --workspace` 编译。
- `cargo test -p <crate>` 跑特定 crate 的测试。
- `cargo clippy --workspace -- -D warnings` 零容忍 lint。
- 改了依赖后跑 `cargo build --workspace` 确认 Cargo.lock 同步。

## Go 编码规范（eos-cli / eos-app）

### 命名

- 包名小写单词，不加下划线/驼峰。
- 导出标识符 PascalCase，未导出 camelCase。

### 架构

- eos-cli：标准 Go 布局（`cmd/` 入口，`internal/` 私有逻辑，`pkg/` 可复用库）。
- eos-app：Wails v3 桌面端，`bridge_*.go` 是 Go↔前端的服务桥接层。
- Go 壳层不做业务裁决（如命令策略/审批），只做 DTO 透传和用户交互。
- 重逻辑在 Rust 内核（eos-core），Go 壳层通过 sidecar JSON-RPC 调用。

### 错误处理

- Go 侧 `error` 必须检查，禁止 `_ = err`。
- 用户可见错误用 i18n key（`internal/i18n/`），不硬编码字符串。

## 通用工作流

### 动手前先分析

- **修改代码前必须充分理解上下文**：读相关文件、理解调用链、确认影响范围。
- 不要只看一个文件就动手——追踪引用、理解上下游。
- 大改动（跨 crate / 多文件）先写计划，确认方案再实施。

### 提交规范

**禁止一句话提交。** 每个 commit 必须有结构化的标题 + 详细的 body。

#### 标题格式

```
type(scope): 中文摘要（不超过 50 字）
```

- **type**（必填）：
  - `feat` — 新功能
  - `fix` — bug 修复
  - `refactor` — 重构（不改行为）
  - `chore` — 构建/依赖/配置等杂项
  - `ci` — CI/CD 变更
  - `docs` — 文档变更
  - `test` — 测试补充/修复
- **scope**（必填）：受影响的模块，如 `sandbox` / `tools` / `mcp` / `orchestrator` / `runtime` / `protocol` / `model` / `cli` / `app` / 多个用 `/` 连接。
- **摘要**：用中文简明描述"做了什么"，不用"更新了""修改了"等模糊词。

#### Body 格式（必填）

Body 必须分节说明，每节用空行分隔：

```
<一句话背景/动机：为什么需要这个改动>

具体改动：
- 文件1：改了什么
- 文件2：改了什么
- ...

影响范围 / 设计决策（可选）：
- 为什么选这个方案
- 什么不在本次范围

验收：测试数 / 构建状态 / 其他验证方式
```

#### 示例

```
feat(sandbox): TOML 驱动的命令执行策略引擎

把 shell 命令裁决从硬编码升级为可声明、带自测的内置规则集。
规则用 TOML 描述，编译期 include_str! 内嵌进二进制。

具体改动：
- rules.rs: 规则引擎（CommandDecision 三态 + PatternToken 多选 + 自测校验）。
- default_rules.toml: 内置默认规则（108 条，覆盖 git/npm/cargo/go/docker/kubectl）。
- lib.rs: check_command 接入规则引擎，Forbid→deny，其余→allow。

不在范围：CEL 表达式求值（字段已预留，将来加 cel-interpreter 激活）。

验收: sandbox 91 测试全过，workspace build 零警告。
```

#### 禁止的提交方式

- ❌ `fix bug` — 没说修了什么 bug、在哪、怎么修的
- ❌ `update files` — 没说更新了什么
- ❌ `feat: add function` — 没说加了什么函数、为什么、在哪
- ❌ 只有标题没有 body
- ❌ body 只有一句话

### CI

- 三仓各有 `.github/workflows/ci.yml`：build + test + clippy/vet。
- PR 必须 CI 全绿才能合并。
- clippy/vet 零容忍（`-D warnings`）。

## 生产与 main 红线

### 1. 公开即不可逆

安装包、更新通道（后台上传 / `is_current`）、GitHub Releases 镜像、门户下载链接、发布说明、截图、营销文案，一旦发出就可能被缓存或镜像，无法保证全部撤回。错误会留下长期负面影响。

「之后再修」不是足够的补救。凡是会碰到公开面的操作，先停下来核对，再动手。

### 2. 正式版只从 `main` 发布

Stable / production 安装包、生产环境的 current、面向生产的下载产物，只能从 **`main` 上已经合并的提交**产出。

下列操作不得从 `dev`、`beta`、功能分支或尚未合并的提交发起：

- 打 Stable / production 包
- 把某个版本设为生产 current（`is_current`）
- 发布生产下载产物

Beta 通道包只从 `beta` 分支来。Dev 通道包只从 `dev` 来，并遵守既有测试策略。**正式 / Stable / production = 只能是 `main`。**

### 3. 只有验证过的代码才能进入 `main`

代码进入 `main` 之前，必须经过评审，并完成验证：相关测试、该改动适用的检查，以及需要时的负责人确认。

推测性或未测试的改动，不得为了发版 squash 进 `main`。长期分支三角保持为 `main` / `dev` / `beta`。

### 4. 不可逆操作先等人确认

与 AI Dev Methodology 的 Release-CICD 一致。上传生产版本、晋升 `is_current`、覆盖已有 Release、删除公开资产之前：

1. 向人类负责人报告 **拟发布版本** 和 **当前最新版本**。
2. 停下来，等对方对这一次操作给出明确确认。
3. 确认之前，不自行发起生产上传，也不执行上述写操作。

沉默、以往的笼统许可，或「看起来该发了」，都不是这一次的确认。

## 动手前检查清单

- [ ] 已读本文。目标分支正确：日常进 `dev`，Beta 包只从 `beta`，正式只从 `main`。
- [ ] 若目标是 `main`：评审已完成，相关测试与适用检查已通过，需要时已有负责人确认。未验证的工作不会为了发版被合进去。
- [ ] 若会碰到安装包、更新通道、`is_current`、GitHub Release、门户下载、发布说明、截图或对外文案：已写明拟发布版本和当前最新版本，并已得到负责人对这一次操作的明确确认。
- [ ] 确认之前：不上传生产包，不晋升 current，不覆盖 Release，不删除公开资产。
- [ ] 不从 `dev`、`beta`、功能分支或未合并提交发布 Stable / production。
- [ ] 写进产品的文案只说用户需要看到的内容；内部讨论不进入界面、提示或任何用户会读到的句子。

## 用户可见文案与内部讨论

面向用户的界面、错误提示、空状态、通知、日志展示、关于页、更新说明，以及任何会进入产品里的文案，只写用户需要看到的内容。

内部讨论不得以任何文案形式写进软件。这包括：团队讨论、方案权衡、agent 之间的对话或思考过程、实现备注、评审意见、排期和优先级争论、调试时留下的临时说明、未发布计划、内部代号，以及环境、凭证、服务器等运维细节。

落笔前先分清受众：

- **应当给用户看的**：帮助用户完成当前操作，用产品自己的口吻说明发生了什么、下一步可以做什么。用户不必理解内部实现。
- **内部细节，不允许给用户看的**：只留在 issue、PR、设计文档、给开发者看的注释，或默认不对用户展示的日志里。不要把这些句子搬进界面、对话框、按钮、提示条，或包装成“系统消息”让用户看到。

拿不准时，按“用户会不会在产品里读到这句”判断：会读到，就改写成用户语言；改不成，就不要出现在产品里。

## 禁止事项

1. **禁止引入兼容兜底/回退路径**来掩盖 bug。
2. **禁止把新功能加到已经过大的文件**（> 800 行）——开新文件。
3. **禁止在壳层做业务裁决**——裁决在 Rust 内核。
4. **禁止 `unwrap()` 在生产代码**——用 `?` 或显式 match。
5. **禁止 bool 参数**——用枚举。
6. **禁止复制粘贴重复逻辑**——封装复用。
7. **禁止不经测试就提交**——改动必须有对应测试覆盖。
8. **禁止引入规划文档描述但代码未实现的功能声明**——文档与代码必须一致。
9. **禁止一句话提交**——commit 必须有标准化标题（type(scope): 摘要）+ 详细 body（改了什么/为什么/怎么验收）。
10. **禁止把内部讨论写进用户可见文案**——见「用户可见文案与内部讨论」。
