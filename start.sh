#!/bin/bash
# Qwen Usage Server Session Start Hook
# 在 Qwen Code 会话开始时自动启动 server 并增加会话计数
# 
# 用法：
#   bash start.sh
# 或在 settings.json 中配置为 SessionStart hook 命令

set -euo pipefail

# ---------------------------------------------------------------
# 可执行文件路径（可通过环境变量覆盖）
# ---------------------------------------------------------------
# qwen-usage 可执行文件路径，默认指向 ~/.local/bin/qwen-usage
: "${QWEN_USAGE_EXE:="${HOME}/.local/bin/qwen-usage"}"

# 可选的 websearch 回退可执行文件路径
: "${WEBSEARCH_EXE:="websearch"}"

# ---------------------------------------------------------------
# 服务端与配置路径
# ---------------------------------------------------------------
: "${SERVER_ADDR:="127.0.0.1:9527"}"
: "${CONFIG_PATH:="${HOME}/.qwen/websearch/config.yaml"}"

# 检查 server 健康端点
_check_server() {
    curl -s --connect-timeout 1 "http://${SERVER_ADDR}/health" > /dev/null 2>&1
}

# 如果 server 未运行，后台启动它
if ! _check_server; then
    echo "[qwen-usage] Starting server..."
    nohup "$QWEN_USAGE_EXE" server > /dev/null 2>&1 &
    sleep 1
fi

# 增加会话计数（优先使用 qwen-usage，失败时回退到 websearch）
"$QWEN_USAGE_EXE" start 2>/dev/null || "$WEBSEARCH_EXE" -c "$CONFIG_PATH" start 2>/dev/null
