// Package stepfun 实现阶跃星辰 StepFun 账户余额查询。
//
// 余额查询: GET https://api.stepfun.com/v1/accounts
//   鉴权: Authorization: Bearer <API Key>
//
// StepFun 是预付费按量计费，无套餐额度。响应为扁平结构。
package stepfun

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
	// accountURL 查询账户信息（用 API Key 鉴权）。
	accountURL = "https://api.stepfun.com/v1/accounts"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// accountResponse 是 /v1/accounts 端点的响应。
type accountResponse struct {
	Object              string  `json:"object"`                // 固定 "account"
	Type                string  `json:"type"`                  // "prepaid" 预付费 / "postpaid" 后付费
	Balance             float64 `json:"balance"`               // 当前可用余额
	TotalCashBalance    float64 `json:"total_cash_balance"`    // 总充值金额
	TotalVoucherBalance float64 `json:"total_voucher_balance"` // 总赠送金额
}

// Provider StepFun 账户余额查询。
type Provider struct {
	apiKey string
}

// New 用配置构造一个 StepFun Provider。
func New(cfg config.ProvidersStepFun) *Provider {
	return &Provider{apiKey: cfg.APIKey}
}

// Name 返回展示名称。
func (p *Provider) Name() string {
	return "StepFun"
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

	account, err := p.getAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询余额失败: %w", err)
	}
	p.buildBalanceItems(usage, account)

	return usage, nil
}

// getAccount 调用 /v1/accounts 查询账户信息。
func (p *Provider) getAccount(ctx context.Context) (*accountResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accountURL, nil)
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

	var result accountResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body: %s)", err, truncate(string(body), 200))
	}
	return &result, nil
}

// buildBalanceItems 将余额数据追加到 Usage。
// balance 是真正该监控低余额的条目；充值/赠送是构成明细，设 NoAlert。
func (p *Provider) buildBalanceItems(usage *provider.Usage, r *accountResponse) {
	// 账户类型展示
	planType := r.Type
	if planType == "" {
		planType = "prepaid"
	}

	usage.Items = append(usage.Items,
		provider.UsageItem{
			Label:  "账户类型",
			Format: provider.FormatText,
			Unit:   planType,
		},
		provider.UsageItem{
			Label:  "可用余额",
			Used:   r.Balance,
			Format: provider.FormatCount,
			Unit:   "CNY",
			// 可用余额参与低余额告警（Used < 阈值时触发）
		},
		provider.UsageItem{
			Label:   "充值余额",
			Used:    r.TotalCashBalance,
			Format:  provider.FormatCount,
			Unit:    "CNY",
			NoAlert: true, // 充值余额是可用余额的构成明细，单独告警会误报
		},
		provider.UsageItem{
			Label:   "赠送余额",
			Used:    r.TotalVoucherBalance,
			Format:  provider.FormatCount,
			Unit:    "CNY",
			NoAlert: true, // 赠送余额是可用余额的构成明细，单独告警会误报
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
