// Package mocks Mock实现
package mocks

import (
	"sync"
	"time"

	"github.com/panda/qwen-usage/internal/database"
)

// MockDB Mock数据库实现
type MockDB struct {
	mu sync.RWMutex

	// 存储数据
	cumulativeStates map[string]*database.CumulativeState
	callRecords      []database.CallRecord

	// 记录调用
	GetCumulativeStateCalls []struct {
		SessionID string
		ModelName string
	}
	UpdateCumulativeStateCalls []*database.CumulativeState
	InsertCallRecordCalls      []*database.CallRecord
	DeleteOldRecordsCalls      []time.Time
	GetStatsCalls              []struct {
		Start time.Time
		End   time.Time
	}
	CloseCalls int

	// 可配置的错误返回
	GetCumulativeStateErr    error
	UpdateCumulativeStateErr error
	InsertCallRecordErr      error
	DeleteOldRecordsErr      error
	GetStatsErr              error
	CloseErr                 error
}

// NewMockDB 创建MockDB
func NewMockDB() *MockDB {
	return &MockDB{
		cumulativeStates: make(map[string]*database.CumulativeState),
		callRecords:      []database.CallRecord{},
	}
}

// cacheKey 生成缓存键
func cacheKey(sessionID, modelName string) string {
	return sessionID + "|" + modelName
}

// GetCumulativeState 获取累计状态
func (m *MockDB) GetCumulativeState(sessionID, modelName string) (*database.CumulativeState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetCumulativeStateCalls = append(m.GetCumulativeStateCalls, struct {
		SessionID string
		ModelName string
	}{SessionID: sessionID, ModelName: modelName})

	if m.GetCumulativeStateErr != nil {
		return nil, m.GetCumulativeStateErr
	}

	key := cacheKey(sessionID, modelName)
	if state, ok := m.cumulativeStates[key]; ok {
		return state, nil
	}

	// 返回空状态
	return &database.CumulativeState{
		SessionID: sessionID,
		ModelName: modelName,
	}, nil
}

// UpdateCumulativeState 更新累计状态
func (m *MockDB) UpdateCumulativeState(state *database.CumulativeState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateCumulativeStateCalls = append(m.UpdateCumulativeStateCalls, state)

	if m.UpdateCumulativeStateErr != nil {
		return m.UpdateCumulativeStateErr
	}

	key := cacheKey(state.SessionID, state.ModelName)
	m.cumulativeStates[key] = state
	return nil
}

// InsertCallRecord 插入调用记录
func (m *MockDB) InsertCallRecord(record *database.CallRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.InsertCallRecordCalls = append(m.InsertCallRecordCalls, record)

	if m.InsertCallRecordErr != nil {
		return m.InsertCallRecordErr
	}

	// 设置记录时间（如果未设置）
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now()
	}

	record.ID = int64(len(m.callRecords) + 1)
	m.callRecords = append(m.callRecords, *record)
	return nil
}

// GetCallRecords 获取调用记录
func (m *MockDB) GetCallRecords(startTime, endTime time.Time) ([]database.CallRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var records []database.CallRecord
	for i, r := range m.callRecords {
		recordedAt := m.callRecords[i].RecordedAt
		if recordedAt.Compare(startTime) >= 0 && recordedAt.Compare(endTime) <= 0 {
			records = append(records, r)
		}
	}
	return records, nil
}

// DeleteOldRecords 删除旧记录
func (m *MockDB) DeleteOldRecords(beforeDate time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteOldRecordsCalls = append(m.DeleteOldRecordsCalls, beforeDate)

	if m.DeleteOldRecordsErr != nil {
		return 0, m.DeleteOldRecordsErr
	}

	var deleted int64
	var remaining []database.CallRecord
	for _, r := range m.callRecords {
		if r.RecordedAt.Compare(beforeDate) < 0 {
			deleted++
		} else {
			remaining = append(remaining, r)
		}
	}
	m.callRecords = remaining
	return deleted, nil
}

// GetStats 获取统计
func (m *MockDB) GetStats(startTime, endTime time.Time) (*database.StatsResponse, error) {
	m.mu.Lock()
	m.GetStatsCalls = append(m.GetStatsCalls, struct {
		Start time.Time
		End   time.Time
	}{Start: startTime, End: endTime})
	m.mu.Unlock()

	if m.GetStatsErr != nil {
		return nil, m.GetStatsErr
	}

	// 先获取记录（不持有锁）
	records, err := m.getCallRecordsInternal(startTime, endTime)
	if err != nil {
		return nil, err
	}

	modelStatsMap := make(map[string]*database.ModelStats)
	var totalTokens int64
	var totalLatency int

	for _, r := range records {
		stats, ok := modelStatsMap[r.ModelName]
		if !ok {
			stats = &database.ModelStats{ModelName: r.ModelName}
			modelStatsMap[r.ModelName] = stats
		}

		stats.RequestCount++
		stats.PromptTokens += int64(r.PromptTokens)
		stats.CompletionTokens += int64(r.CompletionTokens)
		stats.CachedTokens += int64(r.CachedTokens)
		stats.ThoughtsTokens += int64(r.ThoughtsTokens)
		stats.TotalTokens += int64(r.TotalTokens)
		stats.TotalLatencyMs += r.LatencyMs

		totalTokens += int64(r.TotalTokens)
		totalLatency += r.LatencyMs
	}

	for _, stats := range modelStatsMap {
		if stats.RequestCount > 0 {
			stats.AvgLatencyMs = float64(stats.TotalLatencyMs) / float64(stats.RequestCount)
			if stats.PromptTokens > 0 {
				stats.CachePercent = float64(stats.CachedTokens) / float64(stats.PromptTokens) * 100
			}
		}
	}

	var models []database.ModelStats
	for _, stats := range modelStatsMap {
		models = append(models, *stats)
	}

	var avgLatency float64
	if len(records) > 0 {
		avgLatency = float64(totalLatency) / float64(len(records))
	}

	return &database.StatsResponse{
		Models:    models,
		StartTime: startTime.Format("2006-01-02 15:04:05"),
		EndTime:   endTime.Format("2006-01-02 15:04:05"),
		Total: database.TotalStats{
			RequestCount: len(records),
			TotalTokens:  totalTokens,
			AvgLatencyMs: avgLatency,
		},
	}, nil
}

// getCallRecordsInternal 内部方法，获取记录
func (m *MockDB) getCallRecordsInternal(startTime, endTime time.Time) ([]database.CallRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var records []database.CallRecord
	for i, r := range m.callRecords {
		recordedAt := m.callRecords[i].RecordedAt
		if recordedAt.Compare(startTime) >= 0 && recordedAt.Compare(endTime) <= 0 {
			records = append(records, r)
		}
	}
	return records, nil
}

// Close 关闭连接
func (m *MockDB) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CloseCalls++
	return m.CloseErr
}

// Reset 重置mock状态
func (m *MockDB) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cumulativeStates = make(map[string]*database.CumulativeState)
	m.callRecords = []database.CallRecord{}
	m.GetCumulativeStateCalls = nil
	m.UpdateCumulativeStateCalls = nil
	m.InsertCallRecordCalls = nil
	m.DeleteOldRecordsCalls = nil
	m.GetStatsCalls = nil
	m.CloseCalls = 0

	m.GetCumulativeStateErr = nil
	m.UpdateCumulativeStateErr = nil
	m.InsertCallRecordErr = nil
	m.DeleteOldRecordsErr = nil
	m.GetStatsErr = nil
	m.CloseErr = nil
}

// AddRecord 添加测试数据
func (m *MockDB) AddRecord(record database.CallRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record.ID = int64(len(m.callRecords) + 1)
	m.callRecords = append(m.callRecords, record)
}

// SetCumulativeState 设置累计状态
func (m *MockDB) SetCumulativeState(state *database.CumulativeState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := cacheKey(state.SessionID, state.ModelName)
	m.cumulativeStates[key] = state
}