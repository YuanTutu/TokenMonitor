// Package deepseek 实现 DeepSeek 账户余额查询。
//
// 余额查询: GET https://api.deepseek.com/user/balance
//   鉴权: Authorization: Bearer <API Key>
//
// DeepSeek 是纯 API 按量付费，无套餐额度。
package deepseek

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
	balanceURL = "https://api.deepseek.com/user/balance"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// balanceResponse 是 /user/balance 端点的响应。
type balanceResponse struct {
	IsAvailable  bool          `json:"is_available"`
	BalanceInfos []balanceInfo `json:"balance_infos"`
}

// balanceInfo 是单个币种的余额明细。
type balanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// Provider DeepSeek 账户余额查询。
type Provider struct {
	apiKey string
}

// New 用配置构造一个 DeepSeek Provider。
func New(cfg config.ProvidersDeepSeek) *Provider {
	return &Provider{apiKey: cfg.APIKey}
}

// Name 返回展示名称。
func (p *Provider) Name() string {
	return "DeepSeek"
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

// getBalance 调用 /user/balance 查询账户余额。
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
func (p *Provider) buildBalanceItems(usage *provider.Usage, r *balanceResponse) {
	statusText := "不可用"
	if r.IsAvailable {
		statusText = "可用"
	}
	usage.Items = append(usage.Items, provider.UsageItem{
		Label:  "账户状态",
		Format: provider.FormatText,
		Unit:   statusText,
	})

	for _, info := range r.BalanceInfos {
		currency := info.Currency
		if currency == "" {
			currency = "CNY"
		}
		usage.Items = append(usage.Items,
			provider.UsageItem{
				Label:  "总余额",
				Used:   parseFloat(info.TotalBalance),
				Format: provider.FormatCount,
				Unit:   currency,
				// 总余额是真正该监控低余额的条目
			},
			provider.UsageItem{
				Label:   "赠送余额",
				Used:    parseFloat(info.GrantedBalance),
				Format:  provider.FormatCount,
				Unit:    currency,
				NoAlert: true, // 赠送余额是总余额的构成明细，常为 0，单独告警会误报
			},
			provider.UsageItem{
				Label:   "充值余额",
				Used:    parseFloat(info.ToppedUpBalance),
				Format:  provider.FormatCount,
				Unit:    currency,
				NoAlert: true, // 充值余额是总余额的构成明细，单独告警会误报
			},
		)
	}
}

// parseFloat 解析字符串形式的金额为 float64。
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// truncate 截断字符串到 maxLen，超出加省略号。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
