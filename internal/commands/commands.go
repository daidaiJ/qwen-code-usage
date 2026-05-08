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
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var recordResp database.RecordResponse
				if err := json.NewDecoder(resp.Body).Decode(&recordResp); err == nil {
					fmt.Println(recordResp.StatusLine)
					return 0
				}
			}
		}
	}

	// server 不可用，降级为本地直接写入
	statusLine := recordLocalFallback(statusInput)
	fmt.Println(statusLine)
	return 0
}

// recordLocalFallback server 不可用时，直接写入本地 SQLite 并返回状态行
func recordLocalFallback(input database.StatusLineInput) string {
	modelName := input.Model.DisplayName
	if modelName == "" {
		modelName = "unknown"
	}

	var statusLine string
	recorded := false

	db, err := database.GetDB()
	if err == nil {
		defer db.Close()

		for mName, metrics := range input.Metrics.Models {
			if metrics.API.TotalRequests == 0 {
				continue
			}

			if err := processMetricsLocal(db, input.SessionID, mName, metrics); err == nil {
				recorded = true
			}

			if statusLine == "" {
				statusLine = formatStatusLineLocal(input, mName, metrics)
			}
		}

		// 更新上下文窗口历史最值
		if input.ContextWindow.ContextWindowSize > 0 {
			extremes := &database.ContextWindowExtremes{
				MaxContextWindowSize: input.ContextWindow.ContextWindowSize,
				MaxTotalInputTokens:  input.ContextWindow.TotalInputTokens,
				MaxTotalOutputTokens: input.ContextWindow.TotalOutputTokens,
			}
			db.UpdateContextWindowExtremes(extremes)
		}
	}

	// 未产生记录或无 DB 时，仅格式化输出
	if statusLine == "" {
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

		statusLine = fmt.Sprintf("API:%d | cached:%.0f%% | ctx:%.1f%% | tokens:%d | model: %s",
			apiRequests, cachePercent, input.ContextWindow.UsedPercentage, totalTokens, modelName)
	}

	_ = recorded
	return statusLine
}

// processMetricsLocal 本地计算增量并写入 DB（与 server.processModelMetrics 逻辑一致）
func processMetricsLocal(db database.DB, sessionID, modelName string, metrics database.ModelMetrics) error {
	prevState, err := db.GetCumulativeState(sessionID, modelName)
	if err != nil {
		return err
	}

	deltaRequests := metrics.API.TotalRequests - prevState.TotalRequests
	if deltaRequests <= 0 {
		return nil
	}

	deltaLatencyMs := metrics.API.TotalLatencyMs - prevState.TotalLatencyMs
	deltaPrompt := metrics.Tokens.Prompt - prevState.PromptTokens
	deltaCompletion := metrics.Tokens.Completion - prevState.CompletionTokens
	deltaCached := metrics.Tokens.Cached - prevState.CachedTokens
	deltaThoughts := metrics.Tokens.Thoughts - prevState.ThoughtsTokens
	deltaTotal := metrics.Tokens.Total - prevState.TotalTokens

	if deltaLatencyMs < 0 {
		deltaLatencyMs = 0
	}
	if deltaPrompt < 0 {
		deltaPrompt = 0
	}
	if deltaCompletion < 0 {
		deltaCompletion = 0
	}
	if deltaCached < 0 {
		deltaCached = 0
	}
	if deltaThoughts < 0 {
		deltaThoughts = 0
	}
	if deltaTotal < 0 {
		deltaTotal = 0
	}

	var singleLatencyMs int
	if deltaRequests > 0 {
		singleLatencyMs = deltaLatencyMs / deltaRequests
	}

	record := &database.CallRecord{
		SessionID:        sessionID,
		ModelName:        modelName,
		RequestSeq:       metrics.API.TotalRequests,
		LatencyMs:        singleLatencyMs,
		PromptTokens:     deltaPrompt,
		CompletionTokens: deltaCompletion,
		CachedTokens:     deltaCached,
		ThoughtsTokens:   deltaThoughts,
		TotalTokens:      deltaTotal,
	}

	if err := db.InsertCallRecord(record); err != nil {
		return err
	}

	newState := &database.CumulativeState{
		SessionID:        sessionID,
		ModelName:        modelName,
		TotalRequests:    metrics.API.TotalRequests,
		TotalLatencyMs:   metrics.API.TotalLatencyMs,
		PromptTokens:     metrics.Tokens.Prompt,
		CompletionTokens: metrics.Tokens.Completion,
		CachedTokens:     metrics.Tokens.Cached,
		ThoughtsTokens:   metrics.Tokens.Thoughts,
		TotalTokens:      metrics.Tokens.Total,
	}

	return db.UpdateCumulativeState(newState)
}

// formatStatusLineLocal 本地格式化状态行
func formatStatusLineLocal(input database.StatusLineInput, modelName string, metrics database.ModelMetrics) string {
	apiRequests := metrics.API.TotalRequests
	promptTokens := metrics.Tokens.Prompt
	cachedTokens := metrics.Tokens.Cached
	totalTokens := metrics.Tokens.Total
	ctxPercent := input.ContextWindow.UsedPercentage

	var cachePercent float64
	if promptTokens > 0 {
		cachePercent = float64(cachedTokens) * 100 / float64(promptTokens)
	}

	return fmt.Sprintf("API:%d | cached:%.0f%% | ctx:%.1f%% | tokens:%d | model: %s",
		apiRequests, cachePercent, ctxPercent, totalTokens, modelName)
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
	fmt.Println("| Model | Requests | Latency(Avg/P90/P95) | Prompt | Completion | Cached | Thoughts | Cache% | TPS(token/s) |")
	fmt.Println("|-------|----------|----------------------|--------|------------|--------|----------|--------|-----|")

	for _, m := range stats.Models {
		fmt.Printf("| %s | %d | %.0fms/%.0fms/%.0fms | %s | %s | %s | %s | %.1f%% | %.1f |\n",
			m.ModelName,
			m.RequestCount,
			m.AvgLatencyMs,
			m.P90LatencyMs,
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
		fmt.Printf("| 指标 | 值 |\n|------|----|\n")
		fmt.Printf("| 最大上下文窗口 | %s |\n", formatNumber(int64(ext.MaxContextWindowSize)))
		fmt.Printf("| 最大输入 Tokens | %s |\n", formatNumber(int64(ext.MaxTotalInputTokens)))
		fmt.Printf("| 最大输出 Tokens | %s |\n", formatNumber(int64(ext.MaxTotalOutputTokens)))
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
// 流程: 探测server → 无则自动启动 → 发送 /session/start 增加计数
func RunStart() int {
	cfg := config.GetConfig()
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://%s/health", cfg.ServerAddr)
	startURL := fmt.Sprintf("http://%s/session/start", cfg.ServerAddr)

	// 探测 server 是否已就绪
	if !isServerReady(client, healthURL) {
		// server 未运行，尝试自动启动
		fmt.Println("server 未运行，正在启动...")
		if err := platform.StartServerInBackground(); err != nil {
			fmt.Fprintf(os.Stderr, "启动 server 失败: %v\n", err)
			return 1
		}

		// 等待 server 就绪（最多 5 秒）
		if !waitForServer(client, healthURL, 5*time.Second) {
			fmt.Fprintln(os.Stderr, "server 启动超时")
			return 1
		}
	}

	// 发送 session/start
	resp, err := client.Post(startURL, "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "发送 session/start 失败: %v\n", err)
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
// 流程: 向 server 发送 /session/end 减少计数，归零时 server 自动退出
func RunStop() int {
	cfg := config.GetConfig()
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://%s/health", cfg.ServerAddr)
	endURL := fmt.Sprintf("http://%s/session/end", cfg.ServerAddr)

	// server 不存在时直接返回，无需报错
	if !isServerReady(client, healthURL) {
		return 0
	}

	resp, err := client.Post(endURL, "application/json", nil)
	if err != nil {
		// server 可能在处理中已关闭，不算错误
		return 0
	}
	defer resp.Body.Close()

	var sessionResp database.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return 0
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

// isServerReady 检测 server 是否就绪
func isServerReady(client *http.Client, healthURL string) bool {
	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitForServer 轮询等待 server 就绪
func waitForServer(client *http.Client, healthURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isServerReady(client, healthURL) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
