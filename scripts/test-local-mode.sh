#!/bin/bash

# 测试本地 CLI 模式的脚本

set -e

echo "🧪 测试本地 CLI 模式"

# 检查必要的环境变量
if [ -z "$GITHUB_TOKEN" ]; then
    echo "❌ 错误: 请设置 GITHUB_TOKEN 环境变量"
    exit 1
fi

if [ -z "$CLAUDE_API_KEY" ] && [ -z "$GEMINI_API_KEY" ]; then
    echo "❌ 错误: 请设置 CLAUDE_API_KEY 或 GEMINI_API_KEY 环境变量"
    exit 1
fi

if [ -z "$WEBHOOK_SECRET" ]; then
    echo "❌ 错误: 请设置 WEBHOOK_SECRET 环境变量"
    exit 1
fi

# 设置本地模式
export USE_DOCKER=false

# 检查 CLI 工具是否可用
if [ "$CODE_PROVIDER" = "gemini" ] || [ -z "$CODE_PROVIDER" ]; then
    if ! command -v gemini &> /dev/null; then
        echo "⚠️  警告: gemini CLI 未找到，请先安装"
        echo "   安装方法: https://github.com/google-gemini/gemini-cli"
    else
        echo "✅ gemini CLI 已安装"
    fi
fi

if [ "$CODE_PROVIDER" = "claude" ] || [ -z "$CODE_PROVIDER" ]; then
    if ! command -v claude &> /dev/null; then
        echo "⚠️  警告: claude CLI 未找到，请先安装"
        echo "   安装方法: https://github.com/anthropics/anthropic-claude-code"
    else
        echo "✅ claude CLI 已安装"
    fi
fi

echo "🚀 启动本地模式服务器..."

# 启动服务器
go run ./cmd/server &
SERVER_PID=$!

# 等待服务器启动
sleep 3

# 检查服务器是否启动成功
if ! curl -s http://localhost:8888/health > /dev/null; then
    echo "❌ 服务器启动失败"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

echo "✅ 服务器启动成功"

# 清理函数
cleanup() {
    echo "🧹 清理资源..."
    kill $SERVER_PID 2>/dev/null || true
    wait $SERVER_PID 2>/dev/null || true
}

# 设置退出时的清理
trap cleanup EXIT

echo "📋 测试信息:"
echo "   - 模式: 本地 CLI"
echo "   - 代码提供者: ${CODE_PROVIDER:-claude}"
echo "   - 端口: 8888"
echo "   - 健康检查: http://localhost:8888/health"

echo ""
echo "🎯 服务器正在运行，按 Ctrl+C 停止测试"
echo "   你可以通过以下方式测试:"
echo "   1. 在 GitHub Issue 中评论: /code 实现一个简单的 Hello World"
echo "   2. 或者发送测试 Webhook 到: http://localhost:8888/hook"

# 等待用户中断
wait $SERVER_PID 