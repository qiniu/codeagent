#!/bin/bash

echo "CodeAgent 权限测试脚本"

# 检查 codeagent 用户
if id "codeagent" &>/dev/null; then
    echo "✓ codeagent 用户存在"
else
    echo "✗ codeagent 用户不存在"
    echo "请运行: sudo useradd -g codeagent -m codeagent"
fi

# 检查工作空间目录权限
WORKSPACE_DIR="/tmp/xgo-agent"
if [ -d "$WORKSPACE_DIR" ]; then
    OWNER=$(stat -c '%U' "$WORKSPACE_DIR")
    if [ "$OWNER" = "codeagent" ]; then
        echo "✓ 工作空间目录权限正确"
    else
        echo "✗ 工作空间目录权限不正确"
        echo "请运行: sudo chown -R codeagent:codeagent $WORKSPACE_DIR"
    fi
fi