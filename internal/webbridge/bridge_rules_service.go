package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/i18n"
)

func defaultRulesTemplate() string {
	return `# Rules

本文件用于约束 AI 在本仓库中的行为与输出，优先用于“如何改这个仓库的代码”。

## 1. 目标
- 代码风格一致
- 修改可验证（能编译/测试）
- 安全合规（不泄露密钥，不做危险操作）

## 2. 代码约定
- 语言/框架：Go
- 命名、目录结构、导入顺序：遵循仓库现有风格
- 不新增注释（除非用户要求）

## 3. 工程命令
~~~bash
go test ./...
~~~

## 4. 修改策略
- 优先最小改动，先修根因再补体验
- 修改前先读现有代码与约定，避免引入新依赖
- 多文件改动需保持一致性

## 5. 交互与输出
- 输出中文
- 代码引用使用文件链接
- 需要时先解释“为什么这样改”
`
}

func globalRulesPath(lang string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New(i18n.T("error.rules.home_unavailable", lang))
	}
	return resolveInstructionsTarget(filepath.Join(home, ".eos")), nil
}

func workspaceRulesPath(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	return resolveInstructionsTarget(workspace)
}

// resolveInstructionsTarget 返回规则文档实际落点。内核（eos-core-rs
// instructions.rs）只发现 EOS.md / AGENTS.md，从不读 Rules.md——旧的
// Rules.md 是断链设计，用户在 GUI 写的规则从未进入模型上下文。统一改写
// 内核标准文件：目录里已有 EOS.md 用它；否则已有 AGENTS.md 就编辑它
// （避免新建 EOS.md 把 AGENTS.md 遮蔽成死文件）；两者都没有时新建 EOS.md
// （EOS 原生名优先，AGENTS.md 仅为互操作兜底名）。
func resolveInstructionsTarget(dir string) string {
	eos := filepath.Join(dir, "EOS.md")
	if _, err := os.Stat(eos); err == nil {
		return eos
	}
	agents := filepath.Join(dir, "AGENTS.md")
	if _, err := os.Stat(agents); err == nil {
		return agents
	}
	return eos
}

func readRuleDocument(id, scope, title, path, workspacePath, workspaceName string, ruleScope RuleDocumentScope) RuleDocument {
	doc := RuleDocument{
		ID:            id,
		Scope:         scope,
		Title:         title,
		Path:          strings.TrimSpace(path),
		Content:       defaultRulesTemplate(),
		Exists:        false,
		Active:        ruleScope == RuleDocumentScopeActiveOnly,
		WorkspacePath: strings.TrimSpace(workspacePath),
		WorkspaceName: strings.TrimSpace(workspaceName),
	}
	if doc.Path == "" {
		return doc
	}
	raw, err := os.ReadFile(doc.Path)
	if err != nil {
		return doc
	}
	doc.Exists = true
	text := strings.TrimSpace(string(raw))
	if text != "" {
		doc.Content = text
	}
	return doc
}

func resolveRuleWriteTarget(req RulesSaveRequest, lang string) (path string, scopeLabel string, detail string, err error) {
	switch strings.ToLower(strings.TrimSpace(req.Scope)) {
	case "global":
		path, err = globalRulesPath(lang)
		if err != nil {
			return "", "", "", err
		}
		return path, "全局规则", path, nil
	case "workspace":
		workspace := strings.TrimSpace(req.WorkspacePath)
		if workspace == "" {
			return "", "", "", errors.New(i18n.T("error.rules.workspace_path_missing", lang))
		}
		path = workspaceRulesPath(workspace)
		if path == "" {
			return "", "", "", errors.New(i18n.T("error.rules.workspace_path_required", lang))
		}
		return path, "工作区规则", workspace, nil
	default:
		return "", "", "", errors.New(i18n.T("error.rules.scope_unknown", lang))
	}
}
