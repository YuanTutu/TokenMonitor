package notify

import (
	"strings"
	"testing"

	"tokenmonitor/provider"
)

// ── 辅助构造 ──────────────────────────────────────────

// pctItem 构造一个百分比类 UsageItem（GLM Token / Ark 额度型）。
func pctItem(label string, used float64) provider.UsageItem {
	return provider.UsageItem{Label: label, Used: used, Limit: 100, Format: provider.FormatPercent}
}

// countItem 构造一个带 Limit 的计数类 UsageItem（GLM MCP 次数型）。
func countItem(label string, used, limit float64) provider.UsageItem {
	return provider.UsageItem{Label: label, Used: used, Limit: limit, Format: provider.FormatCount, Unit: "次"}
}

// balanceItem 构造一个无 Limit 的计数类 UsageItem（DeepSeek 余额型）。
func balanceItem(label string, used float64) provider.UsageItem {
	return provider.UsageItem{Label: label, Used: used, Limit: 0, Format: provider.FormatCount, Unit: "CNY"}
}

// textItem 构造一个文本类 UsageItem（套餐状态/到期/账户状态型）。
func textItem(label string) provider.UsageItem {
	return provider.UsageItem{Label: label, Format: provider.FormatText, Unit: "可用"}
}

func usagesWith(items ...provider.UsageItem) []*provider.Usage {
	return []*provider.Usage{{ProviderName: "测试", Items: items}}
}

// alertLabels 提取本次告警事件的 label 集合，便于断言。
func alertLabels(events []AlertEvent) []string {
	var labels []string
	for _, e := range events {
		labels = append(labels, e.Label)
	}
	return labels
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

// ── TestCheckAlertsPercent: 百分比类用量告警 ────────────

func TestCheckAlertsPercent(t *testing.T) {
	n := New()
	// 阈值 90：95% 触发，80% 不触发
	u := usagesWith(pctItem("5h Token 用量", 95), pctItem("周 Token 用量", 80))
	events := n.CheckAlerts(u, 90, 10)

	if len(events) != 1 {
		t.Fatalf("期望 1 条告警，实际 %d", len(events))
	}
	if events[0].Label != "5h Token 用量" {
		t.Errorf("告警 label = %q，期望 5h Token 用量", events[0].Label)
	}
	if !strings.Contains(events[0].Message, "95") || !strings.Contains(events[0].Message, "90") {
		t.Errorf("告警文案应含已用值和阈值: %q", events[0].Message)
	}
}

// TestCheckAlertsCountWithLimit: 计数类带 Limit（MCP 次数）告警
func TestCheckAlertsCountWithLimit(t *testing.T) {
	n := New()
	// 950/1000 = 95% >= 90% 触发；80/100 = 80% 不触发
	u := usagesWith(countItem("MCP 月度次数", 950, 1000), countItem("其它次数", 80, 100))
	events := n.CheckAlerts(u, 90, 10)

	if len(events) != 1 {
		t.Fatalf("期望 1 条告警，实际 %d", len(events))
	}
	if events[0].Label != "MCP 月度次数" {
		t.Errorf("告警 label = %q，期望 MCP 月度次数", events[0].Label)
	}
	// 文案应含 used/limit 形式
	if !strings.Contains(events[0].Message, "950") || !strings.Contains(events[0].Message, "1000") {
		t.Errorf("告警文案应含 used/limit: %q", events[0].Message)
	}
}

// ── TestCheckAlertsBalance: DeepSeek 低余额告警（无 Limit 特判）──

func TestCheckAlertsBalance(t *testing.T) {
	n := New()
	// 阈值 10 CNY：余额 8 触发，余额 50 不触发
	u := usagesWith(balanceItem("总余额", 8), balanceItem("充值余额", 50))
	events := n.CheckAlerts(u, 90, 10)

	if len(events) != 1 {
		t.Fatalf("期望 1 条告警，实际 %d", len(events))
	}
	if events[0].Label != "总余额" {
		t.Errorf("告警 label = %q，期望 总余额", events[0].Label)
	}
	// 文案应含 "低于阈值"
	if !strings.Contains(events[0].Message, "低于阈值") || !strings.Contains(events[0].Message, "8") {
		t.Errorf("告警文案应含余额值和'低于阈值': %q", events[0].Message)
	}
}

// TestCheckAlertsBalanceUnitFallback: 无 Unit 时默认 CNY
func TestCheckAlertsBalanceUnitFallback(t *testing.T) {
	n := New()
	item := provider.UsageItem{Label: "余额", Used: 5, Limit: 0, Format: provider.FormatCount, Unit: ""}
	u := []*provider.Usage{{ProviderName: "测试", Items: []provider.UsageItem{item}}}
	events := n.CheckAlerts(u, 90, 10)

	if len(events) != 1 {
		t.Fatalf("期望 1 条告警，实际 %d", len(events))
	}
	if !strings.Contains(events[0].Message, "CNY") {
		t.Errorf("Unit 为空时应回退为 CNY: %q", events[0].Message)
	}
}

// ── TestCheckAlertsDedup: 越过提醒一次，回落清除后可再次提醒 ──

func TestCheckAlertsDedup(t *testing.T) {
	n := New()

	// 第 1 次：95% 越过阈值 90%，应告警
	events := n.CheckAlerts(usagesWith(pctItem("用量", 95)), 90, 10)
	if len(events) != 1 {
		t.Fatalf("第1次越过应告警，实际 %d 条", len(events))
	}

	// 第 2 次：仍 95%（持续超阈值），不应重复告警
	events = n.CheckAlerts(usagesWith(pctItem("用量", 95)), 90, 10)
	if len(events) != 0 {
		t.Fatalf("持续超阈值不应重复告警，实际 %d 条", len(events))
	}

	// 第 3 次：回落到 50%（安全线），不告警，且应清除标记
	events = n.CheckAlerts(usagesWith(pctItem("用量", 50)), 90, 10)
	if len(events) != 0 {
		t.Fatalf("回落安全线不应告警，实际 %d 条", len(events))
	}

	// 第 4 次：再次越过 95%，应重新告警（证明标记已清除）
	events = n.CheckAlerts(usagesWith(pctItem("用量", 95)), 90, 10)
	if len(events) != 1 {
		t.Fatalf("回落后再次越过应重新告警，实际 %d 条", len(events))
	}
}

// ── TestCheckAlertsTextIgnored: FormatText 不告警 ──────

func TestCheckAlertsTextIgnored(t *testing.T) {
	n := New()
	// 套餐状态/到期/账户状态 都是 FormatText，即便数值字段瞎填也不应告警
	u := usagesWith(textItem("套餐状态"), textItem("套餐到期"), textItem("账户状态"))
	events := n.CheckAlerts(u, 90, 10)
	if len(events) != 0 {
		t.Fatalf("FormatText 不应告警，实际 %d 条: %v", len(events), events)
	}
}

// ── TestCheckAlertsThresholdBoundary: 阈值边界（=阈值触发）──

func TestCheckAlertsThresholdBoundary(t *testing.T) {
	n := New()
	// Used 恰好等于阈值 90，应触发（判定为 >=）
	events := n.CheckAlerts(usagesWith(pctItem("用量", 90)), 90, 10)
	if !containsLabel(alertLabels(events), "用量") {
		t.Errorf("Used 恰好等于阈值应告警，events=%v", events)
	}
}

// ── TestCheckAlertsMultiProvider: 多 provider 独立去重 ──

func TestCheckAlertsMultiProvider(t *testing.T) {
	n := New()
	u := []*provider.Usage{
		{ProviderName: "GLM", Items: []provider.UsageItem{pctItem("5h", 95)}},
		{ProviderName: "Ark", Items: []provider.UsageItem{pctItem("月额度", 96)}},
	}
	events := n.CheckAlerts(u, 90, 10)
	if len(events) != 2 {
		t.Fatalf("两个 provider 各越过阈值应各告警一次，实际 %d 条", len(events))
	}
	// 持续超阈值，不应重复
	events = n.CheckAlerts(u, 90, 10)
	if len(events) != 0 {
		t.Fatalf("多 provider 持续超阈值不应重复，实际 %d 条", len(events))
	}
}

// ── TestCheckAlertsInvalidThreshold: 阈值非法时回退默认 ──

func TestCheckAlertsInvalidThreshold(t *testing.T) {
	n := New()
	// 百分比阈值传 0（非法），应回退默认 90；95% 触发，50% 不触发
	u := usagesWith(pctItem("高用量", 95), pctItem("低用量", 50))
	events := n.CheckAlerts(u, 0, 10) // pct=0 非法 → 回退 90
	if len(events) != 1 {
		t.Errorf("非法百分比阈值应回退 90，期望 1 条告警，实际 %d", len(events))
	}
	if !containsLabel(alertLabels(events), "高用量") {
		t.Errorf("应告警 高用量，events=%v", events)
	}
}

// ── TestFormatAlerts: 文案格式 ─────────────────────────

func TestFormatAlerts(t *testing.T) {
	events := []AlertEvent{
		{ProviderName: "智谱 GLM", Label: "5h Token 用量", Message: "已用 95%，超过阈值 90%"},
		{ProviderName: "DeepSeek", Label: "总余额", Message: "仅 8 CNY，低于阈值 10 CNY"},
	}
	msgs := FormatAlerts(events)
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条文案，实际 %d", len(msgs))
	}
	if !strings.HasPrefix(msgs[0], "⚠️") {
		t.Errorf("告警文案应以 ⚠️ 开头: %q", msgs[0])
	}
	if !strings.Contains(msgs[0], "智谱 GLM") || !strings.Contains(msgs[0], "5h Token 用量") {
		t.Errorf("文案应含 provider 和 label: %q", msgs[0])
	}
}

func TestFormatAlertsEmpty(t *testing.T) {
	if msgs := FormatAlerts(nil); msgs != nil {
		t.Errorf("空事件应返回 nil，实际 %v", msgs)
	}
}

// ── TestCheckAlertsNoAlert: NoAlert 标记的明细条目不告警 ──

func TestCheckAlertsNoAlert(t *testing.T) {
	n := New()
	// 模拟 DeepSeek：总余额可告警(8<10触发)，赠送/充值余额 NoAlert 不告警
	total := balanceItem("总余额", 8)
	granted := balanceItem("赠送余额", 0)
	granted.NoAlert = true
	toppedUp := balanceItem("充值余额", 5)
	toppedUp.NoAlert = true
	u := usagesWith(total, granted, toppedUp)

	events := n.CheckAlerts(u, 90, 10)
	if len(events) != 1 {
		t.Fatalf("NoAlert 条目不应告警，期望 1 条(仅总余额)，实际 %d", len(events))
	}
	if events[0].Label != "总余额" {
		t.Errorf("应只告警总余额，实际 %q", events[0].Label)
	}
}

