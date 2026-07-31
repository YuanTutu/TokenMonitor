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
}

// Providers 汇总各 Provider 的凭据配置。
type Providers struct {
	GLM      ProvidersGLM      `json:"glm"`
	Ark      ProvidersArk      `json:"ark"`
	DeepSeek ProvidersDeepSeek `json:"deepseek"`
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

// Default 返回带默认值的初始配置。
func Default() *Config {
	return &Config{
		Providers: Providers{
			GLM:      ProvidersGLM{Enabled: false},
			Ark:      ProvidersArk{Enabled: false, Region: "cn-beijing"},
			DeepSeek: ProvidersDeepSeek{Enabled: false},
		},
		RefreshIntervalMinutes: 10,
		NotifyOnReset:          true,
	}
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
	return c.Providers.GLM.Enabled || c.Providers.Ark.Enabled || c.Providers.DeepSeek.Enabled
}
