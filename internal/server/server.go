// Package server HTTP服务
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/panda/qwen-usage/internal/config"
	"github.com/panda/qwen-usage/internal/database"
	"github.com/panda/qwen-usage/internal/logger"
	"github.com/panda/qwen-usage/pkg/platform"
)

const version = "1.0.0"

// Server HTTP 服务
type Server struct {
	db           database.DB
	server       *http.Server
	sessionMu    sync.Mutex
	sessionCount int
}

// NewServer 创建服务器
func NewServer(addr string, db database.DB) *Server {
	s := &Server{
		db: db,
		server: &http.Server{
			Addr:         addr,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: time.Duration(config.GetConfig().ServerWriteTimeoutMs) * time.Millisecond,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/record", s.handleRecord)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/shutdown", s.handleShutdown)
	mux.HandleFunc("/session/start", s.handleSessionStart)
	mux.HandleFunc("/session/end", s.handleSessionEnd)

	s.server.Handler = s.recoveryMiddleware(mux)

	return s
}

// recoveryMiddleware panic恢复中间件
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.LogError("panic recovered in handler: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// handleRecord 处理record请求
func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input database.StatusLineInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	var statusLine string
	recorded := false
	var errMsg string

	for modelName, metrics := range input.Metrics.Models {
		if metrics.API.TotalRequests == 0 {
			continue
		}

		err := s.processModelMetrics(input.SessionID, modelName, metrics)
		if err != nil {
			errMsg = err.Error()
			logger.LogWarn("failed to process metrics for model %s: %v", modelName, err)
		} else {
			recorded = true
			logger.LogDebug("recorded metrics for model %s, seq=%d", modelName, metrics.API.TotalRequests)
		}

		if statusLine == "" {
			statusLine = formatStatusLine(input, modelName, metrics)
		}
	}

	if statusLine == "" {
		modelName := input.Model.DisplayName
		if modelName == "" {
			modelName = "unknown"
		}
		statusLine = fmt.Sprintf("model: %s | ctx:%.1f%%", modelName, input.ContextWindow.UsedPercentage)
	}

	// 更新上下文窗口历史最值（上下文窗口 = 输入 + 输出 tokens）
	inputOutputSum := input.ContextWindow.TotalInputTokens + input.ContextWindow.TotalOutputTokens
	if inputOutputSum > 0 {
		extremes := &database.ContextWindowExtremes{
			MaxContextWindowSize: inputOutputSum,
			MaxTotalInputTokens:  input.ContextWindow.TotalInputTokens,
			MaxTotalOutputTokens: input.ContextWindow.TotalOutputTokens,
		}
		if err := s.db.UpdateContextWindowExtremes(extremes); err != nil {
			logger.LogWarn("failed to update context window extremes: %v", err)
		}
	}

	resp := database.RecordResponse{
		StatusLine: statusLine,
		Recorded:   recorded,
		Error:      errMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// processModelMetrics 处理单个模型的指标
func (s *Server) processModelMetrics(sessionID, modelName string, metrics database.ModelMetrics) error {
	prevState, err := s.db.GetCumulativeState(sessionID, modelName)
	if err != nil {
		return fmt.Errorf("failed to get cumulative state: %w", err)
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

	// 确保增量非负
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

	if err := s.db.InsertCallRecord(record); err != nil {
		return fmt.Errorf("failed to insert call record: %w", err)
	}

	newState := &database.CumulativeState{
		SessionID:         sessionID,
		ModelName:         modelName,
		TotalRequests:     metrics.API.TotalRequests,
		TotalLatencyMs:    metrics.API.TotalLatencyMs,
		PromptTokens:      metrics.Tokens.Prompt,
		CompletionTokens:  metrics.Tokens.Completion,
		CachedTokens:      metrics.Tokens.Cached,
		ThoughtsTokens:    metrics.Tokens.Thoughts,
		TotalTokens:       metrics.Tokens.Total,
	}

	if err := s.db.UpdateCumulativeState(newState); err != nil {
		return fmt.Errorf("failed to update cumulative state: %w", err)
	}

	return nil
}

// formatStatusLine 格式化状态行
func formatStatusLine(input database.StatusLineInput, modelName string, metrics database.ModelMetrics) string {
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

// handleStats 处理stats请求
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	startTime, endTime := getPeriodRange(period)

	stats, err := s.db.GetStats(startTime, endTime)
	if err != nil {
		http.Error(w, "failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// handleSessionStart 会话开始
func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.sessionMu.Lock()
	s.sessionCount++
	count := s.sessionCount
	s.sessionMu.Unlock()

	logger.LogInfo("session started, count=%d", count)

	resp := database.SessionResponse{
		Count:   count,
		Message: "session started",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleSessionEnd 会话结束
func (s *Server) handleSessionEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.sessionMu.Lock()
	s.sessionCount--
	if s.sessionCount < 0 {
		s.sessionCount = 0
	}
	count := s.sessionCount
	s.sessionMu.Unlock()

	logger.LogInfo("session ended, count=%d", count)

	resp := database.SessionResponse{
		Count:   count,
		Message: "session ended",
	}

	if count == 0 {
		resp.Message = "session ended, server shutting down"
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.Stop()
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleShutdown 强制关闭
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logger.LogInfo("shutdown requested via HTTP")
	fmt.Fprint(w, "OK")
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Stop()
	}()
}

// getPeriodRange 根据周期获取时间范围
func getPeriodRange(period string) (start, end time.Time) {
	end = time.Now()
	switch period {
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

// Start 启动服务器
func (s *Server) Start() error {
	logger.LogInfo("server starting on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop() error {
	logger.LogInfo("server stopping...")
	return s.server.Close()
}

// RunServer 运行服务器（主入口）
func RunServer() error {
	cfg := config.GetConfig()

	// 初始化日志
	lg := logger.GetLogger()
	defer lg.Close()

	logger.LogInfo("qwen-usage server v%s starting", version)

	// 写入 PID 文件
	if err := platform.WritePIDFile(); err != nil {
		logger.LogError("failed to write pid file: %v", err)
		return err
	}
	defer platform.RemovePIDFile()
	logger.LogInfo("pid file written: %s", platform.GetPIDFilePath())

	db, err := database.GetDB()
	if err != nil {
		logger.LogError("failed to init database: %v", err)
		return err
	}
	logger.LogInfo("database initialized: %s", cfg.DBPath)

	server := NewServer(cfg.ServerAddr, db)

	// 优雅关闭
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		logger.LogInfo("shutdown signal received, cleaning up...")
		server.Stop()
		db.Close()
		lg.Close()
		platform.RemovePIDFile()
		logger.LogInfo("server shutdown complete")
		os.Exit(0)
	}()

	return server.Start()
}

// GetVersion 获取版本号
func GetVersion() string {
	return version
}