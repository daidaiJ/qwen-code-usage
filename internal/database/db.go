// Package database 数据库操作接口
package database

import (
	"errors"
	"time"
)

// 常见错误
var (
	ErrNotFound      = errors.New("record not found")
	ErrInvalidInput  = errors.New("invalid input")
	ErrDatabaseClosed = errors.New("database closed")
)

// MockError Mock测试用的错误类型
type MockError struct {
 Msg string
}

func (e *MockError) Error() string {
 return e.Msg
}

// DB 数据库操作接口（便于mock测试）
type DB interface {
	// 状态管理
	GetCumulativeState(sessionID, modelName string) (*CumulativeState, error)
	UpdateCumulativeState(state *CumulativeState) error

	// 记录管理
	InsertCallRecord(record *CallRecord) error
	GetCallRecords(startTime, endTime time.Time) ([]CallRecord, error)
	DeleteOldRecords(beforeDate time.Time) (int64, error)

	// 统计
	GetStats(startTime, endTime time.Time) (*StatsResponse, error)

	// 获取最近记录
	GetRecentRecords(limit int) ([]CallRecord, error)

	// 连接管理
	Close() error
}