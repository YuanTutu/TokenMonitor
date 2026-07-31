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
	// alerting 记录每个 provider+label 当前是否处于告警状态，用于用量告警去重。
	// 越过阈值时置 true 并提醒一次；回落到安全线后置 false，下次再越过会重新提醒。
	alerting map[string]bool
}

type itemState struct {
	lastUsed        float64
	hasUsedValue    bool
	resetNotified   bool
	lastResetsAt    *time.Time
}

// New 创建一个 Notifier。
func New() *Notifier {
	return &Notifier{
		lastSeen: make(map[string]itemState),
		alerting: make(map[string]bool),
	}
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

// ── 用量/余额不足告警 ──────────────────────────────────

// AlertEvent 表示一个用量/余额不足告警事件。
type AlertEvent struct {
	ProviderName string
	Label        string
	Message      string // 可读的告警详情，如 "已用 95%，超过阈值 90%"
}

// CheckAlerts 检查用量/余额是否越过告警阈值，返回本次新发生的告警事件。
// pctThreshold: 百分比类用量告警阈值（0-100，已用达到或超过即告警）。
// balThreshold: 低余额告警阈值（余额 CNY 低于即告警），仅对无 Limit 的计数项生效。
// 去重：同一项越过阈值只提醒一次；回落到安全线后清除标记，再次越过会重新提醒。
func (n *Notifier) CheckAlerts(usages []*provider.Usage, pctThreshold, balThreshold float64) []AlertEvent {
	// 阈值非法时不告警，避免误报
	if pctThreshold <= 0 || pctThreshold > 100 {
		pctThreshold = 90
	}
	if balThreshold < 0 {
		balThreshold = 10
	}

	var events []AlertEvent
	for _, u := range usages {
		for _, item := range u.Items {
			// FormatText（套餐状态/到期/账户状态）无用量数值，跳过
			if item.Format == provider.FormatText {
				continue
			}
			// provider 显式标记不参与告警的明细条目（如 DeepSeek 赠送/充值余额），跳过
			if item.NoAlert {
				continue
			}

			key := u.ProviderName + "|" + item.Label
			shouldAlert, msg := evaluateItem(item, pctThreshold, balThreshold)

			if shouldAlert {
				// 越过阈值：仅当之前未处于告警状态时才提醒（去重）
				if !n.alerting[key] {
					events = append(events, AlertEvent{
						ProviderName: u.ProviderName,
						Label:        item.Label,
						Message:      msg,
					})
				}
				n.alerting[key] = true
			} else {
				// 回落到安全线：清除告警标记，下次再越过会重新提醒
				n.alerting[key] = false
			}
		}
	}
	return events
}

// evaluateItem 判定单条 item 是否应告警，返回 (是否告警, 告警文案)。
// 去重状态由调用方 CheckAlerts 通过 Notifier.alerting 维护。
func evaluateItem(item provider.UsageItem, pctThreshold, balThreshold float64) (shouldAlert bool, msg string) {
	switch item.Format {
	case provider.FormatPercent:
		// 百分比类（GLM Token / Ark 额度）：已用百分比 >= 阈值
		if item.Used >= pctThreshold {
			return true, fmt.Sprintf("已用 %.0f%%，超过阈值 %.0f%%", item.Used, pctThreshold)
		}
	case provider.FormatCount:
		if item.Limit > 0 {
			// 计数类有上限（GLM MCP 次数）：换算百分比 >= 阈值
			pct := item.Used / item.Limit * 100
			if pct >= pctThreshold {
				return true, fmt.Sprintf("已用 %.0f/%.0f（%.0f%%），超过阈值 %.0f%%",
					item.Used, item.Limit, pct, pctThreshold)
			}
		} else {
			// 计数类无上限（DeepSeek 余额）：余额 < 阈值（越低越危险）
			if item.Used < balThreshold {
				unit := item.Unit
				if unit == "" {
					unit = "CNY"
				}
				return true, fmt.Sprintf("仅 %.0f %s，低于阈值 %.0f %s",
					item.Used, unit, balThreshold, unit)
			}
		}
	}
	return false, ""
}

// FormatAlerts 将告警事件格式化为通知文本。
func FormatAlerts(events []AlertEvent) []string {
	var msgs []string
	for _, e := range events {
		msgs = append(msgs, fmt.Sprintf("⚠️  %s - %s %s", e.ProviderName, e.Label, e.Message))
	}
	return msgs
}
