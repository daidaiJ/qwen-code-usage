package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panda/qwen-usage/internal/config"
)

func TestCumulativeState(t *testing.T) {
	state := &CumulativeState{
		SessionID:         "test-session",
		ModelName:         "qwen-3-235b",
		TotalRequests:     10,
		TotalLatencyMs:    1200,
		PromptTokens:      5000,
		CompletionTokens:  1000,
		CachedTokens:      2000,
		ThoughtsTokens:    500,
		TotalTokens:       6500,
	}

	if state.SessionID != "test-session" {
		t.Errorf("expected SessionID to be test-session, got %s", state.SessionID)
	}
	if state.TotalRequests != 10 {
		t.Errorf("expected TotalRequests to be 10, got %d", state.TotalRequests)
	}
}

func TestCallRecord(t *testing.T) {
	record := CallRecord{
		ID:               1,
		SessionID:        "test-session",
		ModelName:        "qwen-3-235b",
		RequestSeq:       5,
		LatencyMs:        120,
		PromptTokens:     500,
		CompletionTokens: 100,
		CachedTokens:     200,
		ThoughtsTokens:   50,
		TotalTokens:      650,
		RecordedAt:       time.Now(),
	}

	if record.SessionID != "test-session" {
		t.Errorf("expected SessionID to be test-session, got %s", record.SessionID)
	}
	if record.RequestSeq != 5 {
		t.Errorf("expected RequestSeq to be 5, got %d", record.RequestSeq)
	}
}

func TestSQLiteDBCRUD(t *testing.T) {
	// 创建临时数据库
	tmpDir := filepath.Join(os.TempDir(), "qwen-usage-db-test")
	_ = os.MkdirAll(tmpDir, 0755)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "test.db")

	// 设置测试配置
	config.ResetConfig()
	config.SetConfig(&config.Config{
		ServerAddr:           "127.0.0.1:9527",
		DBPath:               dbPath,
		ClientTimeoutMs:      100,
		ServerWriteTimeoutMs: 50,
	})

	ResetDB()

	db, err := NewSQLiteDBWithPath(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// 测试 GetCumulativeState（不存在）
	state, err := db.GetCumulativeState("test-session", "qwen-3-235b")
	if err != nil {
		t.Fatalf("GetCumulativeState failed: %v", err)
	}

	if state.TotalRequests != 0 {
		t.Errorf("expected initial TotalRequests to be 0, got %d", state.TotalRequests)
	}

	// 测试 UpdateCumulativeState
	newState := &CumulativeState{
		SessionID:         "test-session",
		ModelName:         "qwen-3-235b",
		TotalRequests:     10,
		TotalLatencyMs:    1200,
		PromptTokens:      5000,
		CompletionTokens:  1000,
		CachedTokens:      2000,
		ThoughtsTokens:    500,
		TotalTokens:       6500,
	}

	err = db.UpdateCumulativeState(newState)
	if err != nil {
		t.Fatalf("UpdateCumulativeState failed: %v", err)
	}

	// 验证更新
	state, err = db.GetCumulativeState("test-session", "qwen-3-235b")
	if err != nil {
		t.Fatalf("GetCumulativeState after update failed: %v", err)
	}

	if state.TotalRequests != 10 {
		t.Errorf("expected TotalRequests to be 10, got %d", state.TotalRequests)
	}

	// 测试 InsertCallRecord
	record := &CallRecord{
		SessionID:        "test-session",
		ModelName:        "qwen-3-235b",
		RequestSeq:       1,
		LatencyMs:        120,
		PromptTokens:     500,
		CompletionTokens: 100,
		CachedTokens:     200,
		ThoughtsTokens:   50,
		TotalTokens:      650,
	}

	err = db.InsertCallRecord(record)
	if err != nil {
		t.Fatalf("InsertCallRecord failed: %v", err)
	}

	if record.ID == 0 {
		t.Error("expected record ID to be set after insert")
	}

	// 测试 GetCallRecords
	startTime := record.RecordedAt.Add(-1 * time.Hour)
	endTime := record.RecordedAt.Add(1 * time.Hour)

	records, err := db.GetCallRecords(startTime, endTime)
	if err != nil {
		t.Fatalf("GetCallRecords failed: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}

	// 测试 GetStats
	stats, err := db.GetStats(startTime, endTime)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.Total.RequestCount != 1 {
		t.Errorf("expected RequestCount to be 1, got %d", stats.Total.RequestCount)
	}

	// 测试 DeleteOldRecords
	oldTime := record.RecordedAt.Add(1 * time.Hour) // 未来时间，会删除
	deleted, err := db.DeleteOldRecords(oldTime)
	if err != nil {
		t.Fatalf("DeleteOldRecords failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// 验证删除后记录数
	records, _ = db.GetCallRecords(record.RecordedAt.Add(-1*time.Hour), record.RecordedAt.Add(1*time.Hour))
	if len(records) != 0 {
		t.Errorf("expected 0 records after delete, got %d", len(records))
	}
}

func TestModelMetrics(t *testing.T) {
	metrics := ModelMetrics{
		API: struct {
			TotalRequests  int `json:"total_requests"`
			TotalErrors    int `json:"total_errors"`
			TotalLatencyMs int `json:"total_latency_ms"`
		}{
			TotalRequests:  5,
			TotalErrors:    0,
			TotalLatencyMs: 500,
		},
		Tokens: struct {
			Prompt     int `json:"prompt"`
			Completion int `json:"completion"`
			Total      int `json:"total"`
			Cached     int `json:"cached"`
			Thoughts   int `json:"thoughts"`
		}{
			Prompt:     1000,
			Completion: 200,
			Total:      1200,
			Cached:     300,
			Thoughts:   50,
		},
	}

	if metrics.API.TotalRequests != 5 {
		t.Errorf("expected TotalRequests to be 5, got %d", metrics.API.TotalRequests)
	}
	if metrics.Tokens.Prompt != 1000 {
		t.Errorf("expected Prompt tokens to be 1000, got %d", metrics.Tokens.Prompt)
	}
}

func TestStatsResponse(t *testing.T) {
	stats := StatsResponse{
		Models: []ModelStats{
			{
				ModelName:        "qwen-3-235b",
				RequestCount:     10,
				AvgLatencyMs:     120.5,
				PromptTokens:     5000,
				CompletionTokens: 1000,
				CachedTokens:     2000,
				TotalTokens:      6500,
				CachePercent:     40.0,
			},
		},
		Total: TotalStats{
			RequestCount: 10,
			TotalTokens:  6500,
			AvgLatencyMs: 120.5,
		},
		StartTime: "2026-04-25 00:00:00",
		EndTime:   "2026-04-25 23:59:59",
	}

	if len(stats.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(stats.Models))
	}
	if stats.Total.RequestCount != 10 {
		t.Errorf("expected RequestCount to be 10, got %d", stats.Total.RequestCount)
	}
}

func TestContextWindowExtremes(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "qwen-usage-extremes-test")
	_ = os.MkdirAll(tmpDir, 0755)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "test.db")

	config.ResetConfig()
	config.SetConfig(&config.Config{
		ServerAddr:           "127.0.0.1:9527",
		DBPath:               dbPath,
		ClientTimeoutMs:      100,
		ServerWriteTimeoutMs: 50,
	})

	ResetDB()

	db, err := NewSQLiteDBWithPath(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Now()

	// 插入第一条记录
	err = db.InsertCallRecord(&CallRecord{
		SessionID:         "s1",
		ModelName:         "qwen-3-235b",
		RequestSeq:        1,
		LatencyMs:         100,
		PromptTokens:      50000,
		CompletionTokens:  10000,
		CachedTokens:      0,
		ThoughtsTokens:    0,
		TotalTokens:       60000,
		ContextWindowSize: 128000,
		CurrentUsage:      20275,
	})
	if err != nil {
		t.Fatalf("InsertCallRecord failed: %v", err)
	}

	// 插入第二条记录（更小的 input，更大的 output）
	err = db.InsertCallRecord(&CallRecord{
		SessionID:         "s1",
		ModelName:         "qwen-coder",
		RequestSeq:        2,
		LatencyMs:         200,
		PromptTokens:      30000,
		CompletionTokens:  20000,
		CachedTokens:      0,
		ThoughtsTokens:    0,
		TotalTokens:       50000,
		ContextWindowSize: 200000,
		CurrentUsage:      15000,
	})
	if err != nil {
		t.Fatalf("InsertCallRecord failed: %v", err)
	}

	// 查询最值
	extremes, err := db.GetContextWindowExtremes(now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("GetContextWindowExtremes failed: %v", err)
	}

	// prompt_tokens=50000 > 30000，取第一条记录的模型
	if extremes.MaxSingleInputTokens != 50000 {
		t.Errorf("expected MaxSingleInputTokens 50000, got %d", extremes.MaxSingleInputTokens)
	}
	if extremes.MaxSingleInputModel != "qwen-3-235b" {
		t.Errorf("expected MaxSingleInputModel 'qwen-3-235b', got '%s'", extremes.MaxSingleInputModel)
	}

	// completion_tokens=20000 > 10000，取第二条记录的模型
	if extremes.MaxSingleOutputTokens != 20000 {
		t.Errorf("expected MaxSingleOutputTokens 20000, got %d", extremes.MaxSingleOutputTokens)
	}
	if extremes.MaxSingleOutputModel != "qwen-coder" {
		t.Errorf("expected MaxSingleOutputModel 'qwen-coder', got '%s'", extremes.MaxSingleOutputModel)
	}

	// MAX(prompt_tokens + completion_tokens) = 50000+10000 = 60000
	if extremes.MaxCurrentUsage != 60000 {
		t.Errorf("expected MaxCurrentUsage 60000, got %d", extremes.MaxCurrentUsage)
	}
	if extremes.MaxCurrentUsageModel != "qwen-3-235b" {
		t.Errorf("expected MaxCurrentUsageModel 'qwen-3-235b', got '%s'", extremes.MaxCurrentUsageModel)
	}

	// 空时间范围应返回全 0
	empty, err := db.GetContextWindowExtremes(now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GetContextWindowExtremes empty range failed: %v", err)
	}
	if empty.MaxCurrentUsage != 0 || empty.MaxSingleInputTokens != 0 || empty.MaxSingleOutputTokens != 0 {
		t.Errorf("expected empty extremes, got usage=%d input=%d output=%d",
			empty.MaxCurrentUsage, empty.MaxSingleInputTokens, empty.MaxSingleOutputTokens)
	}
}