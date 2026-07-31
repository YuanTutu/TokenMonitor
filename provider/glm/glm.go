// Package glm 实现智谱 GLM Coding Plan 套餐用量查询。
//
// 端点: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit
// 鉴权: Authorization 头直接放 API Key 裸值（不带 "Bearer " 前缀）。
// 该端点非官方公开文档化，由控制台逆向得到，返回套餐维度的配额用量。
package glm

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
	// quotaLimitURL 查询 Coding Plan 套餐配额用量。
	quotaLimitURL = "https://open.bigmodel.cn/api/monitor/usage/quota/limit"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// limitType 是智谱返回的配额维度类型。
type limitType string

const (
	limitTokens limitType = "TOKENS_LIMIT" // Token 限额（5h 窗口 / 周窗口）
	limitTime   limitType = "TIME_LIMIT"   // MCP 工具调用次数（月度）
)

// quotaLimitResponse 是 /quota/limit 端点的响应。
type quotaLimitResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data quotaLimitData  `json:"data"`
}

// quotaLimitData 是响应的 data 字段。
type quotaLimitData struct {
	Level  string      `json:"level"` // 套餐档位，如 "pro"
	Limits []limitItem `json:"limits"`
}

// limitItem 是单个配额维度。
type limitItem struct {
	Type          limitType `json:"type"`
	Unit          int64     `json:"unit"`          // 窗口单位类型（3=小时,5=月等枚举）
	Number        int64     `json:"number"`        // 窗口数值（如 5 表示 5 小时窗口）
	Usage         float64   `json:"usage"`         // TIME_LIMIT: 月度总额度
	CurrentValue  float64   `json:"currentValue"`  // TIME_LIMIT: 已用次数
	Remaining     float64   `json:"remaining"`     // TIME_LIMIT: 剩余次数
	Percentage    float64   `json:"percentage"`    // 已用百分比 (0-100)
	NextResetTime int64     `json:"nextResetTime"` // 重置时间（Unix 毫秒）
}

// Provider 智谱 GLM 套餐用量查询。
type Provider struct {
	apiKey string
}

// New 用配置构造一个 GLM Provider。
func New(cfg config.ProvidersGLM) *Provider {
	return &Provider{apiKey: cfg.APIKey}
}

// Name 返回展示名称。
func (p *Provider) Name() string {
	return "智谱 GLM"
}

// Query 查询当前套餐用量。
func (p *Provider) Query(ctx context.Context) (*provider.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, quotaLimitURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	// 智谱 monitor 系列接口鉴权用裸 API Key，不带 Bearer 前缀。
	req.Header.Set("Authorization", p.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

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

	var result quotaLimitResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body: %s)", err, truncate(string(body), 200))
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("接口返回错误 code=%d msg=%s", result.Code, result.Msg)
	}

	return p.buildUsage(&result.Data), nil
}

// buildUsage 将智谱响应映射为标准化 Usage。
func (p *Provider) buildUsage(data *quotaLimitData) *provider.Usage {
	now := time.Now()
	usage := &provider.Usage{
		ProviderName: p.Name(),
		Plan:         data.Level,
		UpdatedAt:    now,
	}

	// 智谱返回的 limits 含多条 TOKENS_LIMIT（5h/周窗口）和一条 TIME_LIMIT（月度 MCP）。
	// nextResetTime 是接口直接返回的重置时间（Unix 毫秒），比自行推算准确。
	tokenSeen := 0
	for _, item := range data.Limits {
		resetsAt := msToTime(item.NextResetTime)
		switch item.Type {
		case limitTokens:
			tokenSeen++
			label := "Token 用量"
			if tokenSeen == 1 {
				label = "5h Token 用量"
			} else {
				label = "周 Token 用量"
			}
			usage.Items = append(usage.Items, provider.UsageItem{
				Label:    label,
				Used:     item.Percentage,
				Limit:    100,
				Format:   provider.FormatPercent,
				ResetsAt: resetsAt,
			})
		case limitTime:
			usage.Items = append(usage.Items, provider.UsageItem{
				Label:    "MCP 月度次数",
				Used:     item.CurrentValue,
				Limit:    item.Usage,
				Format:   provider.FormatCount,
				Unit:     "次",
				ResetsAt: resetsAt,
			})
		}
	}

	return usage
}

// msToTime 将 Unix 毫秒时间戳转为 *time.Time；0 或负值返回 nil。
func msToTime(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms)
	return &t
}

// truncate 截断字符串到 maxLen，超出加省略号。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
