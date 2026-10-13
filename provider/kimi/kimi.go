// Package kimi 实现 Kimi For Coding（Coding Plan）套餐额度用量查询。
//
// 用量查询: GET https://api.kimi.com/coding/v1/usages
//   鉴权: Authorization: Bearer <sk-kimi-...>
//
// 注意：Kimi 开放平台（api.moonshot.cn，按量付费）与 Kimi For Coding
// （api.kimi.com/coding，订阅套餐）是两套独立体系。Coding Plan 的
// sk-kimi- key 在开放平台接口上必然 401，须走本接口。
// 官方要求不要篡改 User-Agent（视为违规）。
package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/provider"
)

const (
	// usagesURL 查询 Coding Plan 套餐额度用量（用 sk-kimi- 订阅 Key 鉴权）。
	usagesURL = "https://api.kimi.com/coding/v1/usages"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// usagesResponse 是 /coding/v1/usages 端点的响应。
// 字段为多层嵌套结构，注意 detail 里的 limit/used/remaining 是字符串类型。
type usagesResponse struct {
	// Limits 是 5 小时滚动窗口配额列表（多数套餐只有 1 项）。
	Limits []limitWindow `json:"limits"`
	// Usages 是各周期用量比例（0-1）。
	Usages map[string]usageRatio `json:"usages"`
}

// limitWindow 是一个 5 小时窗口的配额明细。
type limitWindow struct {
	Window struct {
		Duration int64  `json:"duration"` // 窗口时长（分钟），300=5h
		TimeUnit string `json:"timeUnit"`
	} `json:"window"`
	Detail struct {
		Limit     string `json:"limit"`     // 总配额（字符串）
		Used      string `json:"used"`      // 已用（字符串）
		Remaining string `json:"remaining"` // 剩余（字符串）
		ResetTime string `json:"resetTime"` // 重置时间（RFC3339）
	} `json:"detail"`
}

// usageRatio 是某个周期的用量比例。
type usageRatio struct {
	UsedRatio float64 `json:"used_ratio"` // 已用比例 0-1
	ResetTime string  `json:"reset_time"` // 重置时间（RFC3339）
}

// Provider Kimi For Coding 套餐额度查询。
type Provider struct {
	apiKey string
}

// New 用配置构造一个 Kimi Provider。
func New(cfg config.ProvidersKimi) *Provider {
	return &Provider{apiKey: cfg.APIKey}
}

// Name 返回展示名称。
func (p *Provider) Name() string {
	return "Kimi"
}

// Query 查询套餐额度用量。
func (p *Provider) Query(ctx context.Context) (*provider.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	usage := &provider.Usage{
		ProviderName: p.Name(),
		Plan:         "Coding Plan",
		UpdatedAt:    time.Now(),
	}

	resp, err := p.getUsages(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询用量失败: %w", err)
	}
	p.buildUsage(usage, resp)

	return usage, nil
}

// getUsages 调用 /coding/v1/usages 查询套餐额度用量。
// 注意不设置 User-Agent（官方要求保持真实客户端标识，不要篡改）。
func (p *Provider) getUsages(ctx context.Context) (*usagesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usagesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result usagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body: %s)", err, truncate(string(body), 200))
	}
	return &result, nil
}

// buildUsage 将响应映射为标准化 Usage。
// 产出三条百分比条目：5h 窗口额度 / 月度总量 / 月度 Code。
func (p *Provider) buildUsage(usage *provider.Usage, r *usagesResponse) {
	// 1. 5h 窗口额度：优先用 limits[] 的精确数字，回退 usages.limit_5h 的比例
	if len(r.Limits) > 0 {
		d := r.Limits[0].Detail
		pct := calcPercent(d.Used, d.Limit)
		usage.Items = append(usage.Items, provider.UsageItem{
			Label:    "5h 窗口额度",
			Used:     pct,
			Limit:    100,
			Format:   provider.FormatPercent,
			ResetsAt: parseRFC3339(d.ResetTime),
		})
	} else if u, ok := r.Usages["limit_5h"]; ok {
		usage.Items = append(usage.Items, provider.UsageItem{
			Label:    "5h 窗口额度",
			Used:     u.UsedRatio * 100,
			Limit:    100,
			Format:   provider.FormatPercent,
			ResetsAt: parseRFC3339(u.ResetTime),
		})
	}

	// 2. 月度总量
	if u, ok := r.Usages["limit_month_total"]; ok {
		usage.Items = append(usage.Items, provider.UsageItem{
			Label:    "月度总量",
			Used:     u.UsedRatio * 100,
			Limit:    100,
			Format:   provider.FormatPercent,
			ResetsAt: parseRFC3339(u.ResetTime),
		})
	}

	// 3. 月度 Code
	if u, ok := r.Usages["limit_month_code"]; ok {
		usage.Items = append(usage.Items, provider.UsageItem{
			Label:    "月度 Code",
			Used:     u.UsedRatio * 100,
			Limit:    100,
			Format:   provider.FormatPercent,
			ResetsAt: parseRFC3339(u.ResetTime),
		})
	}
}

// calcPercent 计算已用百分比（used/limit*100），limit 无效时返回 0。
func calcPercent(used, limit string) float64 {
	l := parseFloat(limit)
	if l <= 0 {
		return 0
	}
	return parseFloat(used) / l * 100
}

// parseFloat 解析字符串形式的数字为 float64。
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// parseRFC3339 解析 RFC3339 时间字符串；失败返回 nil。
func parseRFC3339(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// truncate 截断字符串到 maxLen，超出加省略号。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
