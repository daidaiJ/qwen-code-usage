// Package database 数据库模型和接口
package database

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	ID                 int64
	SessionID          string
	ModelName          string
	RequestSeq         int
	LatencyMs          int
	PromptTokens       int
	CompletionTokens   int
	CachedTokens       int
	ThoughtsTokens     int
	TotalTokens        int
	ContextWindowSize  int
	CurrentUsage       int
	RecordedAt         time.Time
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
	TokensPerSec     float64 // 未命中缓存 token 吞吐量 (prompt-cached+completion) / 延迟秒数
}

// StatsResponse stats 响应
type StatsResponse struct {
	Models           []ModelStats           `json:"models"`
	Total            TotalStats             `json:"total"`
	ContextWindowMax *ContextWindowExtremes `json:"context_window_max,omitempty"`
	Period           string                 `json:"period"`
	StartTime        string                 `json:"start_time"`
	EndTime          string                 `json:"end_time"`
}

// ContextWindowExtremes 上下文窗口历史最值（从 call_records 查询结果）
type ContextWindowExtremes struct {
	MaxCurrentUsage       int       `json:"max_current_usage"`
	MaxCurrentUsageModel  string    `json:"max_current_usage_model"`
	MaxCurrentUsageTime   time.Time `json:"max_current_usage_time"`
	MaxSingleInputTokens  int       `json:"max_single_input_tokens"`
	MaxSingleInputModel   string    `json:"max_single_input_model"`
	MaxSingleInputTime    time.Time `json:"max_single_input_time"`
	MaxSingleOutputTokens int       `json:"max_single_output_tokens"`
	MaxSingleOutputModel  string    `json:"max_single_output_model"`
	MaxSingleOutputTime   time.Time `json:"max_single_output_time"`
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

// StreamEvent 流格式事件（来自 stdin 的 JSON 行）
type StreamEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	UUID    string          `json:"uuid,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Event   json.RawMessage `json:"event,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Request json.RawMessage `json:"request,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
}

// StreamSessionStart session_start 事件数据
type StreamSessionStart struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
}

// StreamMessage message 结构（用于 user/assistant 类型）
type StreamMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Usage   *StreamUsage    `json:"usage,omitempty"`
}

// StreamUsage token 用量
type StreamUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// StreamMessageStart message_start 事件
type StreamMessageStart struct {
	Type    string        `json:"type"`
	Message StreamMessage `json:"message"`
}

// StreamContentBlockDelta content_block_delta 事件
type StreamContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// FormatContextWindowSize 将 context_window_size 格式化为可读字符串
// >= 1000000 显示为 "1M"、"2M"；< 1000000 按 1024 算 K，如 "128K"、"200K"
// size <= 0 时返回空字符串，调用方可据此决定是否显示
func FormatContextWindowSize(size int) string {
	if size <= 0 {
		return ""
	}
	if size >= 1000000 {
		return fmt.Sprintf("%dM", size/1000000)
	}
	return fmt.Sprintf("%dK", size/1024)
}

// StreamMetrics 流式请求的延迟和吞吐量统计
type StreamMetrics struct {
	SessionID      string
	ModelName      string
	RequestSeq     int
	StartTimestamp time.Time
	EndTimestamp   time.Time
	LatencyMs      int64
	InputTokens    int
	OutputTokens   int
	CachedTokens   int
	TotalTokens    int
	TokensPerSec   float64
}
