// Package tui 实现命令行交互界面。
//
// 基于 BubbleTea 框架，以卡片式面板展示各套餐余额、进度条、重置倒计时，
// 支持手动刷新(R)、进入配置页(C)、退出(Q)。
// 配置页可在 TUI 内切换 provider 开关、调整刷新间隔，
// 或按 E 用外部编辑器编辑 config.json 修改 API Key。
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"tokenmonitor/config"
	"tokenmonitor/notify"
	"tokenmonitor/provider"
	"tokenmonitor/provider/ark"
	"tokenmonitor/provider/deepseek"
	"tokenmonitor/provider/glm"
)

// ── 颜色 ──────────────────────────────────────────────

var (
	colorTitle   = lipgloss.Color("#7DD3FC")
	colorHeader  = lipgloss.Color("#94A3B8")
	colorProvider = lipgloss.Color("#A5B4FC")
	colorGreen   = lipgloss.Color("#4ADE80")
	colorYellow  = lipgloss.Color("#FBBF24")
	colorRed     = lipgloss.Color("#F87171")
	colorDim     = lipgloss.Color("#64748B")
	colorNotify  = lipgloss.Color("#FCD34D")

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorTitle)
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colorHeader)
	styleHelp     = lipgloss.NewStyle().Foreground(colorDim)
	styleLoading  = lipgloss.NewStyle().Foreground(colorDim)
	styleError    = lipgloss.NewStyle().Foreground(colorRed)
	styleNotify   = lipgloss.NewStyle().Foreground(colorNotify).Bold(true)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorTitle)
	styleNormal   = lipgloss.NewStyle()
)

// ── 视图模式 ──────────────────────────────────────────

type viewMode int

const (
	viewDashboard viewMode = iota // 主面板
	viewConfig                    // 配置页
)

// ── Model ─────────────────────────────────────────────

// Model 是 TUI 的状态。
type Model struct {
	cfg        *config.Config
	configPath string
	version    string // 展示用版本号，如 "v0.1.0 build 20260731"

	// 数据
	usages  []*provider.Usage
	errors  map[string]string // providerName -> error
	pending map[string]bool   // 本次刷新待返回的 provider

	// 状态
	mode      viewMode
	loading   bool
	spinner   spinner.Model
	notifier  *notify.Notifier
	notifications []string
	lastRefresh   time.Time
	width      int
	height     int
	quit       bool

	// 配置页状态
	cfgCursor    int    // 选中项索引
	cfgSavedHint string // 保存提示
}

// New 创建 TUI Model。version 为展示用版本号字符串。
func New(cfg *config.Config, configPath string, version string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		cfg:        cfg,
		configPath: configPath,
		version:    version,
		errors:     make(map[string]string),
		pending:    make(map[string]bool),
		spinner:    s,
		notifier:   notify.New(),
	}
}

// ── 消息类型 ──────────────────────────────────────────

type refreshStartMsg struct{}

type usageResultMsg struct {
	name  string
	usage *provider.Usage
	err   error
}

type tickMsg time.Time

// ── Init ──────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, refresh())
}

func refresh() tea.Cmd {
	return func() tea.Msg {
		return refreshStartMsg{}
	}
}

func queryProvider(name string, p provider.Provider) tea.Cmd {
	return func() tea.Msg {
		usage, err := p.Query(context.Background())
		return usageResultMsg{name: name, usage: usage, err: err}
	}
}

// ── Update ────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// 全局按键
		switch msg.String() {
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		case "esc":
			if m.mode == viewConfig {
				m.mode = viewDashboard
				m.cfgSavedHint = ""
				return m, nil
			}
		}

		// 按视图模式分发
		if m.mode == viewConfig {
			return m.updateConfig(msg)
		}
		return m.updateDashboard(msg)

	case refreshStartMsg:
		return m.beginRefresh()

	case tickMsg:
		if !m.loading {
			return m.beginRefresh()
		}
		return m, m.scheduleTick()

	case usageResultMsg:
		delete(m.pending, msg.name)
		if msg.err != nil {
			m.errors[msg.name] = msg.err.Error()
		} else {
			m.usages = updateUsage(m.usages, msg.name, msg.usage)
		}
		if m.allDone() {
			m.loading = false
			m.lastRefresh = time.Now()
			events := m.notifier.Check(m.usages)
			m.notifications = notify.FormatEvents(events)
			return m, m.scheduleTick()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// updateDashboard 处理主面板按键。
func (m Model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "r":
		if !m.loading {
			m.notifications = nil
			return m.beginRefresh()
		}
	case "c":
		m.mode = viewConfig
		m.cfgCursor = 0
		m.cfgSavedHint = ""
		return m, nil
	}
	return m, nil
}

// updateConfig 处理配置页按键。
func (m Model) updateConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	providers := m.providerList()
	n := len(providers)

	switch msg.String() {
	case "up", "k":
		if m.cfgCursor > 0 {
			m.cfgCursor--
		}
	case "down", "j":
		if m.cfgCursor < n-1 {
			m.cfgCursor++
		}
	case " ":
		// 切换 provider 开关
		if m.cfgCursor < n {
			entry := providers[m.cfgCursor]
			entry.enabled = !entry.enabled
			m.applyProviderToggle(entry)
			m.cfgSavedHint = "已保存"
		}
	case "+", "=":
		m.cfg.RefreshIntervalMinutes++
		m.saveConfig()
		m.cfgSavedHint = "已保存"
	case "-", "_":
		if m.cfg.RefreshIntervalMinutes > 1 {
			m.cfg.RefreshIntervalMinutes--
			m.saveConfig()
			m.cfgSavedHint = "已保存"
		}
	case "t":
		// 切换重置提醒
		m.cfg.NotifyOnReset = !m.cfg.NotifyOnReset
		m.saveConfig()
		m.cfgSavedHint = "已保存"
	case "e":
		openConfig(m.configPath)
		m.cfgSavedHint = "编辑器已打开，修改后按 R 刷新"
	}
	return m, nil
}

// providerEntry 是配置页的一行。
type providerEntry struct {
	key     string // "glm" / "ark" / "deepseek"
	name    string // 展示名
	enabled bool
	hasKey  bool
}

// providerList 返回当前配置的 provider 列表（含状态）。
func (m Model) providerList() []providerEntry {
	return []providerEntry{
		{"glm", "智谱 GLM", m.cfg.Providers.GLM.Enabled, m.cfg.Providers.GLM.APIKey != ""},
		{"ark", "火山方舟", m.cfg.Providers.Ark.Enabled, m.cfg.Providers.Ark.AccessKeyID != ""},
		{"deepseek", "DeepSeek", m.cfg.Providers.DeepSeek.Enabled, m.cfg.Providers.DeepSeek.APIKey != ""},
	}
}

// applyProviderToggle 切换指定 provider 的 enabled 状态。
func (m *Model) applyProviderToggle(e providerEntry) {
	switch e.key {
	case "glm":
		m.cfg.Providers.GLM.Enabled = e.enabled
	case "ark":
		m.cfg.Providers.Ark.Enabled = e.enabled
	case "deepseek":
		m.cfg.Providers.DeepSeek.Enabled = e.enabled
	}
	m.saveConfig()
}

// saveConfig 保存配置到文件。
func (m *Model) saveConfig() {
	_ = config.Save(m.cfg)
}

// ── 刷新逻辑 ──────────────────────────────────────────

func (m Model) startRefresh() tea.Cmd {
	var cmds []tea.Cmd
	if m.cfg.Providers.GLM.Enabled {
		p := glm.New(m.cfg.Providers.GLM)
		cmds = append(cmds, queryProvider(p.Name(), p))
	}
	if m.cfg.Providers.Ark.Enabled {
		p := ark.New(m.cfg.Providers.Ark)
		cmds = append(cmds, queryProvider(p.Name(), p))
	}
	if m.cfg.Providers.DeepSeek.Enabled {
		p := deepseek.New(m.cfg.Providers.DeepSeek)
		cmds = append(cmds, queryProvider(p.Name(), p))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) beginRefresh() (tea.Model, tea.Cmd) {
	m.loading = true
	m.pending = make(map[string]bool)
	m.errors = make(map[string]string)
	if m.cfg.Providers.GLM.Enabled {
		m.pending["智谱 GLM"] = true
	}
	if m.cfg.Providers.Ark.Enabled {
		m.pending["火山方舟"] = true
	}
	if m.cfg.Providers.DeepSeek.Enabled {
		m.pending["DeepSeek"] = true
	}
	return m, m.startRefresh()
}

func (m Model) scheduleTick() tea.Cmd {
	interval := time.Duration(m.cfg.RefreshIntervalMinutes) * time.Minute
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) allDone() bool {
	return len(m.pending) == 0
}

func updateUsage(usages []*provider.Usage, name string, newUsage *provider.Usage) []*provider.Usage {
	if newUsage == nil {
		return usages
	}
	for i, u := range usages {
		if u.ProviderName == name {
			usages[i] = newUsage
			return usages
		}
	}
	return append(usages, newUsage)
}

// ── View ──────────────────────────────────────────────

func (m Model) View() string {
	if m.mode == viewConfig {
		return m.viewConfig()
	}
	return m.viewDashboard()
}

// viewDashboard 渲染主面板。
func (m Model) viewDashboard() string {
	var b strings.Builder

	// 标题栏
	b.WriteString(m.renderTitleBar())
	b.WriteString("\n\n")

	// 加载提示
	if m.loading && len(m.usages) == 0 && len(m.errors) == 0 {
		b.WriteString(styleLoading.Render(m.spinner.View() + " 正在查询..."))
		b.WriteString("\n\n")
	}

	// 卡片
	cardWidth := m.cardWidth()
	// 收集所有 provider 名称（包括有错误的）
	names := m.orderedProviderNames()
	for _, name := range names {
		b.WriteString(m.renderCard(name, cardWidth))
		b.WriteString("\n")
	}

	// 通知
	for _, n := range m.notifications {
		b.WriteString(styleNotify.Render(n))
		b.WriteString("\n")
	}

	// 底部帮助
	b.WriteString("\n")
	b.WriteString(styleHelp.Render("[R]刷新  [C]配置  [Q]退出"))

	return b.String()
}

// renderTitleBar 渲染顶部标题栏。
func (m Model) renderTitleBar() string {
	refreshInfo := ""
	if !m.lastRefresh.IsZero() {
		refreshInfo = fmt.Sprintf("最后刷新: %s", m.lastRefresh.Format("15:04:05"))
	}
	interval := fmt.Sprintf("⏱%dmin", m.cfg.RefreshIntervalMinutes)
	rightInfo := refreshInfo + "  " + interval
	// 版本号靠最右，用最暗的颜色，不干扰主要信息
	return styleTitle.Render("📊 AI 套餐余额监控") + "    " +
		styleHelp.Render(rightInfo) + "  " +
		styleHelp.Render(m.version)
}

// orderedProviderNames 返回按启用顺序排列的 provider 名称列表（含出错的）。
func (m Model) orderedProviderNames() []string {
	var names []string
	if m.cfg.Providers.GLM.Enabled {
		names = append(names, "智谱 GLM")
	}
	if m.cfg.Providers.Ark.Enabled {
		names = append(names, "火山方舟")
	}
	if m.cfg.Providers.DeepSeek.Enabled {
		names = append(names, "DeepSeek")
	}
	return names
}

// cardWidth 计算卡片内容区宽度。
func (m Model) cardWidth() int {
	w := 48
	if m.width > 0 {
		w = m.width - 4 // 两侧边框 + padding
		if w > 56 {
			w = 56
		}
		if w < 36 {
			w = 36
		}
	}
	return w
}

// renderCard 渲染单个 provider 的卡片。
func (m Model) renderCard(name string, width int) string {
	// 查找 usage
	var u *provider.Usage
	for _, uu := range m.usages {
		if uu.ProviderName == name {
			u = uu
			break
		}
	}

	// 查找错误
	errMsg := m.errors[name]

	// 卡片标题
	title := name
	if u != nil && u.Plan != "" {
		title = name + " (" + u.Plan + ")"
	}

	// 构建卡片内容
	var lines []string

	if errMsg != "" {
		// 出错：显示红色错误
		lines = append(lines, styleError.Render("❌ "+truncate(errMsg, width-4)))
	} else if u != nil {
		// 正常：逐行渲染
		for _, item := range u.Items {
			lines = append(lines, m.renderItemLine(item, width))
		}
	} else if m.loading {
		lines = append(lines, styleLoading.Render(m.spinner.View()+" 查询中..."))
	} else {
		lines = append(lines, styleHelp.Render("无数据"))
	}

	// 用 lipgloss 边框渲染
	content := strings.Join(lines, "\n")
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorHeader).
		Padding(0, 1).
		Width(width)

	// 标题嵌入边框顶部
	return cardStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(colorProvider).Bold(true).Render(title),
		"",
		content,
	))
}

// renderItemLine 渲染单条 UsageItem。
// 行格式: "label  bar/used  percent/value  resetsIn"
// 内容区可用宽度 = cardWidth - 2 (左右 padding) - 2 (边框)
func (m Model) renderItemLine(item provider.UsageItem, cardWidth int) string {
	// 实际内容区宽度
	contentW := cardWidth - 2
	labelW := 12
	label := padRight(item.Label, labelW)

	switch item.Format {
	case provider.FormatPercent:
		pct := item.Used
		resetStr := item.ResetsIn(time.Now())
		pctStr := fmt.Sprintf("%5.1f%%", pct)
		// bar 宽度 = 内容宽 - label - pctStr - resetStr - 间距
		rightPart := pctStr
		rightW := runewidth.StringWidth(pctStr)
		if resetStr != "" {
			rightPart += " " + resetStr
			rightW += 1 + runewidth.StringWidth(resetStr)
		}
		barW := contentW - labelW - rightW - 3 // 3 个空格间距
		if barW < 6 {
			barW = 6
		}
		bar := renderBar(pct, barW)
		line := label + " " + bar + " " + rightPart
		return colorizeByPercent(line, pct)

	case provider.FormatText:
		return label + " " + item.Unit

	case provider.FormatCount:
		usedStr := fmt.Sprintf("%.0f", item.Used)
		if item.Limit > 0 {
			usedStr = fmt.Sprintf("%.0f/%.0f", item.Used, item.Limit)
		}
		resetStr := item.ResetsIn(time.Now())
		line := label + " " + usedStr + " " + item.Unit
		if resetStr != "" {
			line += " " + resetStr
		}
		return line

	default:
		return label
	}
}

// renderBar 渲染进度条。
// 0-50% 绿色, 50-80% 黄色, 80%+ 红色。
func renderBar(pct float64, width int) string {
	if width < 1 {
		width = 1
	}
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	// 按阈值着色
	var color lipgloss.Color
	switch {
	case pct >= 80:
		color = colorRed
	case pct >= 50:
		color = colorYellow
	default:
		color = colorGreen
	}
	return lipgloss.NewStyle().Foreground(color).Render(bar)
}

// colorizeByPercent 根据百分比给整行着色。
func colorizeByPercent(line string, pct float64) string {
	switch {
	case pct >= 80:
		return lipgloss.NewStyle().Foreground(colorRed).Render(line)
	case pct >= 50:
		return lipgloss.NewStyle().Foreground(colorYellow).Render(line)
	default:
		return lipgloss.NewStyle().Foreground(colorGreen).Render(line)
	}
}

// ── 配置页视图 ────────────────────────────────────────

func (m Model) viewConfig() string {
	var b strings.Builder
	width := m.cardWidth()

	// 标题
	b.WriteString(styleTitle.Render("⚙ 配置管理"))
	b.WriteString("\n\n")

	// 配置卡片
	var lines []string
	providers := m.providerList()

	lines = append(lines, styleHeader.Render("  Provider                 状态"))
	lines = append(lines, styleHeader.Render(strings.Repeat("─", width-2)))

	for i, p := range providers {
		cursor := "  "
		if i == m.cfgCursor {
			cursor = styleSelected.Render("▶ ")
		}

		checkBox := "[✓]"
		if !p.enabled {
			checkBox = "[ ]"
		}
		if !p.hasKey {
			checkBox = styleError.Render(checkBox)
		}

		statusStr := "已配置"
		if !p.hasKey {
			statusStr = styleError.Render("未配置")
		}

		nameField := padRight(p.name, 18)
		line := cursor + checkBox + " " + nameField + " " + statusStr
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, styleHeader.Render(strings.Repeat("─", width-2)))
	lines = append(lines, "")

	// 刷新间隔
	intervalLine := fmt.Sprintf("  自动刷新间隔: %d 分钟  [+/-]调整", m.cfg.RefreshIntervalMinutes)
	lines = append(lines, intervalLine)

	// 重置提醒
	notifyStr := "开"
	if !m.cfg.NotifyOnReset {
		notifyStr = "关"
	}
	lines = append(lines, fmt.Sprintf("  重置提醒: %s  [T]切换", notifyStr))

	lines = append(lines, "")
	lines = append(lines, styleHelp.Render("  [E] 编辑 API Key (外部编辑器)"))

	// 保存提示
	if m.cfgSavedHint != "" {
		lines = append(lines, "")
		lines = append(lines, styleNotify.Render("  ✓ "+m.cfgSavedHint))
	}

	// 帮助
	lines = append(lines, "")
	lines = append(lines, styleHelp.Render("  [↑↓]选择 [Space]开关 [+/-]间隔 [E]编辑 [Esc]返回"))

	content := strings.Join(lines, "\n")
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorHeader).
		Padding(0, 1).
		Width(width)

	b.WriteString(cardStyle.Render(content))

	return b.String()
}

// ── 工具函数 ──────────────────────────────────────────

// padRight 将字符串右填充到指定宽度（考虑中文宽度）。
func padRight(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// openConfig 用系统默认编辑器打开配置文件。
func openConfig(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}

// truncate 截断字符串。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
