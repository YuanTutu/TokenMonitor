package ark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/provider"
)

const (
	// arkOpenAPIHost 是火山引擎 OpenAPI 公共网关域名（管理面）。
	arkOpenAPIHost = "open.volcengineapi.com"
	// arkService 签名用的服务名。
	arkService = "ark"
	// requestTimeout 单次请求超时。
	requestTimeout = 15 * time.Second
)

// planResponse 是 GetPersonalPlan 接口的响应，返回套餐类型和有效期。
type planResponse struct {
	ResponseMetadata struct {
		Error *apiError `json:"Error"`
	} `json:"ResponseMetadata"`
	Result *planResult `json:"Result"`
}

type apiError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// planResult 是 GetPersonalPlan 返回的套餐信息。
type planResult struct {
	PlanType  string `json:"PlanType"`  // "Pro", "Lite"
	Status    string `json:"Status"`    // "Running", "Expired"
	StartTime string `json:"StartTime"` // ISO8601
	EndTime   string `json:"EndTime"`   // ISO8601 套餐到期时间
	AutoRenew bool   `json:"AutoRenew"`
}

// codingPlanUsageResponse 是 GetCodingPlanUsage 接口的响应。
// 返回 Coding Plan 三个维度的额度使用百分比和重置时间。
// 这是控制台内部接口，不在公开文档列表中。
type codingPlanUsageResponse struct {
	ResponseMetadata struct {
		Error *apiError `json:"Error"`
	} `json:"ResponseMetadata"`
	Result *codingPlanUsageResult `json:"Result"`
}

// codingPlanUsageResult 是 GetCodingPlanUsage 返回的额度用量。
type codingPlanUsageResult struct {
	// QuotaUsage 三个维度的额度使用情况：
	//   session = 5小时窗口限额
	//   weekly  = 周限额
	//   monthly = 月限额
	QuotaUsage []quotaUsage `json:"QuotaUsage"`
	Status     string       `json:"Status"`         // "Running"
	UpdateTimestamp int64   `json:"UpdateTimestamp"` // 数据更新时间(Unix秒)
}

// quotaUsage 是单个维度的额度使用情况。
type quotaUsage struct {
	Level          string  `json:"Level"`          // "session" | "weekly" | "monthly"
	Percent        float64 `json:"Percent"`        // 已用百分比 0-100
	ResetTimestamp int64   `json:"ResetTimestamp"` // 重置时间(Unix秒)
}

// Provider 火山方舟 Coding Plan 套餐用量查询。
type Provider struct {
	creds  Credentials
	region string
}

// New 用配置构造火山方舟 Provider。
func New(cfg config.ProvidersArk) *Provider {
	region := cfg.Region
	if region == "" {
		region = "cn-beijing"
	}
	return &Provider{
		creds:  Credentials{AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey},
		region: region,
	}
}

// Name 返回展示名称。
func (p *Provider) Name() string {
	return "火山方舟"
}

// Query 查询 Coding Plan 套餐额度用量。
// 分两步：1) GetPersonalPlan 查套餐类型 2) GetCodingPlanUsage 查额度百分比。
func (p *Provider) Query(ctx context.Context) (*provider.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 查套餐类型和有效期
	plan, err := p.getPersonalPlan(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询套餐失败: %w", err)
	}

	// 2. 查 Coding Plan 额度用量
	usage, err := p.getCodingPlanUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询额度失败: %w", err)
	}

	return p.buildUsage(plan, usage), nil
}

// getPersonalPlan 调用 GetPersonalPlan 查询 Coding Plan 套餐信息。
func (p *Provider) getPersonalPlan(ctx context.Context) (*planResult, error) {
	body := []byte(`{"Plan":"CodingPlan"}`)
	respBody, err := p.callAPI(ctx, "GetPersonalPlan", body)
	if err != nil {
		return nil, err
	}

	var resp planResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body: %s)", err, truncate(string(respBody), 300))
	}
	if resp.ResponseMetadata.Error != nil {
		return nil, fmt.Errorf("接口错误: code=%s message=%s",
			resp.ResponseMetadata.Error.Code, resp.ResponseMetadata.Error.Message)
	}
	return resp.Result, nil
}

// getCodingPlanUsage 调用 GetCodingPlanUsage 查询 Coding Plan 额度用量。
// 该接口返回 session(5h)/weekly/monthal 三个维度的已用百分比和重置时间。
func (p *Provider) getCodingPlanUsage(ctx context.Context) (*codingPlanUsageResult, error) {
	body := []byte(`{}`)
	respBody, err := p.callAPI(ctx, "GetCodingPlanUsage", body)
	if err != nil {
		return nil, err
	}

	var resp codingPlanUsageResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body: %s)", err, truncate(string(respBody), 300))
	}
	if resp.ResponseMetadata.Error != nil {
		return nil, fmt.Errorf("接口错误: code=%s message=%s",
			resp.ResponseMetadata.Error.Code, resp.ResponseMetadata.Error.Message)
	}
	return resp.Result, nil
}

// callAPI 发送签名请求到火山引擎 OpenAPI。
func (p *Provider) callAPI(ctx context.Context, action string, body []byte) ([]byte, error) {
	query := url.Values{}
	query.Set("Action", action)
	query.Set("Version", "2024-01-01")
	reqURL := fmt.Sprintf("https://%s/?%s", arkOpenAPIHost, query.Encode())

	signed, err := Sign(SignedRequest{
		Method:  http.MethodPost,
		URL:     reqURL,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	}, p.creds, arkService, p.region)
	if err != nil {
		return nil, fmt.Errorf("签名失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", signed.ContentType)
	req.Header.Set("X-Date", signed.XDate)
	req.Header.Set("X-Content-Sha256", signed.XContentSha256)
	req.Header.Set("Host", signed.Host)
	req.Header.Set("Authorization", signed.Authorization)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	return respBody, nil
}

// buildUsage 将套餐信息和额度用量映射为标准化 Usage。
func (p *Provider) buildUsage(plan *planResult, usage *codingPlanUsageResult) *provider.Usage {
	now := time.Now()
	u := &provider.Usage{
		ProviderName: p.Name(),
		Plan:         "Coding Plan",
		UpdatedAt:    now,
	}

	if plan != nil {
		if plan.PlanType != "" {
			u.Plan = "Coding Plan " + plan.PlanType
		}
	}

	// Coding Plan 额度维度标签映射
	levelLabels := map[string]string{
		"session": "5小时窗口额度",
		"weekly":  "周额度",
		"monthly": "月额度",
	}

	if usage != nil && len(usage.QuotaUsage) > 0 {
		for _, q := range usage.QuotaUsage {
			label := levelLabels[q.Level]
			if label == "" {
				label = q.Level + " 额度"
			}
			item := provider.UsageItem{
				Label:  label,
				Used:   q.Percent,
				Limit:  100,
				Format: provider.FormatPercent,
			}
			// ResetTimestamp 是 Unix 秒
			if q.ResetTimestamp > 0 {
				t := time.Unix(q.ResetTimestamp, 0)
				item.ResetsAt = &t
			}
			u.Items = append(u.Items, item)
		}
	} else {
		// 无法获取额度数据时的兜底
		u.Items = append(u.Items, provider.UsageItem{
			Label:  "套餐状态",
			Format: provider.FormatText,
			Unit:   "未知",
		})
	}

	// 套餐到期时间单独展示
	if plan != nil && plan.EndTime != "" {
		if t, err := time.Parse(time.RFC3339, plan.EndTime); err == nil {
			u.Items = append(u.Items, provider.UsageItem{
				Label:    "套餐到期",
				Format:   provider.FormatText,
				Unit:     t.Format("2006-01-02"),
				ResetsAt: &t,
			})
		}
	}

	return u
}

// truncate 截断字符串。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
