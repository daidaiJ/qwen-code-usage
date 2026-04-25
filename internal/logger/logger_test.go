package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{FATAL, "FATAL"},
	}

	for _, test := range tests {
		result := test.level.String()
		if result != test.expected {
			t.Errorf("LogLevel(%d).String() = %s, expected %s", test.level, result, test.expected)
		}
	}
}

func TestNewTestLogger(t *testing.T) {
	logger := NewTestLogger()

	if logger.level != DEBUG {
		t.Errorf("test logger should have DEBUG level, got %d", logger.level)
	}

	if logger.console == nil {
		t.Error("test logger should have console output")
	}
}

func TestLoggerSetLevel(t *testing.T) {
	logger := NewTestLogger()

	logger.SetLevel(WARN)

	if logger.level != WARN {
		t.Errorf("expected level WARN, got %d", logger.level)
	}
}

func TestLoggerOutput(t *testing.T) {
	// 使用测试logger
	logger := NewTestLogger()

	// 这些调用不应该崩溃
	logger.Debug("test debug message")
	logger.Info("test info message")
	logger.Warn("test warn message")
	logger.Error("test error message")
}

func TestStdoutLogger(t *testing.T) {
	logger := StdoutLogger{}

	// 验证类型存在
	_ = logger
}

func TestMultiWriter(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	multi := NewMultiWriter(buf1, buf2)

	data := []byte("test data")
	n, err := multi.Write(data)

	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}

	if buf1.String() != "test data" {
		t.Errorf("buf1 should contain 'test data', got '%s'", buf1.String())
	}

	if buf2.String() != "test data" {
		t.Errorf("buf2 should contain 'test data', got '%s'", buf2.String())
	}
}

func TestLogFunctions(t *testing.T) {
	// 重置logger以确保测试隔离
	ResetLogger()

	// 创建测试logger
	testLogger := NewTestLogger()
	loggerInstance = testLogger
	loggerOnce.Do(func() {})

	// 测试便捷函数不会崩溃
	LogInfo("test info")
	LogWarn("test warn")
	LogError("test error")
	LogDebug("test debug")

	// 主要验证不会崩溃
}

func TestLoggerRotate(t *testing.T) {
	// 创建临时日志文件
	tmpDir := filepath.Join(os.TempDir(), "qwen-usage-log-test")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	logger := &Logger{
		level:   INFO,
		logFile: logPath,
		maxSize: 100, // 设置很小的maxSize以便触发轮转
	}

	// 打开文件
	logger.openFile()

	// 写入足够的数据以触发轮转
	for i := 0; i < 20; i++ {
		logger.Info("test message %d with some content to fill the file", i)
	}

	// 验证日志文件存在
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file should exist")
	}

	logger.Close()
}