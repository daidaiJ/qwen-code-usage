package mocks

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/panda/qwen-usage/internal/database"
)

func TestMockDBBasicOperations(t *testing.T) {
	db := NewMockDB()

	// 测试初始状态
	state, err := db.GetCumulativeState("test-session", "qwen-3-235b")
	if err != nil {
		t.Fatalf("GetCumulativeState failed: %v", err)
	}

	if state.TotalRequests != 0 {
		t.Errorf("expected initial TotalRequests to be 0, got %d", state.TotalRequests)
	}

	// 验证调用记录
	if len(db.GetCumulativeStateCalls) != 1 {
		t.Errorf("expected 1 call, got %d", len(db.GetCumulativeStateCalls))
	}
	if db.GetCumulativeStateCalls[0].SessionID != "test-session" {
		t.Errorf("expected SessionID to be test-session, got %s", db.GetCumulativeStateCalls[0].SessionID)
	}

	// 测试更新状态
	newState := &database.CumulativeState{
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

	// 验证更新后的状态
	state, err = db.GetCumulativeState("test-session", "qwen-3-235b")
	if err != nil {
		t.Fatalf("GetCumulativeState after update failed: %v", err)
	}

	if state.TotalRequests != 10 {
		t.Errorf("expected TotalRequests to be 10, got %d", state.TotalRequests)
	}

	// 测试插入记录
	record := &database.CallRecord{
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
		t.Error("expected record ID to be set")
	}

	if len(db.InsertCallRecordCalls) != 1 {
		t.Errorf("expected 1 insert call, got %d", len(db.InsertCallRecordCalls))
	}

	// 测试统计
	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now().Add(1 * time.Hour)

	stats, err := db.GetStats(startTime, endTime)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.Total.RequestCount != 1 {
		t.Errorf("expected RequestCount to be 1, got %d", stats.Total.RequestCount)
	}
}

func TestMockDBErrorHandling(t *testing.T) {
	db := NewMockDB()

	// 设置错误
	db.GetCumulativeStateErr = &database.MockError{Msg: "not found"}

	_, err := db.GetCumulativeState("test-session", "qwen-3-235b")
	if err == nil {
		t.Error("expected error but got nil")
	}

	// 重置
	db.Reset()

	// 验证重置后正常工作
	_, err = db.GetCumulativeState("test-session", "qwen-3-235b")
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestMockDBDeleteOldRecords(t *testing.T) {
	db := NewMockDB()

	// 添加两条记录：一条旧的，一条新的
	oldRecord := database.CallRecord{
		SessionID:  "test",
		ModelName:  "qwen",
		RecordedAt: time.Now().Add(-2 * 24 * time.Hour), // 2天前
	}
	newRecord := database.CallRecord{
		SessionID:  "test",
		ModelName:  "qwen",
		RecordedAt: time.Now().Add(-1 * time.Hour), // 1小时前
	}

	db.AddRecord(oldRecord)
	db.AddRecord(newRecord)

	// 删除1天前的记录
	beforeDate := time.Now().Add(-24 * time.Hour)
	deleted, err := db.DeleteOldRecords(beforeDate)
	if err != nil {
		t.Fatalf("DeleteOldRecords failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted record, got %d", deleted)
	}

	// 验证剩余记录
	records, _ := db.GetCallRecords(time.Now().Add(-2*time.Hour), time.Now())
	if len(records) != 1 {
		t.Errorf("expected 1 remaining record, got %d", len(records))
	}
}

func TestMockHTTPClient(t *testing.T) {
	client := NewMockHTTPClient()

	// 创建测试请求
	req, _ := http.NewRequest("POST", "/record", bytes.NewReader([]byte(`{"test": true}`)))

	// 设置响应
	client.SetResponse(200, map[string]string{"status": "ok"})

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", resp.StatusCode)
	}

	// 验证请求被记录
	if len(client.Requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(client.Requests))
	}
	if client.Requests[0].URL != "/record" {
		t.Errorf("expected URL to be /record, got %s", client.Requests[0].URL)
	}
}

func TestMockHTTPClientURLMapping(t *testing.T) {
	client := NewMockHTTPClient()

	// 为不同URL设置不同响应
	client.SetResponseForURL("/record", 200, database.RecordResponse{
		StatusLine: "test-status",
		Recorded:   true,
	})

	client.SetResponseForURL("/session/start", 200, database.SessionResponse{
		Count:   1,
		Message: "started",
	})

	// 测试 /record
	req1, _ := http.NewRequest("POST", "/record", nil)
	resp1, _ := client.Do(req1)
	if resp1.StatusCode != 200 {
		t.Errorf("expected 200 for /record, got %d", resp1.StatusCode)
	}

	// 测试 /session/start
	req2, _ := http.NewRequest("POST", "/session/start", nil)
	resp2, _ := client.Do(req2)
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 for /session/start, got %d", resp2.StatusCode)
	}
}

func TestMockHTTPClientReset(t *testing.T) {
	client := NewMockHTTPClient()

	// 发送请求
	req, _ := http.NewRequest("GET", "/test", nil)
	client.Do(req)

	// 设置响应
	client.SetResponse(500, nil)

	// 重置
	client.Reset()

	// 验证重置
	if len(client.Requests) != 0 {
		t.Errorf("expected 0 requests after reset, got %d", len(client.Requests))
	}
	if client.Response != nil {
		t.Error("expected Response to be nil after reset")
	}
}