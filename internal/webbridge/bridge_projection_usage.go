package webbridge

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// Usage 域 projection：成本明细、用量汇总、版本记录加载与计数展示 helper。

func (s *BridgeService) loadCostItems() []CostItemCard {
	items := s.costItemsReadOnly()
	return costItemCardsFromAdapterItems(items)
}

func costItemCardsFromAdapterItems(items []adapter.CostItem) []CostItemCard {
	out := make([]CostItemCard, 0, len(items))
	for _, item := range items {
		out = append(out, CostItemCard{
			Time:               item.Time.Format(time.RFC3339),
			Model:              item.Model,
			InputTokens:        item.InputTokens,
			ReplyTokens:        item.ReplyTokens,
			CachedInputTokens:  item.CachedInputTokens,
			TotalTokens:        item.TotalTokens,
			ContextInputTokens: item.ContextInputTokens,
			CostUSD:            item.CostUSD,
			UsageKnown:         item.UsageKnown,
			CostKnown:          item.CostKnown,
		})
	}
	slices.SortFunc(out, func(a, b CostItemCard) int {
		return strings.Compare(b.Time, a.Time)
	})
	return out
}

func (s *BridgeService) loadUsageSummary() UsageSummaryCard {
	summary := s.usageSummaryReadOnly()
	return usageSummaryCardFromAdapterSummary(summary)
}

func usageSummaryCardFromAdapterSummary(summary adapter.UsageSummary) UsageSummaryCard {
	return UsageSummaryCard{
		Rounds:             summary.Rounds,
		InputTokens:        summary.InputTokens,
		ReplyTokens:        summary.ReplyTokens,
		CachedInputTokens:  summary.CachedInputTokens,
		TotalTokens:        summary.TotalTokens,
		CostUSD:            summary.CostUSD,
		UnknownUsageRounds: summary.UnknownUsageRounds,
		UnknownCostRounds:  summary.UnknownCostRounds,
	}
}

func (s *BridgeService) loadVersions() []VersionCard {
	items := s.versionsReadOnly()
	out := make([]VersionCard, 0, len(items))
	for _, item := range items {
		out = append(out, VersionCard{
			ID:        item.ID,
			File:      item.File,
			CreatedAt: item.CreatedAt.Format(time.RFC3339),
			Summary:   item.Summary,
		})
	}
	slices.SortFunc(out, func(a, b VersionCard) int {
		return strings.Compare(b.CreatedAt, a.CreatedAt)
	})
	return out
}

// toCountLabel 把数量转成展示标签：<=0 一律显示 "0"，
// 避免负数或空值出现在用户可见的版本清理通知里。
func toCountLabel(v int) string {
	if v <= 0 {
		return "0"
	}
	return strconv.Itoa(v)
}
