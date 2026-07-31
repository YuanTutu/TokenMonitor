package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/provider/ark"
	"tokenmonitor/provider/deepseek"
	"tokenmonitor/provider/glm"
)

// TestRenderWithRealData 用真实配置查询两个 provider 并验证 TUI 渲染不崩溃。
// 这不是纯单元测试（依赖网络和配置），用 `go test -run TestRenderWithRealData` 手动运行。
func TestRenderWithRealData(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("无配置文件，跳过:", err)
	}
	if !cfg.AnyEnabled() {
		t.Skip("未启用任何 provider，跳过")
	}

	m := New(cfg, "")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if cfg.Providers.GLM.Enabled {
		u, err := glm.New(cfg.Providers.GLM).Query(ctx)
		if err != nil {
			t.Logf("GLM 查询失败: %v", err)
		} else {
			m.usages = updateUsage(m.usages, u.ProviderName, u)
		}
	}
	if cfg.Providers.Ark.Enabled {
		u, err := ark.New(cfg.Providers.Ark).Query(ctx)
		if err != nil {
			t.Logf("Ark 查询失败: %v", err)
		} else {
			m.usages = updateUsage(m.usages, u.ProviderName, u)
		}
	}
	if cfg.Providers.DeepSeek.Enabled {
		u, err := deepseek.New(cfg.Providers.DeepSeek).Query(ctx)
		if err != nil {
			t.Logf("DeepSeek 查询失败: %v", err)
		} else {
			m.usages = updateUsage(m.usages, u.ProviderName, u)
		}
	}

	m.lastRefresh = time.Now()
	m.loading = false

	rendered := m.View()
	if rendered == "" {
		t.Fatal("渲染结果为空")
	}

	fmt.Println("========== TUI 渲染结果 ==========")
	fmt.Println(rendered)
	fmt.Println("==================================")

	// 验证关键内容存在
	for _, u := range m.usages {
		if u == nil {
			t.Error("存在 nil usage")
		}
	}
}
