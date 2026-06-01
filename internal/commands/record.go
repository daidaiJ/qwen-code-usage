// Package commands 命令实现
package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/panda/qwen-usage/internal/config"
	"github.com/panda/qwen-usage/internal/database"
)

// RunRecord 执行record命令
// statusLineOnly: true 时只返回状态行，不记录到数据库（用于 -s 模式，后续由 stream 命令记录详情）
func RunRecord(statusLineOnly bool) int {
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

	// status_line 模式：只返回状态行，不记录到数据库
	if statusLineOnly {
		var statusLine string
		for mName, metrics := range statusInput.Metrics.Models {
			statusLine = formatStatusLineLocal(statusInput, mName, metrics)
			break // 只取第一个模型
		}
		if statusLine == "" {
			statusLine = formatStatusLineLocal(statusInput, "", database.ModelMetrics{})
		}
		fmt.Println(statusLine)
		return 0
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
			defer func() { _ = resp.Body.Close() }()
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
		defer func() { _ = db.Close() }()

		for mName, metrics := range input.Metrics.Models {
			if metrics.API.TotalRequests == 0 {
				continue
			}

			contextUsage := input.ContextWindow.CurrentUsage
			if contextUsage == 0 {
				contextUsage = input.ContextWindow.TotalInputTokens + input.ContextWindow.TotalOutputTokens
			}
			_, _, err := processMetricsLocal(db, input.SessionID, mName, metrics,
				input.ContextWindow.ContextWindowSize, contextUsage)
			if err == nil {
				recorded = true
			}

			if statusLine == "" {
				statusLine = formatStatusLineLocal(input, mName, metrics)
			}
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
// 返回单次请求的 input/output delta 值
func processMetricsLocal(db database.DB, sessionID, modelName string, metrics database.ModelMetrics, contextWindowSize, currentUsage int) (deltaInput, deltaOutput int, err error) {
	prevState, err := db.GetCumulativeState(sessionID, modelName)
	if err != nil {
		return 0, 0, err
	}

	deltaRequests := metrics.API.TotalRequests - prevState.TotalRequests
	if deltaRequests <= 0 {
		return 0, 0, nil
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
		SessionID:         sessionID,
		ModelName:         modelName,
		RequestSeq:        metrics.API.TotalRequests,
		LatencyMs:         singleLatencyMs,
		PromptTokens:      deltaPrompt,
		CompletionTokens:  deltaCompletion,
		CachedTokens:      deltaCached,
		ThoughtsTokens:    deltaThoughts,
		TotalTokens:       deltaTotal,
		ContextWindowSize: contextWindowSize,
		CurrentUsage:      currentUsage,
	}

	if err := db.InsertCallRecord(record); err != nil {
		return 0, 0, err
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

	if err := db.UpdateCumulativeState(newState); err != nil {
		return 0, 0, err
	}

	return deltaPrompt, deltaCompletion, nil
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
