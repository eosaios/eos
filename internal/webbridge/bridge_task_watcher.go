package webbridge

import (
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// taskWatcherInterval 任务页刷新粒度：AI 起了 dev server / 进程退出之后，
// 最多这个延迟内出现在任务页。用轮询而不是事件推送——task/list 本来就是
// 快照查询，为此给内核加一条「任务变更」广播事件不划算；指纹比对后只在
// 真变化时才发 shell-updated，空转成本可忽略。
const taskWatcherInterval = 2 * time.Second

// startTaskWatcher 周期拉内核 task/list，用与启动路径相同的
// loadTasksFromSnapshot 重建任务卡做指纹比对；有变化就发 shell-updated，
// 前端收到后重取 bootstrap——bootstrap 是唯一数据源（每次全量重算），
// watcher 不落任何状态，只负责把「后台任务有变化」这件事变成一次推送。
// 内核侧快照由 bg_task 工具持续登记（见 eos-core-tools/native/bg_tasks.rs）。
func (s *BridgeService) startTaskWatcher() {
	if s == nil || s.stopCh == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(taskWatcherInterval)
		defer ticker.Stop()

		var lastFingerprint string
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				items, err := s.listTasksRPC()
				if err != nil {
					// 内核未就绪或正在重启：静默跳过，下个周期再试。
					continue
				}
				snapshot := adapter.RuntimeSnapshot{
					Tasks: adapter.BackgroundTasksFromCoreAPI(items),
				}
				// 复用完整转换（含聊天流占位卡与排序），保证与启动路径产出一致。
				cards := s.loadTasksFromSnapshot(snapshot)
				fingerprint := taskCardsFingerprint(cards)
				if fingerprint == lastFingerprint {
					continue
				}
				lastFingerprint = fingerprint
				s.emitShellUpdated()
			}
		}
	}()
}

// taskCardsFingerprint 把会影响任务页展示的字段拼成短指纹：
// 状态、可杀性、日志尾部任何一处变化都触发一次前端推送。
func taskCardsFingerprint(cards []TaskCard) string {
	var b strings.Builder
	for _, card := range cards {
		b.WriteString(card.ID)
		b.WriteByte('|')
		b.WriteString(card.Status)
		b.WriteByte('|')
		if card.CanKill {
			b.WriteByte('k')
		}
		b.WriteByte('|')
		b.WriteString(card.Detail)
		b.WriteByte('|')
		b.WriteString(card.UpdatedAt)
		b.WriteByte('\n')
	}
	return b.String()
}
