//go:build preview

// 预览渲染：用真实配置查询并打印 TUI 主面板 + 配置页的完整画面。
// 运行: go test ./tui/... -tags=preview -run TestPreviewRender -v
package tui

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/notify"
	"tokenmonitor/provider/ark"
	"tokenmonitor/provider/deepseek"
	"tokenmonitor/provider/glm"
)

func TestPreviewRender(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("无配置:", err)
	}
	if !cfg.AnyEnabled() {
		t.Skip("未启用 provider")
	}

	// 模拟一个足够宽的终端
	m := New(cfg, "", "v0.2.0-dev build 20260731")
	m.width = 80
	m.height = 40
	m.loading = false
	m.lastRefresh = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	if cfg.Providers.GLM.Enabled {
		if u, err := glm.New(cfg.Providers.GLM).Query(ctx); err == nil {
			m.usages = updateUsage(m.usages, u.ProviderName, u)
		}
	}
	if cfg.Providers.Ark.Enabled {
		if u, err := ark.New(cfg.Providers.Ark).Query(ctx); err == nil {
			m.usages = updateUsage(m.usages, u.ProviderName, u)
		}
	}
	if cfg.Providers.DeepSeek.Enabled {
		if u, err := deepseek.New(cfg.Providers.DeepSeek).Query(ctx); err == nil {
			m.usages = updateUsage(m.usages, u.ProviderName, u)
		}
	}

	// 生成通知（与 tui.go 刷新完成后的逻辑一致：重置通知受开关控制，告警始终启用）
	var notifs []string
	if cfg.NotifyOnReset {
		notifs = append(notifs, notify.FormatEvents(m.notifier.Check(m.usages))...)
	}
	notifs = append(notifs, notify.FormatAlerts(m.notifier.CheckAlerts(m.usages,
		cfg.AlertPercentThreshold, cfg.AlertBalanceThreshold))...)
	m.notifications = notifs

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "══════════ 主面板 Dashboard ══════════")
	fmt.Fprintln(os.Stderr, m.viewDashboard())
	fmt.Fprintln(os.Stderr, "══════════ 配置页 Config ══════════")
	m.mode = viewConfig
	fmt.Fprintln(os.Stderr, m.viewConfig())
}
