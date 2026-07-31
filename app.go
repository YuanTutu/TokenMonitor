// Package main 的 app.go 是 Wails 绑定层，把现有 provider/config/notify 包的能力
// 暴露给前端调用。逻辑层（provider/config/notify）零改动，本文件只做桥接。
package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"tokenmonitor/config"
	"tokenmonitor/notify"
	"tokenmonitor/provider"
	"tokenmonitor/provider/ark"
	"tokenmonitor/provider/deepseek"
	"tokenmonitor/provider/glm"
)

// App 是 Wails 绑定的应用对象，前端通过它调用后端能力。
type App struct {
	ctx      context.Context
	version  string
	cfg      *config.Config
	notifier *notify.Notifier
}

// NewApp 创建 App，version 为展示用版本号字符串。
func NewApp(version string) *App {
	cfg, err := config.Load()
	if err != nil {
		// 加载失败用默认配置，前端会提示重新配置
		cfg = config.Default()
	}
	// 设置 beeep 的 AppName。
	// Windows Toast 通知要求 AppID 是系统已注册的 AUMID，否则通知会被丢弃。
	// 这里用一个 Windows 内置的通用 AUMID，保证 Toast 能弹出。
	beeep.AppName = "TokenMonitor"
	return &App{
		version:  version,
		cfg:      cfg,
		notifier: notify.New(),
	}
}

// startup 在应用启动时由 Wails 调用，保存 context。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 每次启动重新加载配置，确保拿到最新值
	if cfg, err := config.Load(); err == nil {
		a.cfg = cfg
	}
}

// ── 配置相关 ──────────────────────────────────────────

// GetVersion 返回版本号字符串，供前端标题栏展示。
func (a *App) GetVersion() string {
	return a.version
}

// LoadConfig 加载并返回当前配置（凭据字段会脱敏，避免明文传到前端）。
func (a *App) LoadConfig() *ConfigView {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	a.cfg = cfg
	return toConfigView(cfg)
}

// SaveConfig 保存前端传来的配置。前端传的是脱敏视图，需要合并现有凭据。
func (a *App) SaveConfig(view *ConfigView) error {
	if view == nil {
		return fmt.Errorf("配置为空")
	}
	// 以现有配置为基底，只更新可改字段，保留凭据
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	applyConfigView(cfg, view)
	if err := config.Save(cfg); err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}

// SaveProviderKey 更新指定 provider 的 API Key 并保存。
// providerKey 取值：glm / ark / ark_secret / deepseek。
// cred 为空表示清空该凭据（不改变 enabled）。
func (a *App) SaveProviderKey(providerKey, cred string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch providerKey {
	case "glm":
		cfg.Providers.GLM.APIKey = cred
		if cred != "" {
			cfg.Providers.GLM.Enabled = true
		}
	case "ark":
		cfg.Providers.Ark.AccessKeyID = cred
		if cred != "" {
			cfg.Providers.Ark.Enabled = true
		}
	case "ark_secret":
		cfg.Providers.Ark.SecretAccessKey = cred
	case "deepseek":
		cfg.Providers.DeepSeek.APIKey = cred
		if cred != "" {
			cfg.Providers.DeepSeek.Enabled = true
		}
	default:
		return fmt.Errorf("未知 provider: %s", providerKey)
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}

// GetConfigPath 返回配置文件路径，供前端展示。
func (a *App) GetConfigPath() string {
	path, err := config.ConfigPath()
	if err != nil {
		return ""
	}
	return path
}

// OpenConfigExternal 用系统默认编辑器打开配置文件（备用入口）。
func (a *App) OpenConfigExternal() error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// NotifyOS 弹出操作系统原生通知（Windows 为 Toast）。
// title 为通知标题，body 为正文。前端在检测到告警/重置事件时调用。
// 注意：beeep 的 Notify 在 Windows 上走 Win32 通知 API，无需 AppID 注册。
func (a *App) NotifyOS(title, body string) error {
	// 截断过长的正文，避免通知卡顿
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return beeep.Notify(title, body, "")
}

// NotifyAlerts 批量弹出系统通知。notifs 为已格式化的通知文本列表
// （形如 "⚠️ 智谱 GLM - 周 Token 用量 已用 84%..."）。
// 按类型聚合：告警(⚠️)和重置(🔔)各弹一条汇总通知，避免刷屏。
func (a *App) NotifyAlerts(notifs []string) error {
	if len(notifs) == 0 {
		return nil
	}
	var alerts, resets []string
	for _, n := range notifs {
		if strings.HasPrefix(strings.TrimSpace(n), "⚠️") {
			alerts = append(alerts, n)
		} else {
			resets = append(resets, n)
		}
	}
	// 告警优先弹
	if len(alerts) > 0 {
		title := "用量/余额告警"
		body := strings.Join(alerts, "\n")
		if err := beeep.Notify(title, body, ""); err != nil {
			return err
		}
	}
	if len(resets) > 0 {
		title := "套餐额度重置"
		body := strings.Join(resets, "\n")
		if err := beeep.Notify(title, body, ""); err != nil {
			return err
		}
	}
	return nil
}

// ── 数据查询 ──────────────────────────────────────────

// RefreshResult 是单个 provider 的查询结果，传给前端渲染。
type RefreshResult struct {
	ProviderName string          `json:"providerName"`
	Plan         string          `json:"plan"`
	Items        []UsageItemView `json:"items"`
	Error        string          `json:"error,omitempty"` // 非空表示查询失败
	UpdatedAt    string          `json:"updatedAt"`       // ISO 时间
}

// UsageItemView 是 UsageItem 的前端友好视图。
// ResetsIn 是预算好的倒计时文本，前端直接展示，无需自己算时间。
type UsageItemView struct {
	Label    string  `json:"label"`
	Used     float64 `json:"used"`
	Limit    float64 `json:"limit"`
	Format   string  `json:"format"`  // "percent" | "count" | "text"
	Unit     string  `json:"unit"`
	Percent  float64 `json:"percent"`  // 统一的百分比（0-100），供前端画进度条；text 类型为 0
	ResetsIn string  `json:"resetsIn"` // 倒计时文本，如 "4h30m"；无则空
	NoAlert  bool    `json:"noAlert"`
}

// RefreshResponse 是 RefreshAll 的完整返回。
type RefreshResponse struct {
	Results       []RefreshResult `json:"results"`
	Notifications []string        `json:"notifications"`
	UpdatedAt     string          `json:"updatedAt"`
	Error         string          `json:"error,omitempty"`
}

// RefreshAll 并发查询所有 enabled provider，返回结果列表 + 通知事件。
func (a *App) RefreshAll() RefreshResponse {
	cfg, err := config.Load()
	if err != nil {
		return RefreshResponse{Error: fmt.Sprintf("加载配置失败: %v", err)}
	}
	a.cfg = cfg

	// 构造 provider 实例列表
	type pinfo struct {
		name string
		p    provider.Provider
	}
	var providers []pinfo
	if cfg.Providers.GLM.Enabled {
		providers = append(providers, pinfo{"智谱 GLM", glm.New(cfg.Providers.GLM)})
	}
	if cfg.Providers.Ark.Enabled {
		providers = append(providers, pinfo{"火山方舟", ark.New(cfg.Providers.Ark)})
	}
	if cfg.Providers.DeepSeek.Enabled {
		providers = append(providers, pinfo{"DeepSeek", deepseek.New(cfg.Providers.DeepSeek)})
	}

	if len(providers) == 0 {
		return RefreshResponse{Error: "未启用任何 provider"}
	}

	// 并发查询
	results := make([]RefreshResult, len(providers))
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	for i, pi := range providers {
		wg.Add(1)
		go func(idx int, name string, p provider.Provider) {
			defer wg.Done()
			usage, err := p.Query(ctx)
			if err != nil {
				results[idx] = RefreshResult{
					ProviderName: name,
					Error:        err.Error(),
				}
				return
			}
			results[idx] = toRefreshResult(usage)
		}(i, pi.name, pi.p)
	}
	wg.Wait()

	// 收集成功的 usage 给 notifier 检测通知
	var usages []*provider.Usage
	for _, r := range results {
		if r.Error == "" {
			usages = append(usages, toUsage(r))
		}
	}

	// 生成通知：重置提醒受开关控制，告警始终启用
	var notifs []string
	if cfg.NotifyOnReset {
		notifs = append(notifs, notify.FormatEvents(a.notifier.Check(usages))...)
	}
	notifs = append(notifs, notify.FormatAlerts(a.notifier.CheckAlerts(
		usages, cfg.AlertPercentThreshold, cfg.AlertBalanceThreshold))...)

	// 系统通知：notifier 的 Check/CheckAlerts 已做去重（越过提醒一次、回落清除），
	// 所以这里 notifs 里的都是"本次新产生"的。
	// 双通道通知：① 应用内模态对话框（MessageDialog，走 WebView2 最可靠）
	//            ② Windows Toast（beeep，后台轻提示，失败不影响主流程）
	if len(notifs) > 0 {
		notifsCopy := notifs // 避免闭包捕获迭代变量
		go func() {
			// Toast 通知（后台轻提示，失败静默）
			_ = a.NotifyAlerts(notifsCopy)
		}()
		// 应用内对话框（可靠，会弹到应用窗口前）。
		// 用 goroutine 避免阻塞刷新返回；MessageDialog 会抢焦点提示用户。
		go a.showAlertDialog(notifsCopy)
	}

	return RefreshResponse{
		Results:       results,
		Notifications: notifs,
		UpdatedAt:     time.Now().Format("15:04:05"),
	}
}

// showAlertDialog 弹出应用内模态对话框展示告警/重置通知。
// 使用 Wails MessageDialog，走 WebView2 渲染，不依赖 Windows 通知权限，最可靠。
func (a *App) showAlertDialog(notifs []string) {
	if a.ctx == nil {
		return
	}
	// 区分告警和重置，告警用 WarningDialog（黄色图标），重置用 InfoDialog
	var alerts, resets []string
	for _, n := range notifs {
		if strings.HasPrefix(strings.TrimSpace(n), "⚠️") {
			alerts = append(alerts, n)
		} else {
			resets = append(resets, n)
		}
	}
	if len(alerts) > 0 {
		body := strings.Join(alerts, "\n")
		_, _ = wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.WarningDialog,
			Title:   "TokenMonitor - 用量/余额告警",
			Message: body,
			Buttons: []string{"知道了"},
		})
	}
	if len(resets) > 0 {
		body := strings.Join(resets, "\n")
		_, _ = wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.InfoDialog,
			Title:   "TokenMonitor - 套餐额度重置",
			Message: body,
			Buttons: []string{"知道了"},
		})
	}
}

// ── 视图转换辅助 ──────────────────────────────────────

// ConfigView 是传给前端的配置视图，凭据脱敏。
type ConfigView struct {
	Providers              ProvidersView `json:"providers"`
	RefreshIntervalMinutes int           `json:"refreshIntervalMinutes"`
	NotifyOnReset          bool          `json:"notifyOnReset"`
	AlertPercentThreshold  float64       `json:"alertPercentThreshold"`
	AlertBalanceThreshold  float64       `json:"alertBalanceThreshold"`
}

// ProvidersView 是各 provider 的脱敏视图。
type ProvidersView struct {
	GLM      ProviderView `json:"glm"`
	Ark      ProviderView `json:"ark"`
	DeepSeek ProviderView `json:"deepseek"`
}

// ProviderView 是单个 provider 的脱敏视图。
type ProviderView struct {
	Enabled bool   `json:"enabled"`
	HasKey  bool   `json:"hasKey"`  // 是否已配置凭据
	KeyMask string `json:"keyMask"` // 脱敏后的凭据预览，如 "1e71****f529"
}

func toConfigView(cfg *config.Config) *ConfigView {
	return &ConfigView{
		Providers: ProvidersView{
			GLM:      toProviderView(cfg.Providers.GLM.Enabled, cfg.Providers.GLM.APIKey),
			Ark:      toProviderView(cfg.Providers.Ark.Enabled, cfg.Providers.Ark.AccessKeyID),
			DeepSeek: toProviderView(cfg.Providers.DeepSeek.Enabled, cfg.Providers.DeepSeek.APIKey),
		},
		RefreshIntervalMinutes: cfg.RefreshIntervalMinutes,
		NotifyOnReset:          cfg.NotifyOnReset,
		AlertPercentThreshold:  cfg.AlertPercentThreshold,
		AlertBalanceThreshold:  cfg.AlertBalanceThreshold,
	}
}

func toProviderView(enabled bool, cred string) ProviderView {
	return ProviderView{
		Enabled: enabled,
		HasKey:  cred != "",
		KeyMask: maskCred(cred),
	}
}

// maskCred 脱敏凭据：保留前4后4，中间用 **** 替换；过短则全掩。
func maskCred(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// applyConfigView 把前端视图的可改字段合并回配置（不动凭据）。
func applyConfigView(cfg *config.Config, view *ConfigView) {
	cfg.Providers.GLM.Enabled = view.Providers.GLM.Enabled
	cfg.Providers.Ark.Enabled = view.Providers.Ark.Enabled
	cfg.Providers.DeepSeek.Enabled = view.Providers.DeepSeek.Enabled
	cfg.RefreshIntervalMinutes = view.RefreshIntervalMinutes
	cfg.NotifyOnReset = view.NotifyOnReset
	cfg.AlertPercentThreshold = view.AlertPercentThreshold
	cfg.AlertBalanceThreshold = view.AlertBalanceThreshold
}

// toRefreshResult 把 provider.Usage 转成前端友好的 RefreshResult。
func toRefreshResult(u *provider.Usage) RefreshResult {
	r := RefreshResult{
		ProviderName: u.ProviderName,
		Plan:         u.Plan,
		UpdatedAt:    u.UpdatedAt.Format(time.RFC3339),
	}
	now := time.Now()
	for _, item := range u.Items {
		r.Items = append(r.Items, UsageItemView{
			Label:    item.Label,
			Used:     item.Used,
			Limit:    item.Limit,
			Format:   string(item.Format),
			Unit:     item.Unit,
			Percent:  computePercent(item),
			ResetsIn: item.ResetsIn(now),
			NoAlert:  item.NoAlert,
		})
	}
	return r
}

// computePercent 统一计算百分比（0-100），供前端画进度条。
func computePercent(item provider.UsageItem) float64 {
	switch item.Format {
	case provider.FormatPercent:
		return item.Used
	case provider.FormatCount:
		if item.Limit > 0 {
			return item.Used / item.Limit * 100
		}
	}
	return 0
}

// toUsage 把 RefreshResult 反向转回 provider.Usage，供 notifier 使用。
func toUsage(r RefreshResult) *provider.Usage {
	u := &provider.Usage{
		ProviderName: r.ProviderName,
		Plan:         r.Plan,
		UpdatedAt:    time.Now(),
	}
	for _, iv := range r.Items {
		item := provider.UsageItem{
			Label:   iv.Label,
			Used:    iv.Used,
			Limit:   iv.Limit,
			Format:  provider.Format(iv.Format),
			Unit:    iv.Unit,
			NoAlert: iv.NoAlert,
		}
		u.Items = append(u.Items, item)
	}
	return u
}
