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

// ExeDir 返回可执行文件所在目录
func ExeDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
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

// loadConfig 从文件加载配置，exe 目录优先，回退到 ~/.qwen/usage/
func loadConfig() *Config {
	cfg := DefaultConfig()

	// 优先读取 exe 目录下的 config.json（install 命令生成）
	if exeDir, err := ExeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(exeDir, "config.json")); err == nil {
			var fileCfg Config
			if json.Unmarshal(data, &fileCfg) == nil {
				mergeConfig(cfg, &fileCfg)
				return cfg
			}
		}
	}

	// 回退到 ~/.qwen/usage/config.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".qwen", "usage", "config.json"))
	if err != nil {
		return cfg
	}

	var fileCfg Config
	if json.Unmarshal(data, &fileCfg) != nil {
		return cfg
	}
	mergeConfig(cfg, &fileCfg)
	return cfg
}

func mergeConfig(cfg *Config, fileCfg *Config) {
	if fileCfg.ServerAddr != "" {
		cfg.ServerAddr = fileCfg.ServerAddr
	}
	if fileCfg.DBPath != "" {
		cfg.DBPath = expandTilde(fileCfg.DBPath)
	}
	if fileCfg.ClientTimeoutMs > 0 {
		cfg.ClientTimeoutMs = fileCfg.ClientTimeoutMs
	}
	if fileCfg.ServerWriteTimeoutMs > 0 {
		cfg.ServerWriteTimeoutMs = fileCfg.ServerWriteTimeoutMs
	}
}

// expandTilde 将路径开头的 ~ 展开为用户主目录
func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[1:])
		}
	}
	return path
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