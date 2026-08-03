// Package minimax 实现 MiniMax Token Plan 套餐额度用量查询。
//
// 用量查询: GET https://www.minimaxi.com/v1/token_plan/remains
//   鉴权: Authorization: Bearer <订阅 Key>
//
// MiniMax Token Plan 是订阅套餐型（5h 窗口 + 周窗口），与智谱 GLM / 火山方舟同类。
// 官方文档仅给出 curl 示例未公布完整响应结构，故本实现采用防御式解析：
// 尝试多种可能的字段命名，解析失败时降级展示原始数据摘要，不崩溃。
package minimax

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
	// remainsURL 查询 Token Plan 套餐额度用量（用订阅 Key 鉴权）。
	remainsURL = "https://www.minimaxi.com/v1/token_plan/remains"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// Provider MiniMax Token Plan 用量查询。
type Provider struct {
	apiKey string
}

// New 用配置构造一个 MiniMax Provider。
func New(cfg config.ProvidersMiniMax) *Provider {
	return &Provider{apiKey: cfg.APIKey}
}

// Name 返回展示名称。
func (p *Provider) Name() string {
	return "MiniMax"
}

// Query 查询 Token Plan 套餐额度用量。
func (p *Provider) Query(ctx context.Context) (*provider.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	usage := &provider.Usage{
		ProviderName: p.Name(),
		Plan:         "Token Plan",
		UpdatedAt:    time.Now(),
	}

	body, err := p.getRemains(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询用量失败: %w", err)
	}

	p.buildUsage(usage, body)
	return usage, nil
}

// getRemains 调用量量查询接口，返回原始响应体。
func (p *Provider) getRemains(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remainsURL, nil)
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
	return body, nil
}

// ── 防御式解析 ──────────────────────────────────────────
// 文档未公布完整响应结构，定义多种可能的结构体尝试解析。

// remainsResponse5h 是一种可能的响应结构：顶层含 5h 和 weekly 两个窗口对象。
type remainsResponse5h struct {
	FiveHour window `json:"five_hour"`
	Weekly   window `json:"weekly"`
}

// remainsResponseBase 是另一种可能：含 base_resp 错误信息 + data。
type remainsResponseBase struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Data json.RawMessage `json:"data"`
}

// window 是单个窗口的额度用量（字段名做多命名容错）。
type window struct {
	// 已用量（多种可能命名）
	Used        float64 `json:"used"`
	UsedTokens  float64 `json:"used_tokens"`
	UsedAmount  float64 `json:"used_amount"`
	CurrentUsed float64 `json:"current_used"`
	// 总额度
	Total       float64 `json:"total"`
	TotalTokens float64 `json:"total_tokens"`
	Limit       float64 `json:"limit"`
	Quota       float64 `json:"quota"`
	// 剩余
	Remaining    float64 `json:"remaining"`
	RemainTokens float64 `json:"remain_tokens"`
	// 已用百分比（接口可能直接返回）
	Percent       float64 `json:"percent"`
	UsagePercent  float64 `json:"usage_percent"`
	UsedPercent   float64 `json:"used_percent"`
	Percentage    float64 `json:"percentage"`
	// 重置时间（Unix 秒或毫秒，多种命名）
	ResetTimestamp int64 `json:"reset_timestamp"`
	ResetTime      int64 `json:"reset_time"`
	NextResetTime  int64 `json:"next_reset_time"`
	ExpireTime     int64 `json:"expire_time"`
}

// buildUsage 防御式解析响应并构造 Usage。
// 依次尝试多种可能的结构，成功则产出结构化条目；全部失败则降级展示原始摘要。
func (p *Provider) buildUsage(usage *provider.Usage, body []byte) {
	// 尝试 1：five_hour + weekly 双窗口结构
	if items, ok := tryParseDualWindow(body); ok {
		usage.Items = items
		return
	}

	// 尝试 2：base_resp.data 结构
	if items, ok := tryParseBaseResp(body); ok {
		usage.Items = items
		return
	}

	// 降级：无法识别结构，展示原始 JSON 摘要（截断），不崩溃
	summary := truncate(string(body), 120)
	usage.Items = append(usage.Items, provider.UsageItem{
		Label:  "原始响应",
		Format: provider.FormatText,
		Unit:   summary,
	})
}

// tryParseDualWindow 尝试解析 five_hour + weekly 双窗口结构。
func tryParseDualWindow(body []byte) ([]provider.UsageItem, bool) {
	var resp remainsResponse5h
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false
	}
	// 判断是否真的解析到了有效数据（两个字段不全为零值）
	if isZeroWindow(resp.FiveHour) && isZeroWindow(resp.Weekly) {
		return nil, false
	}

	var items []provider.UsageItem
	if !isZeroWindow(resp.FiveHour) {
		items = append(items, windowToItem("5h 窗口额度", resp.FiveHour))
	}
	if !isZeroWindow(resp.Weekly) {
		items = append(items, windowToItem("周窗口额度", resp.Weekly))
	}
	if len(items) == 0 {
		return nil, false
	}
	return items, true
}

// tryParseBaseResp 尝试解析 base_resp.data 结构。
func tryParseBaseResp(body []byte) ([]provider.UsageItem, bool) {
	var resp remainsResponseBase
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false
	}
	// 如果有错误状态码，不算成功
	if resp.BaseResp.StatusCode != 0 && resp.BaseResp.StatusCode != 1000 {
		return nil, false
	}
	// data 为空则无法继续
	if len(resp.Data) == 0 {
		return nil, false
	}
	// data 可能是双窗口结构
	if items, ok := tryParseDualWindow(resp.Data); ok {
		return items, true
	}
	return nil, false
}

// windowToItem 把窗口数据转为 UsageItem。
// 优先用接口直接返回的百分比；否则用 used/total 计算。
func windowToItem(label string, w window) provider.UsageItem {
	pct := firstNonZero(w.Percent, w.UsagePercent, w.UsedPercent, w.Percentage)
	if pct == 0 {
		used := firstNonZero(w.Used, w.UsedTokens, w.UsedAmount, w.CurrentUsed)
		total := firstNonZero(w.Total, w.TotalTokens, w.Limit, w.Quota)
		if total > 0 {
			pct = used / total * 100
		}
	}

	resetsAt := tsToTime(firstNonZeroInt64(w.ResetTimestamp, w.ResetTime, w.NextResetTime, w.ExpireTime))

	return provider.UsageItem{
		Label:    label,
		Used:     pct,
		Limit:    100,
		Format:   provider.FormatPercent,
		ResetsAt: resetsAt,
	}
}

// isZeroWindow 判断窗口是否全为零值（未解析到有效数据）。
func isZeroWindow(w window) bool {
	return w.Used == 0 && w.UsedTokens == 0 && w.UsedAmount == 0 && w.CurrentUsed == 0 &&
		w.Total == 0 && w.TotalTokens == 0 && w.Limit == 0 && w.Quota == 0 &&
		w.Percent == 0 && w.UsagePercent == 0 && w.UsedPercent == 0 && w.Percentage == 0
}

// firstNonZero 返回第一个非零值。
func firstNonZero(vals ...float64) float64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

// firstNonZeroInt64 返回第一个非零值。
func firstNonZeroInt64(vals ...int64) int64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

// tsToTime 将时间戳转为 *time.Time。
// 自动判断秒（< 1e12）或毫秒（>= 1e12）；0 返回 nil。
func tsToTime(ts int64) *time.Time {
	if ts <= 0 {
		return nil
	}
	var t time.Time
	if ts >= 1e12 {
		t = time.UnixMilli(ts)
	} else {
		t = time.Unix(ts, 0)
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
