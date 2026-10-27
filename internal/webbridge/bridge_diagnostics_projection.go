package webbridge

import (
	"fmt"
	"path/filepath"
	"strings"
)

func (s *BridgeService) loadDiagnostics() DiagnosticsState {
	review := s.pendingReviewReadOnly()
	contextStats := s.contextStatsReadOnly()
	costSummary := s.costSummaryReadOnly()
	crashReport, _ := LoadPendingCrashReport()
	pendingCrashPath := ""
	pendingCrashPanic := ""
	if crashReport != nil {
		pendingCrashPath = crashReport.Path
		pendingCrashPanic = crashReport.Panic
	}
	return DiagnosticsState{
		LogFile:           s.logFile,
		LogTail:           tailLines(s.logFile, 18),
		LSPDiagnostics:    compactStrings(s.lspDiagnosticsReadOnly()),
		ContextPreview:    compactStrings(s.contextPreviewReadOnly()),
		ContextSummary:    fmt.Sprintf("%s · %d 条上下文消息", costSummary, contextStats.MessageCount),
		CostSummary:       costSummary,
		PendingReviewPath: review.Path,
		PendingReviewDiff: review.Diff,
		PendingCrashPath:  pendingCrashPath,
		PendingCrashPanic: pendingCrashPanic,
		ExportDirectory:   filepath.Dir(defaultExportBundlePath()),
	}
}

func (s *BridgeService) buildDiagnosticsReport() string {
	diagnostics := s.loadDiagnostics()
	lines := []string{
		"运行时诊断",
		"  日志文件: " + fallbackText(diagnostics.LogFile, "未配置"),
		"  成本摘要: " + fallbackText(diagnostics.CostSummary, "无"),
		"  上下文概览: " + fallbackText(diagnostics.ContextSummary, "无"),
		"",
		"日志尾部",
	}
	if len(diagnostics.LogTail) == 0 {
		lines = append(lines, "  暂无日志内容")
	} else {
		for _, line := range diagnostics.LogTail {
			lines = append(lines, "  "+line)
		}
	}
	if diagnostics.PendingReviewPath != "" {
		lines = append(lines, "", "待确认 Diff 来源", "  路径: "+diagnostics.PendingReviewPath)
	}
	if diagnostics.PendingCrashPath != "" {
		lines = append(lines, "", "待确认崩溃报告", "  文件: "+diagnostics.PendingCrashPath, "  Panic: "+diagnostics.PendingCrashPanic)
	}
	return strings.Join(lines, "\n")
}
