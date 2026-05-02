// Package database SQLite实现
package database

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/panda/qwen-usage/internal/config"
)

// SQLiteDB SQLite数据库实现
type SQLiteDB struct {
	conn            *sql.DB
	mu              sync.RWMutex
	cumulativeCache map[string]*CumulativeState
	cacheMu         sync.RWMutex
}

// dbInstance 数据库单例
var dbInstance *SQLiteDB
var dbOnce sync.Once

// GetDB 获取数据库实例
func GetDB() (DB, error) {
	var initErr error
	dbOnce.Do(func() {
		dbInstance, initErr = newSQLiteDB()
	})
	if initErr != nil {
		return nil, initErr
	}
	return dbInstance, nil
}

// ResetDB 重置数据库实例（用于测试）
func ResetDB() {
	dbOnce = sync.Once{}
	dbInstance = nil
}

// newSQLiteDB 创建SQLite数据库连接
func newSQLiteDB() (*SQLiteDB, error) {
	cfg := config.GetConfig()

	// 确保 DB 目录存在
	if err := config.EnsureDBPath(cfg.DBPath); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// 连接数据库
	conn, err := sql.Open("sqlite", cfg.DBPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池
	conn.SetMaxOpenConns(1) // SQLite 单写连接
	conn.SetMaxIdleConns(1)

	db := &SQLiteDB{
		conn:            conn,
		cumulativeCache: make(map[string]*CumulativeState),
	}

	// 创建表
	if err := db.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return db, nil
}

// NewSQLiteDBWithPath 使用指定路径创建数据库（用于测试）
func NewSQLiteDBWithPath(dbPath string) (*SQLiteDB, error) {
	if err := config.EnsureDBPath(dbPath); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db := &SQLiteDB{
		conn:            conn,
		cumulativeCache: make(map[string]*CumulativeState),
	}

	if err := db.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return db, nil
}

// createTables 创建数据表
func (db *SQLiteDB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS cumulative_state (
			session_id TEXT NOT NULL,
			model_name TEXT NOT NULL,
			total_requests INTEGER NOT NULL DEFAULT 0,
			total_latency_ms INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			thoughts_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (session_id, model_name)
		)`,
		`CREATE TABLE IF NOT EXISTS call_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			model_name TEXT NOT NULL,
			request_seq INTEGER NOT NULL,
			latency_ms INTEGER NOT NULL,
			prompt_tokens INTEGER NOT NULL,
			completion_tokens INTEGER NOT NULL,
			cached_tokens INTEGER NOT NULL,
			thoughts_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recorded_at ON call_records(recorded_at)`,
		`CREATE INDEX IF NOT EXISTS idx_session_model ON call_records(session_id, model_name)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// cacheKey 生成缓存键
func cacheKey(sessionID, modelName string) string {
	return sessionID + "|" + modelName
}

// GetCumulativeState 获取累计状态（优先从缓存）
func (db *SQLiteDB) GetCumulativeState(sessionID, modelName string) (*CumulativeState, error) {
	key := cacheKey(sessionID, modelName)

	// 先查缓存
	db.cacheMu.RLock()
	if state, ok := db.cumulativeCache[key]; ok {
		db.cacheMu.RUnlock()
		return state, nil
	}
	db.cacheMu.RUnlock()

	// 从数据库查询
	db.mu.RLock()
	defer db.mu.RUnlock()

	state := &CumulativeState{
		SessionID: sessionID,
		ModelName: modelName,
	}

	query := `SELECT total_requests, total_latency_ms, prompt_tokens, completion_tokens,
		cached_tokens, thoughts_tokens, total_tokens
		FROM cumulative_state WHERE session_id = ? AND model_name = ?`

	err := db.conn.QueryRow(query, sessionID, modelName).Scan(
		&state.TotalRequests, &state.TotalLatencyMs, &state.PromptTokens,
		&state.CompletionTokens, &state.CachedTokens, &state.ThoughtsTokens,
		&state.TotalTokens,
	)

	if err == sql.ErrNoRows {
		// 不存在，返回空状态
		db.cacheMu.Lock()
		db.cumulativeCache[key] = state
		db.cacheMu.Unlock()
		return state, nil
	}
	if err != nil {
		return nil, err
	}

	// 存入缓存
	db.cacheMu.Lock()
	db.cumulativeCache[key] = state
	db.cacheMu.Unlock()

	return state, nil
}

// UpdateCumulativeState 更新累计状态（缓存 + DB）
func (db *SQLiteDB) UpdateCumulativeState(state *CumulativeState) error {
	// 更新缓存
	key := cacheKey(state.SessionID, state.ModelName)
	db.cacheMu.Lock()
	db.cumulativeCache[key] = state
	db.cacheMu.Unlock()

	// 同步写入数据库
	db.mu.Lock()
	defer db.mu.Unlock()

	query := `INSERT OR REPLACE INTO cumulative_state
		(session_id, model_name, total_requests, total_latency_ms, prompt_tokens,
		completion_tokens, cached_tokens, thoughts_tokens, total_tokens, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	_, err := db.conn.Exec(query,
		state.SessionID, state.ModelName, state.TotalRequests, state.TotalLatencyMs,
		state.PromptTokens, state.CompletionTokens, state.CachedTokens,
		state.ThoughtsTokens, state.TotalTokens,
	)

	return err
}

// InsertCallRecord 插入调用记录
func (db *SQLiteDB) InsertCallRecord(record *CallRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// 使用当前时间
	now := time.Now()
	record.RecordedAt = now

	query := `INSERT INTO call_records
		(session_id, model_name, request_seq, latency_ms, prompt_tokens,
		completion_tokens, cached_tokens, thoughts_tokens, total_tokens, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := db.conn.Exec(query,
		record.SessionID, record.ModelName, record.RequestSeq, record.LatencyMs,
		record.PromptTokens, record.CompletionTokens, record.CachedTokens,
		record.ThoughtsTokens, record.TotalTokens, now,
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	record.ID = id
	return nil
}

// GetCallRecords 获取时间范围内的调用记录
func (db *SQLiteDB) GetCallRecords(startTime, endTime time.Time) ([]CallRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	query := `SELECT id, session_id, model_name, request_seq, latency_ms,
		prompt_tokens, completion_tokens, cached_tokens, thoughts_tokens, total_tokens, recorded_at
		FROM call_records WHERE recorded_at >= ? AND recorded_at <= ?
		ORDER BY recorded_at ASC`

	rows, err := db.conn.Query(query, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CallRecord
	for rows.Next() {
		var r CallRecord
		err := rows.Scan(&r.ID, &r.SessionID, &r.ModelName, &r.RequestSeq,
			&r.LatencyMs, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.ThoughtsTokens, &r.TotalTokens, &r.RecordedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, nil
}

// DeleteOldRecords 删除旧记录
func (db *SQLiteDB) DeleteOldRecords(beforeDate time.Time) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.conn.Exec(
		`DELETE FROM call_records WHERE recorded_at < ?`,
		beforeDate,
	)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// GetStats 获取统计数据
func (db *SQLiteDB) GetStats(startTime, endTime time.Time) (*StatsResponse, error) {
	records, err := db.GetCallRecords(startTime, endTime)
	if err != nil {
		return nil, err
	}

	// 按模型分组统计
	modelStatsMap := make(map[string]*ModelStats)
	var allLatencies []int
	var totalTokens int64
	var totalLatency int

	for _, r := range records {
		stats, ok := modelStatsMap[r.ModelName]
		if !ok {
			stats = &ModelStats{ModelName: r.ModelName}
			modelStatsMap[r.ModelName] = stats
		}

		stats.RequestCount++
		stats.PromptTokens += int64(r.PromptTokens)
		stats.CompletionTokens += int64(r.CompletionTokens)
		stats.CachedTokens += int64(r.CachedTokens)
		stats.ThoughtsTokens += int64(r.ThoughtsTokens)
		stats.TotalTokens += int64(r.TotalTokens)
		stats.TotalLatencyMs += r.LatencyMs
		allLatencies = append(allLatencies, r.LatencyMs)

		totalTokens += int64(r.TotalTokens)
		totalLatency += r.LatencyMs
	}

	// 计算延迟统计
	for _, stats := range modelStatsMap {
		if stats.RequestCount > 0 {
			stats.AvgLatencyMs = float64(stats.TotalLatencyMs) / float64(stats.RequestCount)
			if stats.PromptTokens > 0 {
				stats.CachePercent = float64(stats.CachedTokens) / float64(stats.PromptTokens) * 100
			}
			// 计算未命中缓存 token 吞吐量 (tokens/s)
			latencySec := float64(stats.TotalLatencyMs) / 1000.0
			if latencySec > 0 {
				uncachedTokens := stats.PromptTokens - stats.CachedTokens
				if uncachedTokens < 0 {
					uncachedTokens = 0
				}
				stats.TokensPerSec = float64(uncachedTokens+stats.CompletionTokens) / latencySec
			}
		}
	}

	// 计算百分位延迟
	var models []ModelStats
	for _, stats := range modelStatsMap {
		// 收集该模型的延迟数据
		var modelLatencies []int
		for _, r := range records {
			if r.ModelName == stats.ModelName {
				modelLatencies = append(modelLatencies, r.LatencyMs)
			}
		}
		sort.Ints(modelLatencies)
		if len(modelLatencies) > 0 {
			stats.P50LatencyMs = float64(modelLatencies[len(modelLatencies)*50/100])
			stats.P95LatencyMs = float64(modelLatencies[len(modelLatencies)*95/100])
		}
		models = append(models, *stats)
	}

	// 排序
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelName < models[j].ModelName
	})

	// 总体统计
	var avgLatency float64
	if len(records) > 0 {
		avgLatency = float64(totalLatency) / float64(len(records))
	}

	return &StatsResponse{
		Models:    models,
		StartTime: startTime.Format("2006-01-02 15:04:05"),
		EndTime:   endTime.Format("2006-01-02 15:04:05"),
		Total: TotalStats{
			RequestCount: len(records),
			TotalTokens:  totalTokens,
			AvgLatencyMs: avgLatency,
		},
	}, nil
}

// GetRecentRecords 获取最近N条记录
func (db *SQLiteDB) GetRecentRecords(limit int) ([]CallRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	query := `SELECT id, session_id, model_name, request_seq, latency_ms,
		prompt_tokens, completion_tokens, cached_tokens, thoughts_tokens, total_tokens, recorded_at
		FROM call_records ORDER BY recorded_at DESC LIMIT ?`

	rows, err := db.conn.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CallRecord
	for rows.Next() {
		var r CallRecord
		err := rows.Scan(&r.ID, &r.SessionID, &r.ModelName, &r.RequestSeq,
			&r.LatencyMs, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.ThoughtsTokens, &r.TotalTokens, &r.RecordedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, nil
}

// Close 关闭数据库连接
func (db *SQLiteDB) Close() error {
	return db.conn.Close()
}