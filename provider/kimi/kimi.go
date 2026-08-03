// Package kimi 实现月之暗面 Kimi (Moonshot AI) 账户余额查询。
//
// 余额查询: GET https://api.moonshot.cn/v1/users/me/balance
//   鉴权: Authorization: Bearer <API Key>
//
// Kimi 是纯 API 按量付费，无套餐额度。响应结构类似 DeepSeek 但字段为数值型。
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
	// balanceURL 查询账户余额（用 API Key 鉴权）。
	balanceURL = "https://api.moonshot.cn/v1/users/me/balance"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// balanceResponse 是 /v1/users/me/balance 端点的响应。
type balanceResponse struct {
	Code int          `json:"code"`
	Data balanceData  `json:"data"`
}

// balanceData 是响应的 data 字段，含三类余额。
type balanceData struct {
	AvailableBalance float64 `json:"available_balance"` // 可用余额（现金+代金券）
	VoucherBalance   float64 `json:"voucher_balance"`   // 代金券余额
	CashBalance      float64 `json:"cash_balance"`      // 现金余额（可为负=欠费）
}

// Provider Kimi 账户余额查询。
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

// Query 查询账户余额。
func (p *Provider) Query(ctx context.Context) (*provider.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	usage := &provider.Usage{
		ProviderName: p.Name(),
		Plan:         "API 按量付费",
		UpdatedAt:    time.Now(),
	}

	balance, err := p.getBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询余额失败: %w", err)
	}
	p.buildBalanceItems(usage, balance)

	return usage, nil
}

// getBalance 调用余额查询接口。
func (p *Provider) getBalance(ctx context.Context) (*balanceResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, balanceURL, nil)
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

	var result balanceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body: %s)", err, truncate(string(body), 200))
	}
	return &result, nil
}

// buildBalanceItems 将余额数据追加到 Usage。
// available_balance 是真正该监控低余额的条目；代金券/现金是构成明细，设 NoAlert。
func (p *Provider) buildBalanceItems(usage *provider.Usage, r *balanceResponse) {
	usage.Items = append(usage.Items,
		provider.UsageItem{
			Label:  "可用余额",
			Used:   r.Data.AvailableBalance,
			Format: provider.FormatCount,
			Unit:   "CNY",
			// 可用余额参与低余额告警（Used < 阈值时触发）
		},
		provider.UsageItem{
			Label:   "代金券余额",
			Used:    r.Data.VoucherBalance,
			Format:  provider.FormatCount,
			Unit:    "CNY",
			NoAlert: true, // 代金券是可用余额的构成明细，单独告警会误报
		},
		provider.UsageItem{
			Label:   "现金余额",
			Used:    r.Data.CashBalance,
			Format:  provider.FormatCount,
			Unit:    "CNY",
			NoAlert: true, // 现金是可用余额的构成明细，单独告警会误报
		},
	)
}

// truncate 截断字符串到 maxLen，超出加省略号。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
