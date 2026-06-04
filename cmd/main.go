// qwen-usage CLI入口
package main

import (
	"fmt"
	"os"

	"github.com/panda/qwen-usage/internal/commands"
	"github.com/panda/qwen-usage/internal/config"
	"github.com/panda/qwen-usage/internal/server"
	"github.com/panda/qwen-usage/pkg/platform"
)

const (
	usage = `qwen-usage - Qwen Code 用量统计工具

用法:
  qwen-usage <command> [options]

命令:
  install    安装：生成配置和启动脚本，可选创建开机自启动
  uninstall  卸载：移除开机自启动的符号链接
  status     查询 server 运行状态
  stop       优雅停止 server
  server     启动后台服务（前台）
  record     记录用量并输出状态行
  export     导出用量报表
  clear      清理旧数据
  version    显示版本号

选项:
  -h, --help     显示帮助信息

示例:
  qwen-usage install              # 生成配置和启动脚本
  qwen-usage install -a           # 生成配置和启动脚本并设置开机自启动
  qwen-usage status               # 查询 server 状态
  qwen-usage stop                 # 优雅停止 server
  qwen-usage uninstall            # 移除开机自启动
  qwen-usage server               # 前台启动服务
  qwen-usage record < input.json  # 记录用量
  qwen-usage export -period day   # 导出今日报表
  qwen-usage clear                # 清理31天前数据
`
)

// 子命令帮助文本
const (
	helpInstall = `install - 安装

用法:
  qwen-usage install [options]

功能:
  在 exe 所在目录生成默认配置文件和 VBS 启动脚本。
  使用 -a 参数时，额外在开机启动目录创建符号链接，实现开机自启动。

  幂等操作：已存在的文件不会被覆盖，符号链接已存在则跳过。

选项:
  -a, --auto-start    创建开机自启动符号链接
  -h, --help          显示帮助信息

示例:
  qwen-usage install          # 仅生成配置和启动脚本
  qwen-usage install -a       # 生成文件并设置开机自启动
`

	helpUninstall = `uninstall - 卸载

用法:
  qwen-usage uninstall

功能:
  移除开机启动目录中的 qwen-usage.vbs 符号链接。
  不删除 exe 目录下的 config.json 和 start_server.vbs。

  幂等操作：符号链接不存在时正常返回。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage uninstall
`

	helpStatus = `status - 查询 server 状态

用法:
  qwen-usage status

功能:
  查询后台 server 的运行状态，包括监听地址和进程 PID。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage status
`

	helpStop = `stop - 优雅停止 server

用法:
  qwen-usage stop

功能:
  向运行中的 server 发送关闭请求，等待其优雅退出并清理 PID 文件。
  server 未运行时正常返回（幂等）。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage stop
`

	helpServer = `server - 启动后台服务

用法:
  qwen-usage server

功能:
  前台启动 HTTP 服务，接收 record 请求并记录用量数据。
  通常由 VBS 启动脚本自动调用，无需手动运行。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage server
`

	helpRecord = `record - 记录用量并输出状态行

用法:
  qwen-usage record [options] < input.json

功能:
  从 stdin 读取 Qwen Code 的 status line JSON，记录用量数据，
  并输出格式化的状态行用于显示。

输入:
  Qwen Code 的 status line JSON（通过 stdin 传入）

选项:
  -s              status_line 模式：只返回状态行，不记录到数据库
  -h, --help      显示帮助信息

示例:
  qwen-usage record < input.json           # 记录并输出状态行
  qwen-usage record -s < input.json        # 只输出状态行
`

	helpExport = `export - 导出用量报表

用法:
  qwen-usage export [options]

功能:
  导出用量统计数据，支持按时间段汇总或显示最近记录列表。

选项:
  -period <值>    时间周期: day(今天), week(7天), month(30天), 5h(5小时)
                   默认: day
  -n <数量>       显示最近 N 条记录列表（不汇总统计）
  -format <值>    输出格式: markdown, json
  -json           等同于 -format json
  -h, --help      显示帮助信息

示例:
  qwen-usage export                     # 今日统计报表
  qwen-usage export -period week        # 最近一周报表
  qwen-usage export -period 5h          # 最近5小时报表
  qwen-usage export -n 20               # 最近20条记录
  qwen-usage export -n 50 -json         # 最近50条记录(JSON格式)
`

	helpClear = `clear - 清理旧数据

用法:
  qwen-usage clear [options]

功能:
  删除指定天数之前的用量记录数据，释放存储空间。

选项:
  -days <天数>    清理多少天前的数据，默认: 31
  -h, --help      显示帮助信息

示例:
  qwen-usage clear              # 清理31天前的数据
  qwen-usage clear -days 7      # 清理7天前的数据
`
)

// checkHelp 检查是否需要显示帮助
func checkHelp(args []string, helpText string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Print(helpText)
			return true
		}
	}
	return false
}

func main() {
	// 确保 config 在所有操作之前初始化（支持 exe 目录配置优先）
	config.GetConfig()

	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "install":
		if checkHelp(args, helpInstall) {
			return
		}
		runInstall(args)
	case "uninstall":
		if checkHelp(args, helpUninstall) {
			return
		}
		runUninstall()
	case "status":
		if checkHelp(args, helpStatus) {
			return
		}
		runStatus()
	case "stop":
		if checkHelp(args, helpStop) {
			return
		}
		runStop()
	case "server":
		if checkHelp(args, helpServer) {
			return
		}
		runServerCmd()
	case "record":
		if checkHelp(args, helpRecord) {
			return
		}
		os.Exit(runRecordCmd(args))
	case "export":
		if checkHelp(args, helpExport) {
			return
		}
		os.Exit(runExportCmd(args))
	case "clear":
		if checkHelp(args, helpClear) {
			return
		}
		os.Exit(runClearCmd(args))
	case "version", "-v", "--version":
		fmt.Println(server.GetVersion())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
}

func runInstall(args []string) {
	autoStart := false
	for _, arg := range args {
		if arg == "-a" || arg == "--auto-start" {
			autoStart = true
		}
	}

	cfgPath, err := platform.GenerateConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成配置文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("配置文件: %s\n", cfgPath)

	vbsPath, err := platform.GenerateVBS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成启动脚本失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("启动脚本: %s\n", vbsPath)

	if autoStart {
		if err := platform.InstallStartup(); err != nil {
			fmt.Fprintf(os.Stderr, "设置开机自启动失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("开机自启动: %s\\qwen-usage.lnk\n", platform.StartupDir())
	}

	fmt.Println("安装完成")
}

func runUninstall() {
	if err := platform.UninstallStartup(); err != nil {
		fmt.Fprintf(os.Stderr, "移除自启动脚本失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("自启动脚本已移除（配置文件和启动脚本保留在 exe 目录）")
}

func runStatus() {
	running, pid, err := platform.QueryStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询状态失败: %v\n", err)
		os.Exit(1)
	}

	cfg := config.GetConfig()
	if running {
		fmt.Printf("server 运行中\n")
		fmt.Printf("  地址: %s\n", cfg.ServerAddr)
		if pid > 0 {
			fmt.Printf("  PID:  %d\n", pid)
		}
	} else {
		fmt.Println("server 未运行")
	}
}

func runStop() {
	if err := platform.StopServer(); err != nil {
		fmt.Fprintf(os.Stderr, "停止 server 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server 已停止")
}

func runExportCmd(args []string) int {
	period := "day"
	format := "markdown"
	limit := 0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-period":
			if i+1 < len(args) {
				period = args[i+1]
				i++
			}
		case "-format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		case "-json":
			format = "json"
		case "-n":
			if i+1 < len(args) {
				_, _ = fmt.Sscanf(args[i+1], "%d", &limit)
				i++
			}
		}
	}

	return commands.RunExport(period, "", "", format, limit)
}

func runClearCmd(args []string) int {
	days := 31

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-days":
			if i+1 < len(args) {
				_, _ = fmt.Sscanf(args[i+1], "%d", &days)
				i++
			}
		}
	}

	return commands.RunClear(days)
}

func runServerCmd() {
	if err := server.RunServer(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func runRecordCmd(args []string) int {
	statusLineOnly := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-s":
			statusLineOnly = true
		}
	}

	return commands.RunRecord(statusLineOnly)
}
