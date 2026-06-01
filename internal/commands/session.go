// Package commands 命令实现
package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/panda/qwen-usage/internal/config"
	"github.com/panda/qwen-usage/internal/database"
	"github.com/panda/qwen-usage/pkg/platform"
)

// RunStart 执行start命令（启动会话）
// 流程: 探测server → 无则自动启动 → 发送 /session/start 增加计数
func RunStart() int {
	cfg := config.GetConfig()
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://%s/health", cfg.ServerAddr)
	startURL := fmt.Sprintf("http://%s/session/start", cfg.ServerAddr)

	// 探测 server 是否已就绪
	if !isServerReady(client, healthURL) {
		// server 未运行，尝试自动启动
		fmt.Println("server 未运行，正在启动...")
		if err := platform.StartServerInBackground(); err != nil {
			fmt.Fprintf(os.Stderr, "启动 server 失败: %v\n", err)
			return 1
		}

		// 等待 server 就绪（最多 5 秒）
		if !waitForServer(client, healthURL, 5*time.Second) {
			fmt.Fprintln(os.Stderr, "server 启动超时")
			return 1
		}
	}

	// 发送 session/start
	resp, err := client.Post(startURL, "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "发送 session/start 失败: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	var sessionResp database.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		fmt.Fprintf(os.Stderr, "解析响应失败: %v\n", err)
		return 1
	}

	fmt.Printf("会话开始，当前计数: %d\n", sessionResp.Count)
	return 0
}

// RunStop 执行stop命令（结束会话）
// 流程: 向 server 发送 /session/end 减少计数，归零时 server 自动退出
func RunStop() int {
	cfg := config.GetConfig()
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://%s/health", cfg.ServerAddr)
	endURL := fmt.Sprintf("http://%s/session/end", cfg.ServerAddr)

	// server 不存在时直接返回，无需报错
	if !isServerReady(client, healthURL) {
		return 0
	}

	resp, err := client.Post(endURL, "application/json", nil)
	if err != nil {
		// server 可能在处理中已关闭，不算错误
		return 0
	}
	defer func() { _ = resp.Body.Close() }()

	var sessionResp database.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return 0
	}

	fmt.Printf("会话结束，当前计数: %d\n", sessionResp.Count)
	if sessionResp.Count == 0 {
		fmt.Println("计数归零，server 正在关闭...")
	}
	return 0
}

// RunKill 执行kill命令（强制关闭）
func RunKill() int {
	cfg := config.GetConfig()

	url := fmt.Sprintf("http://%s/shutdown", cfg.ServerAddr)
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 shutdown 请求失败: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接 server 失败: %v\n", err)
		pid, readErr := platform.ReadPIDFile()
		if readErr == nil {
			fmt.Fprintln(os.Stderr, platform.GetKillCommandHint(pid))
		}
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "server 返回错误: %d\n", resp.StatusCode)
		return 1
	}

	fmt.Println("server 正在强制关闭...")
	return 0
}

// isServerReady 检测 server 是否就绪
func isServerReady(client *http.Client, healthURL string) bool {
	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitForServer 轮询等待 server 就绪
func waitForServer(client *http.Client, healthURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isServerReady(client, healthURL) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
