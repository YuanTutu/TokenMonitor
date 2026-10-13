// Package config 管理各 Provider 的凭据和程序设置。
// 配置以明文 JSON 存储在用户目录 ~/.tokenusage/config.json。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 是整个程序的配置根。
type Config struct {
	Providers              Providers `json:"providers"`
	RefreshIntervalMinutes int       `json:"refreshIntervalMinutes"`
	NotifyOnReset          bool      `json:"notifyOnReset"`
	// NotifyOnAlert 控制用量/余额告警是否发送弹窗通知。关闭后仍生成告警并展示在
	// 界面通知区，但不弹应用内对话框和系统通知。
	// 用指针区分"旧配置缺失字段（nil，补默认 true）"和"用户主动关闭（false）"。
	NotifyOnAlert *bool `json:"notifyOnAlert"`
	// AlertPercentThreshold 是百分比类用量（Token/额度）的告警阈值，已用百分比达到或超过该值时告警。
	AlertPercentThreshold float64 `json:"alertPercentThreshold"`
	// AlertBalanceThreshold 是 DeepSeek 这类纯余额账户的低余额告警阈值，余额（CNY）低于该值时告警。
	AlertBalanceThreshold float64 `json:"alertBalanceThreshold"`
	// Theme 是界面主题："glass"（液态玻璃，默认）或 "terminal"（终端美学）。
	Theme string `json:"theme"`
	// ProviderOrder 是 provider 卡片的展示顺序（key 列表，如 ["glm","kimi",...]）。
	// 缺失或含未知 key 时在 Load 中补齐为默认顺序。
	ProviderOrder []string `json:"providerOrder"`
}

// Providers 汇总各 Provider 的凭据配置。
type Providers struct {
	GLM      ProvidersGLM      `json:"glm"`
	Ark      ProvidersArk      `json:"ark"`
	DeepSeek ProvidersDeepSeek `json:"deepseek"`
	Kimi     ProvidersKimi     `json:"kimi"`
	StepFun  ProvidersStepFun  `json:"stepfun"`
	MiniMax  ProvidersMiniMax  `json:"minimax"`
}

// ProvidersGLM 是智谱 GLM 的凭据。
type ProvidersGLM struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
}

// ProvidersArk 是火山方舟的凭据。
type ProvidersArk struct {
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	Region          string `json:"region"` // 默认 cn-beijing
}

// ProvidersDeepSeek 是 DeepSeek 的凭据。
// APIKey 用于查询余额（GET /user/balance）、模型列表（GET /models）
// 和单次调用用量探测（POST /chat/completions），均使用 Bearer 鉴权。
type ProvidersDeepSeek struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
}

// ProvidersKimi 是月之暗面 Kimi (Moonshot AI) 的凭据。
// APIKey 用于查询余额（GET /v1/users/me/balance），Bearer 鉴权。
type ProvidersKimi struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
}

// ProvidersStepFun 是阶跃星辰 StepFun 的凭据。
// APIKey 用于查询账户余额（GET /v1/accounts），Bearer 鉴权。
type ProvidersStepFun struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
}

// ProvidersMiniMax 是 MiniMax 的凭据。
// APIKey 是 Token Plan 订阅 Key，用于查询套餐额度用量（GET /v1/token_plan/remains），Bearer 鉴权。
type ProvidersMiniMax struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
}

// Default 返回带默认值的初始配置。
func Default() *Config {
	return &Config{
		Providers: Providers{
			GLM:      ProvidersGLM{Enabled: false},
			Ark:      ProvidersArk{Enabled: false, Region: "cn-beijing"},
			DeepSeek: ProvidersDeepSeek{Enabled: false},
			Kimi:     ProvidersKimi{Enabled: false},
			StepFun:  ProvidersStepFun{Enabled: false},
			MiniMax:  ProvidersMiniMax{Enabled: false},
		},
		RefreshIntervalMinutes: 10,
		NotifyOnReset:          true,
		NotifyOnAlert:          boolPtr(true),
		AlertPercentThreshold:  90,
		AlertBalanceThreshold:  10,
		Theme:                  "glass",
		ProviderOrder:          DefaultProviderOrder(),
	}
}

// DefaultProviderOrder 返回默认的 provider 展示顺序。
func DefaultProviderOrder() []string {
	return []string{"glm", "ark", "deepseek", "kimi", "stepfun", "minimax"}
}

// boolPtr 返回 bool 的指针，用于区分 JSON 缺失字段与显式 false。
func boolPtr(b bool) *bool {
	return &b
}

// normalizeOrder 规范化 provider 排序：保留用户顺序中的合法 key（去重），
// 缺失的 provider key 补到末尾（按默认顺序）。
func normalizeOrder(order []string) []string {
	valid := make(map[string]bool)
	for _, k := range DefaultProviderOrder() {
		valid[k] = true
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(valid))
	for _, k := range order {
		if valid[k] && !seen[k] {
			seen[k] = true
			result = append(result, k)
		}
	}
	for _, k := range DefaultProviderOrder() {
		if !seen[k] {
			result = append(result, k)
		}
	}
	return result
}

// NotifyAlertEnabled 返回告警弹窗是否开启（nil 视为开启）。
func (c *Config) NotifyAlertEnabled() bool {
	return c.NotifyOnAlert == nil || *c.NotifyOnAlert
}

// configPath 返回配置文件的绝对路径: ~/.tokenusage/config.json。
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户目录: %w", err)
	}
	return filepath.Join(home, ".tokenusage", "config.json"), nil
}

// Load 读取配置文件。文件不存在时返回默认配置（不视为错误）。
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c := Default()
			return c, nil
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 填充默认值
	if c.RefreshIntervalMinutes <= 0 {
		c.RefreshIntervalMinutes = 10
	}
	if c.Providers.Ark.Region == "" {
		c.Providers.Ark.Region = "cn-beijing"
	}
	// 告警阈值兜底：旧配置文件没有这两个字段时会反序列化为 0，需补默认值。
	// 百分比阈值合法范围 (0,100]，缺失或非法补 90。
	// 余额阈值缺失时补 10；用户可主动设为 0 表示关闭余额告警（通过 TUI 按键可调回）。
	if c.AlertPercentThreshold <= 0 || c.AlertPercentThreshold > 100 {
		c.AlertPercentThreshold = 90
	}
	// 注意：余额阈值无法用 "<=0" 区分「缺失」与「主动设0」，故只在首次升级时
	// 通过 Save 把默认值落盘；此处对缺失（恰好为 0 且百分比也是默认补的 90）补 10。
	if c.AlertBalanceThreshold == 0 && c.AlertPercentThreshold == 90 {
		// 仅当确实是从旧配置升级（两字段都没写过）时补；该启发对正常使用无影响。
		c.AlertBalanceThreshold = 10
	}
	if c.AlertBalanceThreshold < 0 {
		c.AlertBalanceThreshold = 0
	}
	// 主题兜底：缺失或非法值补 "glass"
	if c.Theme != "glass" && c.Theme != "terminal" {
		c.Theme = "glass"
	}
	// 排序兜底：保留用户顺序中的合法 key，去除重复/未知，缺失的补到末尾
	c.ProviderOrder = normalizeOrder(c.ProviderOrder)

	return &c, nil
}

// Save 将配置写入文件。目录不存在时自动创建。
func Save(c *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return nil
}

// ConfigPath 返回配置文件路径，供外部展示。
func ConfigPath() (string, error) {
	return configPath()
}

// AnyEnabled 返回是否有任意 Provider 被启用。
func (c *Config) AnyEnabled() bool {
	return c.Providers.GLM.Enabled || c.Providers.Ark.Enabled || c.Providers.DeepSeek.Enabled ||
		c.Providers.Kimi.Enabled || c.Providers.StepFun.Enabled || c.Providers.MiniMax.Enabled
}
