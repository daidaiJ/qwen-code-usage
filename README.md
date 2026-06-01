# qwen-usage

Qwen Code 用量统计工具。通过 Status Line JSON 数据，计算增量并记录到数据库。

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
│       ├── record.go            # record 子命令
│       ├── export.go            # export/clear 子命令
│       └── session.go           # start/stop/kill 子命令
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

## 观测模式

### Status Line 模式

通过 status line JSON 数据，计算增量并记录到数据库。

```
┌─────────────┐   HTTP(超时100ms)   ┌─────────────────┐
│ qwen-usage  │ ─────────────────► │  qwen-usage     │
│   record    │    成功/失败都返回   │    server       │
│  (CLI客户端) │ ◄───────────────── │  (后台守护进程)  │
└─────────────┘    立即输出状态行    └────────┬────────┘
       │                                      │
       │ server不可用时                        ▼
       │ 直接写入 SQLite               ┌─────────────┐
       └──────────────────────────────►│   SQLite    │
                                       └─────────────┘
```

**特点**：
- 低延迟：只输出状态行，不阻塞
- 适合高频调用场景
- 通过累计值计算增量

## 架构

- **Server**：后台 HTTP 服务，维护内存缓存累计值（`sync.Mutex` 保护），计算增量后写入 SQLite
- **CLI Client**：轻量客户端，server 不可用时自动降级直接写入 SQLite + 输出状态行
- **会话管理**：`start` 自动探测并启动 server，`stop` 减少计数，归零时 server 优雅退出
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

### 1. 会话管理（自动）

`start` 命令会自动探测 server 是否就绪，未就绪则在后台启动，然后增加会话计数：

```bash
qwen-usage start    # 自动探测/启动 server，计数 +1
qwen-usage stop     # 计数 -1，归零时 server 自动退出
```

多会话场景：每个 Qwen Code 会话启动时调用 `start`，结束时调用 `stop`，计数归零后 server 自动关闭。

如需手动管理 server：

```bash
qwen-usage server              # 前台运行
qwen-usage server &            # Linux/macOS 后台运行
qwen-usage kill                # 强制关闭
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
  },
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/qwen-usage start",
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
            "command": "/path/to/qwen-usage stop",
            "name": "qwen-usage-stop",
            "description": "Stop qwen-usage server when session ends"
          }
        ]
      }
    ]
  }
}
```

> `start` 会自动探测并启动 server，无需额外的启动脚本。

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

**报表内容**：

- **按模型统计**：请求数、延迟（Avg/P90/P95）、各类 token、缓存命中率、吞吐量
- **汇总**：总调用、总 token、平均延迟
- **上下文窗口历史最值**：

| 指标 | 值 | 模型 | 时间 |
|------|----|----|------|
| 最大上下文使用量 | 0 | - | - |
| 单次最大输入 | 0 | - | - |
| 单次最大输出 | 0 | - | - |

> 延迟百分位使用 SQL 窗口函数在数据库层计算，避免全量加载延迟数据到内存。

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

## 数据库表

### `cumulative_state` — 累计状态

用于增量计算，记录每个 `(session_id, model_name)` 的累计 API 请求、延迟、各类 token 数。

### `call_records` — 单次调用记录

每次 API 调用的增量数据，包含延迟、各类 token 数、上下文窗口状态、记录时间。按时间索引。

### 上下文窗口历史最值

不再使用单独的表，而是直接从 `call_records` 查询 `MAX(prompt_tokens)`、`MAX(completion_tokens)`、`MAX(current_usage)`，并回溯对应的模型名和时间。支持按时间范围筛选。

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
# 会话管理（自动）
qwen-usage start               # 自动探测/启动 server，增加会话计数
qwen-usage stop                # 减少会话计数，归零时 server 自动退出
qwen-usage server              # 手动启动 server（前台）
qwen-usage kill                # 强制关闭 server

# 数据记录
qwen-usage record              # 记录用量（server 不可用时自动写入本地 DB）

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