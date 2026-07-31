// Package provider 定义统一的套餐余额查询接口和数据模型。
package provider

import (
	"context"
	"time"
)

// Provider 是一个 AI 套餐余额查询提供方的统一接口。
// 每个 Provider 实现负责自己的鉴权和解析逻辑，返回标准化的 Usage。
type Provider interface {
	// Name 返回展示名称，如 "智谱 GLM"。
	Name() string
	// Query 查询当前余额/用量。ctx 用于超时控制。
	Query(ctx context.Context) (*Usage, error)
}

// Usage 是一次查询的标准化结果。
type Usage struct {
	ProviderName string     // 提供方名称
	Plan         string     // 套餐档位，如 "Pro"、"Coding Plan"
	Items        []UsageItem // 具体的用量条目（一个套餐可能有多个维度）
	UpdatedAt    time.Time  // 查询时间
}

// UsageItem 是单个维度的用量条目。
type UsageItem struct {
	Label    string     // 条目标签，如 "5h Token 用量"
	Used     float64    // 已用量（百分比时 0-100；计数时为已用数量）
	Limit    float64    // 上限（百分比时=100；计数时为总额度）
	Format   Format     // 格式类型
	Unit     string     // 计数单位，如 "次"、"积分"（百分比时为空）
	ResetsAt *time.Time // 重置时间，nil 表示无固定重置时间
}

// Format 描述用量的展示格式。
type Format string

const (
	FormatPercent Format = "percent" // 百分比（Used/Limit 均为 0-100）
	FormatCount   Format = "count"   // 计数（Used/Limit 为具体数量）
	FormatText    Format = "text"    // 纯文本状态（Unit 存放状态文本，Used/Limit 不使用）
)

// PercentLeft 返回剩余百分比（仅对 FormatPercent 有意义）。
func (i UsageItem) PercentLeft() float64 {
	left := i.Limit - i.Used
	if left < 0 {
		left = 0
	}
	return left
}

// ResetsIn 返回距离重置的时长字符串，如 "2h30m"；无重置时间返回 ""。
func (i UsageItem) ResetsIn(now time.Time) string {
	if i.ResetsAt == nil {
		return ""
	}
	d := i.ResetsAt.Sub(now)
	if d <= 0 {
		return "已重置"
	}
	return humanDuration(d)
}

// humanDuration 将时长转为人类可读字符串。
func humanDuration(d time.Duration) string {
	if d < time.Hour {
		return d.Truncate(time.Minute).String()
	}
	if d < 24*time.Hour {
		return d.Truncate(time.Minute).String()
	}
	days := int(d / (24 * time.Hour))
	rem := d - time.Duration(days)*24*time.Hour
	hours := int(rem / time.Hour)
	if hours > 0 {
		return formatDays(days) + " " + formatHours(hours)
	}
	return formatDays(days)
}

func formatDays(days int) string {
	if days == 1 {
		return "1天"
	}
	return itoa(days) + "天"
}

func formatHours(hours int) string {
	if hours == 1 {
		return "1小时"
	}
	return itoa(hours) + "小时"
}

// itoa 是不依赖 strconv 的简单整数转字符串（避免循环导入）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
