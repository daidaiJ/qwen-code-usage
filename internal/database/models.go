// Package database 数据库模型和接口
package database

import "time"

// StatusLineInput Qwen Code status line 输入结构
type StatusLineInput struct {
	SessionID string `json:"session_id"`
	Version   string `json:"version"`
	Model     struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		ContextWindowSize   int     `json:"context_window_size"`
		UsedPercentage      float64 `json:"used_percentage"`
		RemainingPercentage float64 `json:"remaining_percentage"`
		CurrentUsage        int     `json:"current_usage"`
		TotalInputTokens    int     `json:"total_input_tokens"`
		TotalOutputTokens   int     `json:"total_output_tokens"`
	} `json:"context_window"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Git struct {
		Branch string `json:"branch"`
	} `json:"git"`
	Metrics struct {
		Models map[string]ModelMetrics `json:"models"`
		Files  struct {
			TotalLinesAdded   int `json:"total_lines_added"`
			TotalLinesRemoved int `json:"total_lines_removed"`
		} `json:"files"`
	} `json:"metrics"`
	Vim struct {
		Mode string `json:"mode"`
	} `json:"vim"`
}

// ModelMetrics 单个模型的指标
type ModelMetrics struct {
	API struct {
		TotalRequests  int `json:"total_requests"`
		TotalErrors    int `json:"total_errors"`
		TotalLatencyMs int `json:"total_latency_ms"`
	} `json:"api"`
	Tokens struct {
		Prompt     int `json:"prompt"`
		Completion int `json:"completion"`
		Total      int `json:"total"`
		Cached     int `json:"cached"`
		Thoughts   int `json:"thoughts"`
	} `json:"tokens"`
}

// CumulativeState 累计状态（用于计算增量）
type CumulativeState struct {
	SessionID        string
	ModelName        string
	TotalRequests    int
	TotalLatencyMs   int
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	ThoughtsTokens   int
	TotalTokens      int
}

// CallRecord 单次调用记录
type CallRecord struct {
	ID               int64
	SessionID        string
	ModelName        string
	RequestSeq       int
	LatencyMs        int
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	ThoughtsTokens   int
	TotalTokens      int
	RecordedAt       time.Time
}

// RecordRequest record 请求
type RecordRequest struct {
	SessionID string       `json:"session_id"`
	ModelName string       `json:"model_name"`
	Metrics   ModelMetrics `json:"metrics"`
	Context   ContextInfo  `json:"context"`
}

// ContextInfo 上下文信息
type ContextInfo struct {
	UsedPercentage float64 `json:"used_percentage"`
	TotalTokens    int     `json:"total_tokens"`
}

// RecordResponse record 响应
type RecordResponse struct {
	StatusLine string `json:"status_line"`
	Recorded   bool   `json:"recorded"`
	Error      string `json:"error,omitempty"`
}

// StatsRequest stats 请求
type StatsRequest struct {
	Period string `json:"period"` // day, week, month, 5h
	From   string `json:"from"`   // YYYY-MM-DD
	To     string `json:"to"`     // YYYY-MM-DD
}

// ModelStats 模型统计
type ModelStats struct {
	ModelName        string
	RequestCount     int
	TotalLatencyMs   int
	AvgLatencyMs     float64
	P50LatencyMs     float64
	P95LatencyMs     float64
	PromptTokens     int64
	CompletionTokens int64
	CachedTokens     int64
	ThoughtsTokens   int64
	TotalTokens      int64
	CachePercent     float64
}

// StatsResponse stats 响应
type StatsResponse struct {
	Models    []ModelStats `json:"models"`
	Total     TotalStats   `json:"total"`
	Period    string       `json:"period"`
	StartTime string       `json:"start_time"`
	EndTime   string       `json:"end_time"`
}

// TotalStats 汇总统计
type TotalStats struct {
	RequestCount int     `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// SessionResponse 会话计数响应
type SessionResponse struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}