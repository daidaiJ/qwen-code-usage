# qwen-usage

Qwen Code 用量统计工具。通过监听 Qwen Code 的 status line JSON 数据，自动计算增量并存储到 SQLite，支持按天/周/月/5小时导出 Markdown 用量报表。

## 项目结构

```
qwen-usage/
├── cmd/
│   └── qwen-usage/
│       └── main.go              # CLI 入口
├── internal/
│   ├── config/
│       ├── config.go            # 配置管理
│       └── config_test.go       # 配置测试
│   ├── database/
│       ├── db.go                # 数据库接口
│       ├── sqlite.go            # SQLite 实现
│       ├── models.go            # 数据模型
│       └── database_test.go     # 数据库测试
│   ├── server/
│       └── server.go            # HTTP 服务
│   ├── commands/
│       └── commands.go          # 命令实现
│   └── logger/
│       ├── logger.go            # 日志系统
│       └── logger_test.go       # 日志测试
├── pkg/
│   └── platform/
│       ├── platform.go          # 跨平台兼容处理
│       └── platform_test.go     # 平台测试
├── test/
│   └── mocks/
│       ├── mock_db.go           # Mock 数据库
│       ├── mock_http.go         # Mock HTTP 客户端
│       └ mock_test.go           # Mock 测试
├── bin/                         # 编译输出
├── go.mod
├── go.sum
├── Makefile                     # 构建脚本
└── README.md
```

## 架构

```
┌─────────────┐   HTTP(超时100ms)   ┌─────────────────┐
│ qwen-usage  │ ─────────────────► │  qwen-usage     │
│   record    │    成功/失败都返回   │    server       │
│  (CLI客户端) │ ◄───────────────── │  (后台守护进程)  │
└─────────────┘    立即输出状态行    └────────┬────────┘
                                             │
                                             ▼
                                      ┌─────────────┐
                                      │   SQLite    │
                                      └─────────────┘
```

- **Server**：后台 HTTP 服务，维护内存缓存累计值，计算增量后写入 SQLite
- **CLI Client**：轻量客户端，server 不可用时自动降级直接输出状态行
- **直接访问**：`export` / `clear` 命令直接操作 SQLite，不依赖 server

## 安装

### 编译安装

```bash
# 克隆仓库
cd qwen-usage

# 使用 Makefile 构建
make build            # 当前平台
make build-linux      # Linux (WSL 兼容)
make build-windows    # Windows
make build-all        # 所有平台

# 或直接使用 go build
go build -o bin/qwen-usage ./cmd/qwen-usage
```

### 依赖

- Go 1.23+
- 使用 `modernc.org/sqlite`（纯 Go 实现，无需安装 sqlite3 系统库，**跨平台兼容**）

## WSL Ubuntu 22 兼容性

本项目完全兼容 WSL Ubuntu 22：

- 使用纯 Go SQLite 驱动（`modernc.org/sqlite`），无需 CGO
- 跨平台路径处理（`filepath.Join`）
- 自动检测操作系统并显示正确的命令提示
- 支持 Linux 信号处理（SIGINT/SIGTERM）

```bash
# 在 WSL 中构建
make build-linux

# 安装到 PATH
cp bin/qwen-usage ~/.local/bin/
```

## 配置

配置文件：`~/.qwen/usage/config.json`

首次运行自动生成默认配置：

```json
{
  "server_addr": "127.0.0.1:9527",
  "db_path": "~/.qwen/usage/usage.db",
  "client_timeout_ms": 100,
  "server_write_timeout_ms": 50
}
```

## 使用

### 1. 启动 Server

```bash
# 前台运行
./qwen-usage server

# 后台运行（Linux/macOS/WSL）
./qwen-usage server &

# 后台运行（Windows）
start /B qwen-usage.exe server
```

### 2. 配置 Qwen Code Status Line

在 `~/.qwen/settings.json` 中添加：

```json
{
  "ui": {
    "statusLine": {
      "type": "command",
      "command": "input=$(cat); /path/to/qwen-usage record <<< \"$input\""
    }
  }
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "bash -c '/path/to/qwen-usage start '",
            "name": "qwen-usage-start",
            "description": "Start qwen-usage server for token tracking"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "bash -c '/path/to/qwen-usage stop '",
            "name": "qwen-usage-stop",
            "description": "Stop qwen-usage server when session ends"
          }
        ]
      }
    ]
  }
}
```

### 3. 导出报表

```bash
# 按时间段汇总统计
./qwen-usage export              # 导出今天
./qwen-usage export -period 5h   # 导出最近5小时
./qwen-usage export -period week # 导出本周
./qwen-usage export -period month # 导出本月
./qwen-usage export -json        # JSON 格式

# 显示最近记录列表
./qwen-usage export -n 20        # 最近20条记录
./qwen-usage export -n 50 -json  # 最近50条记录(JSON格式)
```

## 测试

```bash
# 运行所有测试
make test

# 运行测试并生成覆盖率报告
make test-coverage

# 运行特定模块测试
go test -v ./internal/database
go test -v ./test/mocks
```

## 开发

```bash
# 开发环境设置
make dev-setup

# 代码格式化
make fmt

# 代码检查
make vet
make lint
```

## 命令汇总

```bash
# 服务管理
qwen-usage server              # 启动后台服务
qwen-usage start               # 增加会话计数（由 hook 调用）
qwen-usage stop                # 减少会话计数，为0时关闭服务（由 hook 调用）
qwen-usage kill                # 强制关闭服务

# 数据记录
qwen-usage record              # 记录用量（由 Qwen Code 自动调用）

# 数据导出
qwen-usage export              # 导出今天报表
qwen-usage export -period 5h   # 导出最近5小时
qwen-usage export -period week # 导出最近一周
qwen-usage export -period month # 导出最近一个月
qwen-usage export -n 20        # 显示最近20条记录
qwen-usage export -n 50 -json  # 最近50条记录(JSON格式)
qwen-usage export -json        # JSON 格式输出

# 数据清理
qwen-usage clear               # 清理31天前数据
qwen-usage clear -days 7       # 清理7天前数据

# 帮助
qwen-usage -h                  # 显示全局帮助
qwen-usage export -h           # 显示 export 子命令帮助
qwen-usage server --help       # 显示 server 子命令帮助
qwen-usage <command> -h        # 显示任意子命令帮助

# 其他
qwen-usage version             # 显示版本
```

### 子命令帮助

每个子命令都支持 `-h` 或 `--help` 参数查看详细帮助：

```bash
qwen-usage server -h      # 服务启动帮助
qwen-usage start -h       # 会话计数增加帮助
qwen-usage stop -h        # 会话计数减少帮助
qwen-usage kill -h        # 强制关闭帮助
qwen-usage record -h      # 用量记录帮助
qwen-usage export -h      # 报表导出帮助
qwen-usage clear -h       # 数据清理帮助
```

## License

Apache-2.0 license