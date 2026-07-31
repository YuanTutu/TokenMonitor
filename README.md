# TokenMonitor

桌面应用，集中监控多个 AI 平台的套餐余额和用量。基于 Wails v2（Go 后端 + Web 前端），提供现代精致的图形界面。

## 支持的平台

| 平台 | 展示内容 | 鉴权方式 |
|------|---------|---------|
| 智谱 GLM | 5h Token 用量、周 Token 用量、MCP 月度次数 | API Key |
| 火山方舟 (Volcengine Ark) | 5h 窗口额度、周额度、月额度、套餐到期 | AccessKey + SecretKey（V4 签名） |
| DeepSeek | 账户状态、总余额、赠送余额、充值余额 | API Key |

## 功能特性

- 🖥️ **桌面 GUI 界面** - 基于 Wails v2 + WebView2，圆角卡片、进度条可视化、深色主题
- 📊 **竖向卡片列表** - 每个 provider 一张卡片，支持向下滚动，便于后续扩展更多平台
- 🎨 **进度条阈值着色** - 绿 <50%、黄 50-80%、红 ≥80%
- ⏱️ **自动刷新** - 可配置刷新间隔，默认 10 分钟
- 🔔 **重置提醒** - 检测套餐额度重置并通知
- ⚠️ **用量/余额告警** - 用量达阈值（默认 90%）或余额过低（默认 10 CNY）时告警，越过提醒一次、回落清除
- ⚙️ **应用内配置** - 直接在界面内开关 provider、编辑 API Key、调整阈值
- 🔐 **本地配置** - 凭据存储在 `~/.tokenusage/config.json`，明文不加密

## 界面预览

桌面应用采用深色主题，竖向卡片列表，进度条可视化用量，顶部工具栏刷新与配置：

- **主面板**：每个 AI 平台一张圆角卡片，含进度条（绿/黄/红阈值着色）、用量百分比、重置倒计时；卡片按启用顺序竖向排列，超出窗口可滚动
- **配置页**：点右上角 ⚙️ 进入，可开关 provider、直接输入编辑 API Key（脱敏显示）、调整刷新间隔与告警阈值
- **通知区**：重置提醒（绿）与用量/余额告警（红）显示在卡片上方
- **状态栏**：标题、版本号、最后刷新时间、刷新间隔

## 操作方式

桌面应用以鼠标点击为主，辅以键盘：

| 操作 | 主面板 | 配置页 |
|------|--------|--------|
| 刷新 | 点「刷新」按钮 | - |
| 进入配置 | 点右上角 ⚙️ | - |
| 返回主面板 | - | 点「← 返回」 |
| 开关 Provider | - | 点击 toggle 开关 |
| 编辑 API Key | - | 点「编辑 Key」，输入后回车保存 |
| 调整刷新间隔 | - | 点 +/− 按钮 |
| 调整告警阈值 | - | 点 +/− 按钮 |
| 切换重置提醒 | - | 点击 toggle 开关 |
| 外部编辑器 | - | 点 📝 用系统编辑器打开 config.json |
| `Esc` | - | 关闭编辑 Key 弹窗 |

## 安装与使用

### 直接使用预编译包

从 [Releases](https://github.com/YuanTutu/TokenMonitor/releases) 下载 zip，解压后双击 `tokenmonitor.exe` 运行。

> 运行需要 WebView2 运行时（Windows 11 已内置，Windows 10 会自动引导安装）。

### 从源码构建

前置条件：
- Go 1.26+
- Node.js 18+（仅构建时需要，运行时不需要）
- [Wails CLI](https://wails.io/)：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

```bash
# 开发热重载
wails dev

# 生产构建（产出 build/bin/tokenmonitor.exe）
wails build
```

首次运行会在应用内显示配置引导，点右上角 ⚙️ 进入配置页填写各平台 API Key。

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
  "notifyOnReset": true,
  "alertPercentThreshold": 90,
  "alertBalanceThreshold": 10
}
```

各平台 API Key 获取方式：
- **智谱 GLM**: https://open.bigmodel.cn 控制台 > API Keys
- **火山方舟**: https://console.volcengine.com IAM > AccessKey
- **DeepSeek**: https://platform.deepseek.com API Keys

## 项目结构

```
TokenMonitor/
├── main.go                        # Wails 应用入口，版本号注入
├── app.go                         # Wails 绑定层，暴露 config/provider/notify 给前端
├── wails.json                     # Wails 配置
├── config/
│   └── config.go                  # 配置结构体 + 读写 ~/.tokenusage/config.json
├── provider/
│   ├── provider.go                # Provider 接口 + 统一数据模型
│   ├── glm/glm.go                 # 智谱 GLM 用量查询
│   ├── ark/
│   │   ├── ark.go                 # 火山方舟用量查询
│   │   └── signer.go              # Volcengine V4 签名算法
│   └── deepseek/deepseek.go       # DeepSeek 余额查询
├── notify/
│   └── notify.go                  # 重置提醒 + 用量/余额告警逻辑
├── frontend/                      # Web 前端（HTML/CSS/JS + Vite）
│   ├── index.html
│   ├── src/main.js                # 前端逻辑（渲染、交互、刷新调度）
│   └── src/style.css              # 现代深色主题样式
└── build/                         # Wails 构建配置与产物
```

## 技术栈

- [Go](https://go.dev/) 1.26 - 后端
- [Wails](https://wails.io/) v2 - 桌面应用框架（Go + WebView2）
- [Vite](https://vitejs.dev/) - 前端构建工具
- 原生 HTML/CSS/JS - 前端（无框架，轻量）

## License

MIT
