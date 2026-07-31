// Package main 是 TokenMonitor 程序的入口。
//
// 启动命令行 TUI 展示各 AI 套餐余额。首次运行若无配置则引导填写 API Key。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokenmonitor/config"
	"tokenmonitor/tui"
)

// 版本信息，默认值用于 go run / 未注入 ldflags 的开发构建。
// 正式发布时由构建脚本通过 -ldflags 注入：
//   -X main.version=v0.1.0 -X main.buildDate=20260731
var (
	version   = "dev"
	buildDate = "unknown"
)

// versionString 返回展示用的完整版本号，格式: vx.x.x build YYYYMMDD。
func versionString() string {
	return fmt.Sprintf("%s build %s", version, buildDate)
}

func main() {
	// -v / --version 打印版本号后退出
	showVersion := flag.Bool("version", false, "打印版本号并退出")
	flag.BoolVar(showVersion, "v", false, "打印版本号并退出（简写）")
	flag.Parse()

	if *showVersion {
		fmt.Println("TokenMonitor " + versionString())
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 加载配置失败: %v\n", err)
		os.Exit(1)
	}

	configPath, _ := config.ConfigPath()

	// 首次运行或无任何 Provider 启用时，进入配置引导
	if !cfg.AnyEnabled() {
		fmt.Println("🔧 首次使用，请配置 AI 套餐的凭据。")
		fmt.Println("配置文件位置:", configPath)
		fmt.Println()
		runSetup(cfg)
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n✅ 配置已保存到", configPath)
	}

	// 重新加载（用户可能刚改了配置）
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 重新加载配置失败: %v\n", err)
		os.Exit(1)
	}

	if !cfg.AnyEnabled() {
		fmt.Println("⚠️ 没有启用任何 Provider。请编辑配置文件后重试:")
		fmt.Println("  ", configPath)
		os.Exit(0)
	}

	// 启动 TUI
	p := tea.NewProgram(tui.New(cfg, configPath, versionString()), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 启动失败: %v\n", err)
		os.Exit(1)
	}
}

// runSetup 交互式配置引导，让用户填写各 Provider 的凭据。
func runSetup(cfg *config.Config) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== 智谱 GLM ===")
	fmt.Println("端点: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit")
	fmt.Println("鉴权: API Key 裸值（不带 Bearer）")
	fmt.Print("请输入智谱 API Key（留空跳过）: ")
	apiKey := readLine(reader)
	if apiKey != "" {
		cfg.Providers.GLM.Enabled = true
		cfg.Providers.GLM.APIKey = apiKey
		fmt.Println("  ✅ 已启用智谱 GLM")
	} else {
		cfg.Providers.GLM.Enabled = false
	}

	fmt.Println()
	fmt.Println("=== 火山方舟 ===")
	fmt.Println("端点: POST https://open.volcengineapi.com/?Action=GetUsageDetails&Version=2024-01-01")
	fmt.Println("鉴权: 火山引擎 IAM AccessKey/SecretKey（V4 签名）")
	fmt.Print("请输入 AccessKey ID（留空跳过）: ")
	ak := readLine(reader)
	if ak != "" {
		fmt.Print("请输入 Secret Access Key: ")
		sk := readLine(reader)
		if sk != "" {
			cfg.Providers.Ark.Enabled = true
			cfg.Providers.Ark.AccessKeyID = ak
			cfg.Providers.Ark.SecretAccessKey = sk
			fmt.Println("  ✅ 已启用火山方舟")
		} else {
			fmt.Println("  ⚠️ SK 为空，跳过火山方舟")
		}
	} else {
		cfg.Providers.Ark.Enabled = false
	}

	fmt.Println()
	fmt.Println("=== DeepSeek ===")
	fmt.Println("端点: GET https://api.deepseek.com/user/balance")
	fmt.Println("鉴权: Bearer API Key")
	fmt.Println("查询账户余额（总余额、赠送余额、充值余额）")
	fmt.Print("请输入 DeepSeek API Key（留空跳过）: ")
	dskKey := readLine(reader)
	if dskKey != "" {
		cfg.Providers.DeepSeek.Enabled = true
		cfg.Providers.DeepSeek.APIKey = dskKey
		fmt.Println("  ✅ 已启用 DeepSeek")
	} else {
		cfg.Providers.DeepSeek.Enabled = false
	}

	fmt.Println()
	fmt.Print("自动刷新间隔(分钟，默认10): ")
	intervalStr := readLine(reader)
	if intervalStr == "" {
		cfg.RefreshIntervalMinutes = 10
	} else {
		var n int
		fmt.Sscanf(intervalStr, "%d", &n)
		if n > 0 {
			cfg.RefreshIntervalMinutes = n
		} else {
			cfg.RefreshIntervalMinutes = 10
		}
	}
}

// readLine 读取一行输入并 trim 空白。
func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
