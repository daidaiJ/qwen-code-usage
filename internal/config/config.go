// Package config 配置管理
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config 应用配置
type Config struct {
	ServerAddr           string `json:"server_addr"`
	DBPath               string `json:"db_path"`
	ClientTimeoutMs      int    `json:"client_timeout_ms"`
	ServerWriteTimeoutMs int    `json:"server_write_timeout_ms"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		ServerAddr:           "127.0.0.1:9527",
		DBPath:               filepath.Join(homeDir, ".qwen", "usage", "usage.db"),
		ClientTimeoutMs:      100,
		ServerWriteTimeoutMs: 50,
	}
}

var (
	configOnce sync.Once
	appConfig  *Config
)

// GetConfig 获取配置（单例）
func GetConfig() *Config {
	configOnce.Do(func() {
		appConfig = loadConfig()
	})
	return appConfig
}

// ResetConfig 重置配置（用于测试）
func ResetConfig() {
	configOnce = sync.Once{}
	appConfig = nil
}

// SetConfig 设置配置（用于测试）
func SetConfig(cfg *Config) {
	ResetConfig()
	appConfig = cfg
	configOnce.Do(func() {})
}

// loadConfig 从文件加载配置
func loadConfig() *Config {
	cfg := DefaultConfig()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	configPath := filepath.Join(homeDir, ".qwen", "usage", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return cfg
	}

	// 合并配置
	if fileCfg.ServerAddr != "" {
		cfg.ServerAddr = fileCfg.ServerAddr
	}
	if fileCfg.DBPath != "" {
		cfg.DBPath = fileCfg.DBPath
	}
	if fileCfg.ClientTimeoutMs > 0 {
		cfg.ClientTimeoutMs = fileCfg.ClientTimeoutMs
	}
	if fileCfg.ServerWriteTimeoutMs > 0 {
		cfg.ServerWriteTimeoutMs = fileCfg.ServerWriteTimeoutMs
	}

	return cfg
}

// EnsureDBPath 确保 DB 目录存在
func EnsureDBPath(dbPath string) error {
	dir := filepath.Dir(dbPath)
	return os.MkdirAll(dir, 0755)
}

// GetDataDir 获取数据目录路径
func GetDataDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".qwen", "usage")
}