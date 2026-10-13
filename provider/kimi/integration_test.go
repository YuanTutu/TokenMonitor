//go:build integration

package kimi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tokenmonitor/config"
)

// 集成测试：用真实 key 验证 Query 解析（手动运行：
//   go test ./provider/kimi/ -tags=integration -run TestVerifyQuery -v）
func TestVerifyQuery(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("无配置:", err)
	}
	if !cfg.Providers.Kimi.Enabled || cfg.Providers.Kimi.APIKey == "" {
		t.Skip("kimi 未配置")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	usage, err := New(cfg.Providers.Kimi).Query(ctx)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}
	fmt.Printf("[Kimi] Plan=%s\n", usage.Plan)
	for _, item := range usage.Items {
		reset := "-"
		if item.ResetsAt != nil {
			reset = item.ResetsAt.Format("2006-01-02 15:04")
		}
		fmt.Printf("  - %s: %.1f%% (重置 %s)\n", item.Label, item.Used, reset)
	}
	if len(usage.Items) < 3 {
		t.Errorf("期望 3 条条目，实际 %d", len(usage.Items))
	}
}
