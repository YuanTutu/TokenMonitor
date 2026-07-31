// Package main 是 TokenMonitor 桌面应用的入口。
//
// 基于 Wails v2（Go 后端 + Web 前端），展示各 AI 套餐余额。
// 首次运行若无配置则在前端 UI 内引导填写 API Key。
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// 版本信息，默认值用于 go run / 未注入 ldflags 的开发构建。
// 正式发布时由构建脚本通过 -ldflags 注入：
//
//	-X main.version=v1.0.0 -X main.buildDate=20260731
var (
	version   = "dev"
	buildDate = "unknown"
)

// versionString 返回展示用的完整版本号，格式: vx.x.x build YYYYMMDD。
func versionString() string {
	return fmt.Sprintf("%s build %s", version, buildDate)
}

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// -v / --version 打印版本号后退出（命令行模式，不启动 GUI）
	showVersion := flag.Bool("version", false, "打印版本号并退出")
	flag.BoolVar(showVersion, "v", false, "打印版本号并退出（简写）")
	flag.Parse()

	if *showVersion {
		fmt.Println("TokenMonitor " + versionString())
		return
	}

	// 启动 Wails 桌面应用
	app := NewApp(versionString())

	err := wails.Run(&options.App{
		Title:  "TokenMonitor",
		Width:  880,
		Height: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 248, G: 250, B: 252, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 启动失败: %v\n", err)
		os.Exit(1)
	}
}
