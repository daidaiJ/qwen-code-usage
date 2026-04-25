package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsWindows(t *testing.T) {
	result := IsWindows()
	expected := runtime.GOOS == "windows"
	if result != expected {
		t.Errorf("IsWindows() = %v, expected %v", result, expected)
	}
}

func TestIsLinux(t *testing.T) {
	result := IsLinux()
	expected := runtime.GOOS == "linux"
	if result != expected {
		t.Errorf("IsLinux() = %v, expected %v", result, expected)
	}
}

func TestGetKillCommandHint(t *testing.T) {
	pid := 12345
	hint := GetKillCommandHint(pid)

	if IsWindows() {
		if !contains(hint, "taskkill") {
			t.Errorf("Windows hint should contain 'taskkill', got: %s", hint)
		}
	} else {
		if !contains(hint, "kill") {
			t.Errorf("Linux hint should contain 'kill', got: %s", hint)
		}
	}
}

func TestGetDaemonHint(t *testing.T) {
	hint := GetDaemonHint()

	if IsWindows() {
		if !contains(hint, "start /B") {
			t.Errorf("Windows daemon hint should contain 'start /B', got: %s", hint)
		}
	} else {
		if !contains(hint, "&") {
			t.Errorf("Linux daemon hint should contain '&', got: %s", hint)
		}
	}
}

func TestPIDFileOperations(t *testing.T) {
	// 使用临时目录模拟
	tmpDir := filepath.Join(os.TempDir(), "qwen-usage-pid-test")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	// 修改GetDataDir的行为（通过设置临时HOME）
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// 测试写入
	err := WritePIDFile()
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	pidPath := GetPIDFilePath()
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Errorf("PID file should exist at %s", pidPath)
	}

	// 测试读取
	pid, err := ReadPIDFile()
	if err != nil {
		t.Fatalf("ReadPIDFile failed: %v", err)
	}

	if pid != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), pid)
	}

	// 测试删除
	RemovePIDFile()
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed")
	}
}

func TestGetShutdownSignals(t *testing.T) {
	signals := GetShutdownSignals()

	if len(signals) != 2 {
		t.Errorf("expected 2 signals, got %d", len(signals))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}