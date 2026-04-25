// Package logger 日志系统
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/panda/qwen-usage/internal/config"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger 日志器
type Logger struct {
	mu         sync.Mutex
	level      LogLevel
	file       *os.File
	console    *log.Logger
	fileLogger *log.Logger
	logFile    string
	maxSize    int64
}

var (
	loggerInstance *Logger
	loggerOnce     sync.Once
)

// GetLogger 获取日志实例（单例）
func GetLogger() *Logger {
	loggerOnce.Do(func() {
		loggerInstance = newLogger()
	})
	return loggerInstance
}

// ResetLogger 重置日志实例（用于测试）
func ResetLogger() {
	loggerOnce = sync.Once{}
	loggerInstance = nil
}

// newLogger 创建日志实例
func newLogger() *Logger {
	logPath := filepath.Join(config.GetDataDir(), "server.log")

	// 确保日志目录存在
	os.MkdirAll(filepath.Dir(logPath), 0755)

	l := &Logger{
		level:   INFO,
		logFile: logPath,
		maxSize: 10 * 1024 * 1024, // 10MB
	}

	// 打开日志文件
	l.openFile()

	// 控制台 logger
	l.console = log.New(os.Stdout, "", 0)

	return l
}

// NewTestLogger 创建测试用的日志器（输出到 stdout）
func NewTestLogger() *Logger {
	return &Logger{
		level:   DEBUG,
		console: log.New(os.Stdout, "", 0),
	}
}

// openFile 打开日志文件
func (l *Logger) openFile() {
	file, err := os.OpenFile(l.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		l.console.Printf("[ERROR] failed to open log file: %v", err)
		return
	}
	l.file = file
	l.fileLogger = log.New(file, "", 0)
}

// checkRotate 检查日志轮转
func (l *Logger) checkRotate() {
	if l.file == nil {
		return
	}

	info, err := l.file.Stat()
	if err != nil {
		return
	}

	if info.Size() >= l.maxSize {
		l.rotate()
	}
}

// rotate 日志轮转
func (l *Logger) rotate() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
	}

	// 备份旧日志
	backup := l.logFile + "." + time.Now().Format("20060102_150405")
	os.Rename(l.logFile, backup)

	// 重新打开
	l.openFile()
}

// log 写入日志
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, level.String(), msg)

	// 输出到控制台（ERROR 及以上级别）
	if level >= WARN {
		l.console.Println(line)
	}

	// 输出到文件
	if l.fileLogger != nil {
		l.fileLogger.Println(line)
		l.checkRotate()
	}
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal 致命日志
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
	os.Exit(1)
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Close 关闭日志
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// 便捷函数
func LogInfo(format string, args ...interface{})  { GetLogger().Info(format, args...) }
func LogWarn(format string, args ...interface{})  { GetLogger().Warn(format, args...) }
func LogError(format string, args ...interface{}) { GetLogger().Error(format, args...) }
func LogDebug(format string, args ...interface{}) { GetLogger().Debug(format, args...) }

// StdoutLogger 仅输出到 stdout 的 logger
type StdoutLogger struct{}

func (l StdoutLogger) Fatal(v ...interface{}) {
	fmt.Println(v...)
	os.Exit(1)
}

// MultiWriter 多输出写入器
type MultiWriter struct {
	writers []io.Writer
}

func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	return &MultiWriter{writers: writers}
}

func (m *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range m.writers {
		w.Write(p)
	}
	return len(p), nil
}