# qwen-usage

Qwen Code 用量统计工具。通过 Status Line JSON 数据，计算增量并记录到数据库。

## 快速开始

### Windows（推荐）

```bash
# 1. 构建
make build-windows

# 2. 把 qwen-usage.exe 放到目标目录，然后执行：
qwen-usage install              # 生成 config.json + start_server.vbs
qwen-usage install -a           # 同上，并创建开机自启动符号链接

# 3. 验证
qwen-usage status               # 查询 server 状态
qwen-usage stop                 # 优雅停止 server
qwen-usage uninstall            # 移除开机自启动（保留配置和脚本文件）
```

### Linux / macOS

使用 hooks 方式管理 server 生命周期（见下方"配置 Qwen Code"）。

## 项目结构

```
qwen-usage/
├── cmd/
│   └── main.go                     # CLI 入口
├── internal/
│   ├── config/
│   │   ├── config.go               # 配置管理（exe 目录优先）
│   │   ├── embed.go                # go:embed 默认配置
│   │   ├── default_config.json     # 嵌入的默认配置
│   │   └── config_test.go
│   ├── database/
│   │   ├── db.go                   # 数据库接口
│   │   ├── sqlite.go               # SQLite 实现
│   │   ├── models.go               # 数据模型
│   │   └── database_test.go
│   ├── server/
│   │   └── server.go               # HTTP 服务
│   ├── commands/
│   │   ├── record.go               # record 子命令
│   │   └── export.go               # export/clear 子命令
│   └── logger/
│       ├── logger.go               # 日志系统
│       └── logger_test.go
├── pkg/
│   └── platform/
│       ├── platform.go             # 跨平台公共逻辑
│       ├── platform_windows.go     # Windows 进程分离
│       ├── platform_unix.go        # Unix 进程分离
│       ├── service_windows.go      # Windows 服务管理（install/status/stop/uninstall）
│       └── service_stub.go         # 非 Windows 平台桩
├── go.mod
├── Makefile
└── README.md
```

## 命令

### 服务管理

| 命令 | 说明 | 幂等 |
|------|------|------|
| `install` | 在 exe 目录生成 config.json + start_server.vbs | ✅ 已存在则跳过 |
| `install -a` | 同上，并在开机启动目录创建 VBS 符号链接 | ✅ 链接已存在则跳过 |
| `status` | 查询 server 运行状态、地址、PID | ✅ |
| `stop` | 优雅停止 server，清理 PID 文件 | ✅ server 未运行时正常返回 |
| `uninstall` | 移除开机启动目录的符号链接 | ✅ 不存在时正常返回 |
| `server` | 前台启动 server（通常由 VBS 脚本调用） | — |
| `version` | 显示构建版本号（git tag） | — |

### 数据记录

| 命令 | 说明 |
|------|------|
| `record < input.json` | 从 stdin 读取 Status Line JSON，记录用量并输出状态行 |
| `record -s < input.json` | 只输出状态行，不记录到数据库 |

### 数据导出与清理

| 命令 | 说明 |
|------|------|
| `export` | 今日统计报表 |
| `export -period week` | 最近一周报表 |
| `export -period 5h` | 最近 5 小时报表 |
| `export -n 20` | 最近 20 条记录 |
| `export -json` | JSON 格式输出 |
| `clear` | 清理 31 天前数据 |
| `clear -days 7` | 清理 7 天前数据 |

每个子命令都支持 `-h` 查看详细帮助。

## 安装流程详解

### `qwen-usage install`

在 exe 所在目录生成两个文件：

1. **config.json** — 默认配置（从 exe 内嵌数据写入，不覆盖已有文件）
2. **start_server.vbs** — VBS 启动脚本，隐藏窗口运行 `qwen-usage server`

### `qwen-usage install -a`

在 install 基础上，额外执行：

3. **符号链接** — 在 `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\` 创建 `qwen-usage.vbs` 指向 exe 目录的 `start_server.vbs`

> 创建符号链接需要**管理员权限**或开启 **Windows 开发者模式**。

### 配置加载顺序

```
exe 目录/config.json  →  优先
     ↓ 不存在时
~/.qwen/usage/config.json  →  回退
     ↓ 不存在时
内置默认值
```

### 默认配置

```json
{
  "server_addr": "127.0.0.1:9527",
  "db_path": "~/.qwen/usage/usage.db",
  "client_timeout_ms": 100,
  "server_write_timeout_ms": 50
}
```

## 配置 Qwen Code Status Line

在 `~/.qwen/settings.json` 中添加：

```json
{
  "ui": {
    "statusLine": {
      "type": "command",
      "command": "input=$(cat); /path/to/qwen-usage record <<< \"$input\""
    }
  }
}
```

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Qwen Code 会话生命周期                      │
│  Status Line  ──► qwen-usage record < input.json             │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    CLI (cmd/main.go)                          │
│  record ──► POST /record (失败降级为本地 SQLite)              │
│  export ──► 直接读 SQLite 输出报表                             │
│  clear  ──► 直接操作 SQLite 删除旧数据                         │
│  install/uninstall/status/stop ──► 服务管理                    │
└─────────────────────────────────────────────────────────────┘
              │                              │
              ▼                              ▼
┌─────────────────────────┐    ┌─────────────────────────────┐
│   HTTP Server (:9527)    │    │   本地降级 (record 命令)     │
│  增量计算 → SQLite        │    │   直接写入 SQLite           │
└─────────────────────────┘    └─────────────────────────────┘
```

- **增量计算**：通过 `cumulative_state` 表记录上次累计值，每次计算 delta
- **双路径写入**：server 可用时走 HTTP，不可用时本地直接写 SQLite
- **纯 Go SQLite**：`modernc.org/sqlite`，无需 CGO，跨平台零配置

## 构建

```bash
make build            # 当前平台
make build-windows    # Windows amd64
make build-linux      # Linux amd64
make build-mac        # macOS amd64
make build-all        # 所有平台

# 指定版本号构建（注入到 qwen-usage version）
make build-windows VERSION=v1.2.3

# 安装到 GOPATH/bin
make install
```

## 测试

```bash
make test
make test-coverage
```
