// Package platform 跨平台兼容处理
package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/panda/qwen-usage/internal/config"
)

// GetPIDFilePath 获取PID文件路径
func GetPIDFilePath() string {
	return filepath.Join(config.GetDataDir(), "server.pid")
}

// WritePIDFile 写入PID文件
func WritePIDFile() error {
	pidPath := GetPIDFilePath()
	dir := filepath.Dir(pidPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// RemovePIDFile 删除PID文件
func RemovePIDFile() {
	pidPath := GetPIDFilePath()
	_ = os.Remove(pidPath)
}

// ReadPIDFile 读取PID文件
func ReadPIDFile() (int, error) {
	pidPath := GetPIDFilePath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}

// IsWindows 检查是否为Windows系统
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsLinux 检查是否为Linux系统
func IsLinux() bool {
	return runtime.GOOS == "linux"
}

// GetKillCommandHint 获取终止进程的命令提示
func GetKillCommandHint(pid int) string {
	if IsWindows() {
		return fmt.Sprintf("提示: 可以使用 taskkill /PID %d 强制终止", pid)
	}
	return fmt.Sprintf("提示: 可以使用 kill %d 强制终止", pid)
}

// GetDaemonHint 获取后台运行的命令提示
func GetDaemonHint() string {
	if IsWindows() {
		return "提示: Windows 下请使用 'start /B qwen-usage server' 后台运行"
	}
	return "提示: Linux/macOS 下请使用 'qwen-usage server &' 后台运行"
}

// GetShutdownSignals 获取系统支持的关闭信号
func GetShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// GetLogPath 获取日志文件路径
func GetLogPath() string {
	return filepath.Join(config.GetDataDir(), "server.log")
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	return filepath.Join(config.GetDataDir(), "config.json")
}

// StartServerInBackground 在后台启动 server 进程
func StartServerInBackground() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	cmd := exec.Command(exePath, "server")
	cmd.Env = os.Environ()

	// 分离标准IO，避免阻塞父进程
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// 平台特定的进程分离
	setSysProcAttr(cmd)

	return cmd.Start()
}