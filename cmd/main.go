// qwen-usage CLI入口
package main

import (
	"fmt"
	"os"

	"github.com/panda/qwen-usage/internal/commands"
	"github.com/panda/qwen-usage/internal/server"
	"github.com/panda/qwen-usage/pkg/platform"
)

const (
	version = "1.0.0"
	usage   = `qwen-usage - Qwen Code 用量统计工具

用法:
  qwen-usage <command> [options]

命令:
  server    启动后台服务
  start     增加会话计数（由 hook 调用）
  stop      减少会话计数，为0时关闭服务（由 hook 调用）
  kill      强制终止后台服务
  record    记录用量并输出状态行
  export    导出用量报表
  clear     清理旧数据
  version   显示版本号

选项:
  -h, --help     显示帮助信息

示例:
  qwen-usage server                    # 启动服务
  qwen-usage record < input.json      # 记录用量
  qwen-usage export -period day       # 导出今日报表
  qwen-usage export -period week      # 导出本周报表
  qwen-usage export -period 5h        # 导出最近5小时报表
  qwen-usage export -n 20             # 显示最近20条记录
  qwen-usage export -n 50 -json       # 以JSON格式显示最近50条记录
  qwen-usage clear                    # 清理31天前数据（默认）
  qwen-usage clear -days 7            # 清理7天前数据
`
)

// 子命令帮助文本
const (
	helpServer = `server - 启动后台服务

用法:
  qwen-usage server [options]

功能:
  启动 HTTP 服务，接收 record/start/stop 请求，记录用量数据。

选项:
  -d, --daemon    提示后台运行方式（不实际启动后台）
  -h, --help      显示帮助信息

示例:
  qwen-usage server              # 前台启动服务
`

	helpStart = `start - 增加会话计数

用法:
  qwen-usage start

功能:
  探测 server 是否就绪，未就绪则自动在后台启动 server，
  然后发送 /session/start 增加会话计数。

  由 Qwen Code 的 SessionStart hook 调用。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage start
`

	helpStop = `stop - 减少会话计数

用法:
  qwen-usage stop

功能:
  向 server 发送 /session/end 减少会话计数。
  当计数归零时，server 自动优雅退出。
  若 server 已不存在，静默返回。

  由 Qwen Code 的 SessionEnd hook 调用。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage stop
`

	helpKill = `kill - 强制终止后台服务

用法:
  qwen-usage kill

功能:
  强制关闭正在运行的后台服务，无论会话计数是否为 0。

选项:
  -h, --help      显示帮助信息

示例:
  qwen-usage kill
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
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "server":
		if checkHelp(args, helpServer) {
			return
		}
		runServerCmd(args)
	case "start":
		if checkHelp(args, helpStart) {
			return
		}
		os.Exit(commands.RunStart())
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
	case "stop":
		if checkHelp(args, helpStop) {
			return
		}
		os.Exit(commands.RunStop())
	case "kill":
		if checkHelp(args, helpKill) {
			return
		}
		os.Exit(commands.RunKill())
	case "version", "-v", "--version":
		fmt.Printf("qwen-usage version %s\n", server.GetVersion())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
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

func runServerCmd(args []string) {
	// 检查是否后台运行
	for _, arg := range args {
		if arg == "-d" || arg == "--daemon" {
			fmt.Println(platform.GetDaemonHint())
			break
		}
	}

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