//go:build integration

// 集成测试：依赖真实网络和 ~/.tokenusage/config.json 配置。
// 默认不参与 `go test`，需手动运行：
//   go test ./notify/... -tags=integration -run TestIntegrationAlerts -v
package notify

import (
	"context"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/provider"
	"tokenmonitor/provider/ark"
	"tokenmonitor/provider/deepseek"
	"tokenmonitor/provider/glm"
)

// TestIntegrationAlerts 用真实配置查询三个 provider，跑一遍告警检测，
// 打印各项用量和触发的告警，人工核对去重行为。
func TestIntegrationAlerts(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("无配置文件，跳过:", err)
	}
	if !cfg.AnyEnabled() {
		t.Skip("未启用任何 provider，跳过")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	var usages []*provider.Usage
	if cfg.Providers.GLM.Enabled {
		if u, err := glm.New(cfg.Providers.GLM).Query(ctx); err != nil {
			t.Logf("GLM 查询失败: %v", err)
		} else {
			usages = append(usages, u)
		}
	}
	if cfg.Providers.Ark.Enabled {
		if u, err := ark.New(cfg.Providers.Ark).Query(ctx); err != nil {
			t.Logf("Ark 查询失败: %v", err)
		} else {
			usages = append(usages, u)
		}
	}
	if cfg.Providers.DeepSeek.Enabled {
		if u, err := deepseek.New(cfg.Providers.DeepSeek).Query(ctx); err != nil {
			t.Logf("DeepSeek 查询失败: %v", err)
		} else {
			usages = append(usages, u)
		}
	}

	t.Logf("告警阈值: 用量 %.0f%% / 余额 %.0f CNY", cfg.AlertPercentThreshold, cfg.AlertBalanceThreshold)
	for _, u := range usages {
		t.Logf("[%s] 套餐: %s", u.ProviderName, u.Plan)
		for _, item := range u.Items {
			t.Logf("  - %s: format=%s used=%v limit=%v unit=%s",
				item.Label, item.Format, item.Used, item.Limit, item.Unit)
		}
	}

	n := New()
	events := n.CheckAlerts(usages, cfg.AlertPercentThreshold, cfg.AlertBalanceThreshold)
	t.Logf("第1次检测告警数: %d", len(events))
	for _, e := range events {
		t.Logf("  ⚠️ %s - %s %s", e.ProviderName, e.Label, e.Message)
	}

	// 第二次检测应去重（持续告警不重复）
	events2 := n.CheckAlerts(usages, cfg.AlertPercentThreshold, cfg.AlertBalanceThreshold)
	t.Logf("第2次检测告警数（应为0，去重）: %d", len(events2))
}
