package webbridge

import (
	"context"
	"fmt"
	"strings"
)

func requestFailureMessage(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "请求失败"
	}
	if strings.EqualFold(detail, context.DeadlineExceeded.Error()) {
		return "请求超时"
	}
	if strings.Contains(strings.ToLower(detail), "context deadline exceeded") {
		return "请求超时"
	}
	if strings.HasPrefix(detail, "请求失败") {
		return detail
	}
	return "请求失败：" + detail
}

func runtimeSummaryForMessage(message ChatMessage) string {
	events := nonNilSlice(message.RuntimeEvents)
	if len(events) == 0 {
		return ""
	}
	last := events[len(events)-1]
	lastTitle := strings.TrimSpace(last.Title)
	if lastTitle == "" {
		lastTitle = "运行步骤"
	}
	countText := fmt.Sprintf("%d 个步骤", len(events))
	switch strings.ToLower(strings.TrimSpace(message.State)) {
	case "completed":
		if strings.HasPrefix(lastTitle, "验收 ") {
			return "已完成 | " + lastTitle
		}
		return "已完成 | " + countText
	case "failed":
		return "失败 | 最近一步：" + lastTitle
	case "waiting":
		return "等待操作 | " + lastTitle
	default:
		return "正在运行 | " + lastTitle
	}
}
