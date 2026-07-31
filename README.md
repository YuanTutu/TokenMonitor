# TokenMonitor

命令行 TUI 工具，集中监控多个 AI 平台的套餐余额和用量。

## 支持的平台

| 平台 | 展示内容 | 鉴权方式 |
|------|---------|---------|
| 智谱 GLM | 5h Token 用量、周 Token 用量、MCP 月度次数 | API Key |
| 火山方舟 (Volcengine Ark) | 5h 窗口额度、周额度、月额度、套餐到期 | AccessKey + SecretKey（V4 签名） |
| DeepSeek | 账户状态、总余额、赠送余额、充值余额 | API Key |

## 功能特性

- 📊 **卡片式 TUI 界面** - 每个 provider 一个带边框卡片，进度条可视化用量
- 🎨 **阈值着色** - 进度条按用量百分比着色（绿 <50%、黄 50-80%、红 ≥80%）
- ⏱️ **自动刷新** - 可配置刷新间隔，默认 10 分钟
- 🔔 **重置提醒** - 检测套餐额度重置并通知
- ⚙️ **TUI 内配置** - 开关 provider、调整刷新间隔，或按 E 用外部编辑器编辑配置
- 🔐 **本地配置** - 凭据存储在 `~/.tokenusage/config.json`，明文不加密

## 界面预览

```
╭─ 智谱 GLM (pro) ──────────────────────────────╮
│ 5h Token 用量 ██████████████░░░░░  58.0% 1h40m │
│ 周 Token 用量 ████████████████░░  72.0% 5天6h  │
│ MCP 月度次数 102/1000 次 27天6小时              │
╰───────────────────────────────────────────────╯
╭─ 火山方舟 (Coding Plan Pro) ──────────────────╮
│ 5小时窗口额度 ███████████░░░░░░  43.2% 49m0s  │
│ 周额度       ██████████████░░░  65.8% 2天12h  │
│ 月额度       ██████░░░░░░░░░░░  32.9% 28天12h │
│ 套餐到期     2026-08-28                        │
╰───────────────────────────────────────────────╯
╭─ DeepSeek (API 按量付费) ─────────────────────╮
│ 账户状态     可用                              │
│ 总余额       30 CNY                            │
│ 赠送余额     0 CNY                             │
│ 充值余额     30 CNY                            │
╰───────────────────────────────────────────────╯

[R]刷新  [C]配置  [Q]退出
```

## 快捷键

| 按键 | 主面板 | 配置页 |
|------|--------|--------|
| `R` | 手动刷新 | - |
| `C` | 进入配置页 | - |
| `Q` / `Ctrl+C` | 退出 | 退出 |
| `↑` / `↓` | - | 选择 provider |
| `Space` | - | 切换 provider 开关 |
| `+` / `-` | - | 调整刷新间隔 |
| `T` | - | 切换重置提醒 |
| `E` | - | 用编辑器打开配置文件 |
| `Esc` | - | 返回主面板 |

## 安装与使用

### 前置条件

- Go 1.21+

### 编译运行

```bash
go build -o tokenmonitor.exe .
./tokenmonitor.exe
```

首次运行会进入配置引导，依次输入各平台的 API Key（可留空跳过）。

### 配置文件

配置存储在 `~/.tokenusage/config.json`：

```json
{
  "providers": {
    "glm": { "enabled": true, "apiKey": "your-glm-api-key" },
    "ark": {
      "enabled": true,
      "accessKeyId": "your-ak",
      "secretAccessKey": "your-sk",
      "region": "cn-beijing"
    },
    "deepseek": { "enabled": true, "apiKey": "your-deepseek-api-key" }
  },
  "refreshIntervalMinutes": 10,
  "notifyOnReset": true
}
```

各平台 API Key 获取方式：
- **智谱 GLM**: https://open.bigmodel.cn 控制台 > API Keys
- **火山方舟**: https://console.volcengine.com IAM > AccessKey
- **DeepSeek**: https://platform.deepseek.com API Keys

## 项目结构

```
TokenMonitor/
├── main.go                        # 入口，配置引导 + 启动 TUI
├── config/
│   └── config.go                  # 配置结构体 + 读写 ~/.tokenusage/config.json
├── provider/
│   ├── provider.go                # Provider 接口 + 统一数据模型
│   ├── glm/
│   │   └── glm.go                 # 智谱 GLM 用量查询
│   └── ark/
│       ├── ark.go                 # 火山方舟用量查询
│       └── signer.go              # Volcengine V4 签名算法
│   └── deepseek/
│       └── deepseek.go            # DeepSeek 余额查询
├── notify/
│   └── notify.go                  # 重置提醒逻辑
└── tui/
    └── tui.go                     # BubbleTea TUI 界面
```

## 技术栈

- [Go](https://go.dev/) 1.26
- [BubbleTea](https://github.com/charmbracelet/bubbletea) - TUI 框架
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - 终端样式
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI 组件（spinner）

## License

MIT
