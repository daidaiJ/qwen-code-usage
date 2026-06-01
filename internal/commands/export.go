// Package commands 命令实现
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panda/qwen-usage/internal/database"
)

// RunExport 执行export命令
func RunExport(period, from, to string, outputFormat string, limit int) int {
	db, err := database.GetDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	// 如果指定了 -n 参数，输出最近N条记录列表
	if limit > 0 {
		records, err := db.GetRecentRecords(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get recent records: %v\n", err)
			return 1
		}
		outputRecordsList(records, outputFormat)
		return 0
	}

	startTime, endTime := getPeriodRange(period)

	stats, err := db.GetStats(startTime, endTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get stats: %v\n", err)
		return 1
	}

	switch outputFormat {
	case "json":
		outputJSON(stats)
	case "markdown", "md":
		outputMarkdown(stats, period)
	default:
		outputMarkdown(stats, period)
	}

	return 0
}

// getPeriodRange 计算时间范围
func getPeriodRange(period string) (start, end time.Time) {
	end = time.Now()
	switch strings.ToLower(period) {
	case "day":
		start = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	case "week":
		start = end.AddDate(0, 0, -7)
	case "month":
		start = end.AddDate(0, -1, 0)
	case "5h":
		start = end.Add(-5 * time.Hour)
	default:
		start = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	}
	return
}

// outputJSON 输出JSON格式
func outputJSON(stats *database.StatsResponse) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(stats)
}

// outputMarkdown 输出Markdown格式
func outputMarkdown(stats *database.StatsResponse, period string) {
	periodName := getPeriodName(period)

	fmt.Printf("# Token 用量报告 (%s)\n\n", periodName)
	fmt.Printf("**时间范围**: %s ~ %s\n\n", stats.StartTime, stats.EndTime)

	if len(stats.Models) == 0 {
		fmt.Println("*无数据*")
		return
	}

	fmt.Println("## 按模型统计")
	fmt.Println("")
	fmt.Println("| Model | Requests | Latency(Avg/P50/P95) | Prompt | Completion | Cached | Thoughts | Cache% | TPS(token/s) |")
	fmt.Println("|-------|----------|----------------------|--------|------------|--------|----------|--------|-----|")

	for _, m := range stats.Models {
		fmt.Printf("| %s | %d | %.0fms/%.0fms/%.0fms | %s | %s | %s | %s | %.1f%% | %.1f |\n",
			m.ModelName,
			m.RequestCount,
			m.AvgLatencyMs,
			m.P50LatencyMs,
			m.P95LatencyMs,
			formatNumber(m.PromptTokens),
			formatNumber(m.CompletionTokens),
			formatNumber(m.CachedTokens),
			formatNumber(m.ThoughtsTokens),
			m.CachePercent,
			m.TokensPerSec,
		)
	}

	fmt.Printf("\n## 汇总\n\n")
	fmt.Printf("- **总调用**: %d 次\n", stats.Total.RequestCount)
	fmt.Printf("- **总 Token**: %s\n", formatNumber(stats.Total.TotalTokens))
	fmt.Printf("- **平均延迟**: %.1f ms\n", stats.Total.AvgLatencyMs)

	if stats.ContextWindowMax != nil {
		ext := stats.ContextWindowMax
		fmt.Printf("\n## 上下文窗口历史最值\n\n")
		fmt.Printf("| 指标 | 值 | 模型 | 时间 |\n|------|----|------|------|\n")

		usageModel := ext.MaxCurrentUsageModel
		if usageModel == "" {
			usageModel = "-"
		}
		usageTime := "-"
		if !ext.MaxCurrentUsageTime.IsZero() {
			usageTime = ext.MaxCurrentUsageTime.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("| 最大上下文使用量 | %s | %s | %s |\n", formatNumber(int64(ext.MaxCurrentUsage)), usageModel, usageTime)

		inputModel := ext.MaxSingleInputModel
		if inputModel == "" {
			inputModel = "-"
		}
		inputTime := "-"
		if !ext.MaxSingleInputTime.IsZero() {
			inputTime = ext.MaxSingleInputTime.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("| 单次最大输入 | %s | %s | %s |\n", formatNumber(int64(ext.MaxSingleInputTokens)), inputModel, inputTime)

		outputModel := ext.MaxSingleOutputModel
		if outputModel == "" {
			outputModel = "-"
		}
		outputTime := "-"
		if !ext.MaxSingleOutputTime.IsZero() {
			outputTime = ext.MaxSingleOutputTime.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("| 单次最大输出 | %s | %s | %s |\n", formatNumber(int64(ext.MaxSingleOutputTokens)), outputModel, outputTime)
	}
}

// getPeriodName 获取周期名称
func getPeriodName(period string) string {
	switch period {
	case "day":
		return "今天"
	case "week":
		return "最近一周"
	case "month":
		return "最近一个月"
	case "5h":
		return "最近 5 小时"
	default:
		return period
	}
}

// formatNumber 格式化数字
func formatNumber(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// outputRecordsList 输出记录列表
func outputRecordsList(records []database.CallRecord, format string) {
	if len(records) == 0 {
		fmt.Println("无记录")
		return
	}

	if format == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(records)
		return
	}

	// 表格格式输出
	fmt.Println("| Time | Model | Req# | Latency | Prompt | Completion | Cached | Thoughts | Total |")
	fmt.Println("|------|-------|------|---------|--------|------------|--------|----------|-------|")

	for _, r := range records {
		fmt.Printf("| %s | %s | %d | %dms | %s | %s | %s | %s | %s |\n",
			r.RecordedAt.Format("01-02 15:04:05"),
			r.ModelName,
			r.RequestSeq,
			r.LatencyMs,
			formatNumber(int64(r.PromptTokens)),
			formatNumber(int64(r.CompletionTokens)),
			formatNumber(int64(r.CachedTokens)),
			formatNumber(int64(r.ThoughtsTokens)),
			formatNumber(int64(r.TotalTokens)),
		)
	}
}

// RunClear 执行clear命令
func RunClear(days int) int {
	db, err := database.GetDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	beforeDate := time.Now().AddDate(0, 0, -days)

	deleted, err := db.DeleteOldRecords(beforeDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to delete old records: %v\n", err)
		return 1
	}

	fmt.Printf("已清理 %d 条 %s 之前的记录", deleted, beforeDate.Format("2006-01-02"))
	return 0
}
