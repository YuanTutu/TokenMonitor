// Package notify 负责套餐重置提醒。
//
// 策略：每次刷新后对比上次状态，当某项的 ResetsAt 已过且用量回落（Used 变小），
// 判定已重置，触发通知。每个周期只提醒一次。
package notify

import (
	"fmt"
	"time"

	"tokenmonitor/provider"
)

// Notifier 跟踪各套餐项的重置状态，检测重置事件。
type Notifier struct {
	// lastSeen 记录每个 provider+label 的上次 Used 值和是否已提醒。
	lastSeen map[string]itemState
}

type itemState struct {
	lastUsed        float64
	hasUsedValue    bool
	resetNotified   bool
	lastResetsAt    *time.Time
}

// New 创建一个 Notifier。
func New() *Notifier {
	return &Notifier{lastSeen: make(map[string]itemState)}
}

// ResetEvent 表示一个检测到的重置事件。
type ResetEvent struct {
	ProviderName string
	Label        string
}

// Check 分析本次查询结果，返回新发生的重置事件列表。
// events 为空表示无新重置。
func (n *Notifier) Check(usages []*provider.Usage) []ResetEvent {
	now := time.Now()
	var events []ResetEvent

	for _, u := range usages {
		for _, item := range u.Items {
			key := u.ProviderName + "|" + item.Label
			prev, existed := n.lastSeen[key]

			// 检测重置：ResetsAt 已过 且 上次有较高用量 且 本次用量明显回落
			if item.ResetsAt != nil && item.ResetsAt.Before(now) && existed && prev.hasUsedValue {
				dropped := prev.lastUsed - item.Used
				// 用量回落超过 10 个百分点（百分比）或 10%（计数）视为重置
				significantDrop := false
				if item.Format == provider.FormatPercent {
					significantDrop = dropped > 10
				} else if prev.lastUsed > 0 {
					significantDrop = dropped/prev.lastUsed > 0.1
				}

				if significantDrop && !prev.resetNotified {
					events = append(events, ResetEvent{
						ProviderName: u.ProviderName,
						Label:        item.Label,
					})
				}
			}

			// 重置提醒后，新周期开始时清除已提醒标记
			newState := itemState{
				lastUsed:     item.Used,
				hasUsedValue: true,
				lastResetsAt: item.ResetsAt,
			}
			// 如果 ResetsAt 变了（进入了新周期），清除已提醒标记
			if existed && prev.lastResetsAt != nil && item.ResetsAt != nil {
				if !item.ResetsAt.Equal(*prev.lastResetsAt) {
					newState.resetNotified = false
				} else {
					newState.resetNotified = prev.resetNotified || (len(events) > 0 && events[len(events)-1].Label == item.Label)
				}
			} else if !existed {
				newState.resetNotified = false
			} else {
				newState.resetNotified = prev.resetNotified
			}

			n.lastSeen[key] = newState
		}
	}

	return events
}

// FormatEvents 将重置事件格式化为通知文本。
func FormatEvents(events []ResetEvent) []string {
	var msgs []string
	for _, e := range events {
		msgs = append(msgs, fmt.Sprintf("🔔 %s - %s 已重置", e.ProviderName, e.Label))
	}
	return msgs
}
