package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServerAddr != "127.0.0.1:9527" {
		t.Errorf("expected default ServerAddr to be 127.0.0.1:9527, got %s", cfg.ServerAddr)
	}

	if cfg.ClientTimeoutMs != 100 {
		t.Errorf("expected default ClientTimeoutMs to be 100, got %d", cfg.ClientTimeoutMs)
	}

	if cfg.ServerWriteTimeoutMs != 50 {
		t.Errorf("expected default ServerWriteTimeoutMs to be 50, got %d", cfg.ServerWriteTimeoutMs)
	}

	homeDir, _ := os.UserHomeDir()
	expectedDBPath := filepath.Join(homeDir, ".qwen", "usage", "usage.db")
	if cfg.DBPath != expectedDBPath {
		t.Errorf("expected default DBPath to be %s, got %s", expectedDBPath, cfg.DBPath)
	}
}

func TestGetConfigSingleton(t *testing.T) {
	ResetConfig()

	cfg1 := GetConfig()
	cfg2 := GetConfig()

	if cfg1 != cfg2 {
		t.Error("GetConfig should return the same instance")
	}
}

func TestSetConfig(t *testing.T) {
	ResetConfig()

	customCfg := &Config{
		ServerAddr:           "127.0.0.1:9999",
		DBPath:               "/custom/path/db.db",
		ClientTimeoutMs:      200,
		ServerWriteTimeoutMs: 100,
	}

	SetConfig(customCfg)

	cfg := GetConfig()
	if cfg.ServerAddr != "127.0.0.1:9999" {
		t.Errorf("expected ServerAddr to be 127.0.0.1:9999, got %s", cfg.ServerAddr)
	}
	if cfg.ClientTimeoutMs != 200 {
		t.Errorf("expected ClientTimeoutMs to be 200, got %d", cfg.ClientTimeoutMs)
	}
}

func TestEnsureDBPath(t *testing.T) {
	// 使用临时目录
	tmpDir := filepath.Join(os.TempDir(), "qwen-usage-test")
	dbPath := filepath.Join(tmpDir, "subdir", "test.db")

	err := EnsureDBPath(dbPath)
	if err != nil {
		t.Errorf("EnsureDBPath failed: %v", err)
	}

	// 检查目录是否存在
	dir := filepath.Dir(dbPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("directory %s should exist", dir)
	}

	// 清理
	_ = os.RemoveAll(tmpDir)
}

func TestGetDataDir(t *testing.T) {
	dir := GetDataDir()

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".qwen", "usage")

	if dir != expected {
		t.Errorf("expected GetDataDir to be %s, got %s", expected, dir)
	}
}

func TestExpandTilde(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.qwen/usage/usage.db", filepath.Join(homeDir, ".qwen", "usage", "usage.db")},
		{"/absolute/path/db.db", "/absolute/path/db.db"},
		{"", ""},
	}

	for _, tt := range tests {
		got := expandTilde(tt.input)
		if got != tt.expected {
			t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMergeConfigExpandsTilde(t *testing.T) {
	cfg := DefaultConfig()
	fileCfg := &Config{
		DBPath: "~/.qwen/usage/custom.db",
	}
	mergeConfig(cfg, fileCfg)

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".qwen", "usage", "custom.db")
	if cfg.DBPath != expected {
		t.Errorf("expected merged DBPath to be %s, got %s", expected, cfg.DBPath)
	}
}