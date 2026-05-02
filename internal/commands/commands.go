// Package commands 命令实现
package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/panda/qwen-usage/internal/config"
	"github.com/panda/qwen-usage/internal/database"
	"github.com/panda/qwen-usage/pkg/platform"
)

// RunRecord 执行record命令
func RunRecord() int {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read stdin: %v\n", err)
		return 1
	}

	var statusInput database.StatusLineInput
	if err := json.Unmarshal(input, &statusInput); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse json: %v\n", err)
		return 1
	}

	cfg := config.GetConfig()

	client := &http.Client{
		Timeout: time.Duration(cfg.ClientTimeoutMs) * time.Millisecond,
	}

	url := fmt.Sprintf("http://%s/record", cfg.ServerAddr)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(input))
	if err != nil {
		printFallbackStatusLine(statusInput)
		return 0
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		printFallbackStatusLine(statusInput)
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		printFallbackStatusLine(statusInput)
		return 0
	}

	var recordResp database.RecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&recordResp); err != nil {
		printFallbackStatusLine(statusInput)
		return 0
	}

	fmt.Println(recordResp.StatusLine)
	return 0
}

// printFallbackStatusLine 降级输出状态行
func printFallbackStatusLine(input database.StatusLineInput) {
	modelName := input.Model.DisplayName
	if modelName == "" {
		modelName = "unknown"
	}

	var apiRequests int
	var promptTokens int
	var cachedTokens int
	var totalTokens int

	for _, metrics := range input.Metrics.Models {
		apiRequests += metrics.API.TotalRequests
		promptTokens += metrics.Tokens.Prompt
		cachedTokens += metrics.Tokens.Cached
		totalTokens += metrics.Tokens.Total
	}

	var cachePercent float64
	if promptTokens > 0 {
		cachePercent = float64(cachedTokens) * 100 / float64(promptTokens)
	}

	ctxPercent := input.ContextWindow.UsedPercentage

	statusLine := fmt.Sprintf("API:%d | cached:%.0f%% | ctx:%.1f%% | tokens:%d | model: %s",
		apiRequests, cachePercent, ctxPercent, totalTokens, modelName)

	fmt.Println(statusLine)
}

// RunExport 执行export命令
func RunExport(period, from, to string, outputFormat string, limit int) int {
	db, err := database.GetDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init database: %v\n", err)
		return 1
	}
	defer db.Close()

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
	encoder.Encode(stats)
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
	fmt.Println("| Model | Requests | Latency(Avg/P50/P95) | Prompt | Completion | Cached | Thoughts | Cache% | t/s |")
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
		encoder.Encode(records)
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
	defer db.Close()

	beforeDate := time.Now().AddDate(0, 0, -days)

	deleted, err := db.DeleteOldRecords(beforeDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to delete old records: %v\n", err)
		return 1
	}

	fmt.Printf("已清理 %d 条 %s 之前的记录", deleted, beforeDate.Format("2006-01-02"))
	return 0
}

// RunStart 执行start命令（启动会话）
func RunStart() int {
	cfg := config.GetConfig()

	url := fmt.Sprintf("http://%s/session/start", cfg.ServerAddr)
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建请求失败: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接 server 失败: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	var sessionResp database.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		fmt.Fprintf(os.Stderr, "解析响应失败: %v\n", err)
		return 1
	}

	fmt.Printf("会话开始，当前计数: %d\n", sessionResp.Count)
	return 0
}

// RunStop 执行stop命令（结束会话）
func RunStop() int {
	cfg := config.GetConfig()

	url := fmt.Sprintf("http://%s/session/end", cfg.ServerAddr)
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建请求失败: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接 server 失败: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	var sessionResp database.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		fmt.Fprintf(os.Stderr, "解析响应失败: %v\n", err)
		return 1
	}

	fmt.Printf("会话结束，当前计数: %d\n", sessionResp.Count)
	if sessionResp.Count == 0 {
		fmt.Println("计数归零，server 正在关闭...")
	}
	return 0
}

// RunKill 执行kill命令（强制关闭）
func RunKill() int {
	cfg := config.GetConfig()

	url := fmt.Sprintf("http://%s/shutdown", cfg.ServerAddr)
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 shutdown 请求失败: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接 server 失败: %v\n", err)
		pid, readErr := platform.ReadPIDFile()
		if readErr == nil {
			fmt.Fprintln(os.Stderr, platform.GetKillCommandHint(pid))
		}
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "server 返回错误: %d\n", resp.StatusCode)
		return 1
	}

	fmt.Println("server 正在强制关闭...")
	return 0
}