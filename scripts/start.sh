#!/bin/bash

# XGo Agent 启动脚本
# 这个脚本展示了如何安全地启动 CodeAgent 服务

set -e

# 检查必需的配置
check_required_config() {
    if [ -z "$GITHUB_TOKEN" ]; then
        echo "错误: 请设置 GITHUB_TOKEN 环境变量"
        echo "例如: export GITHUB_TOKEN='your-github-token'"
        exit 1
    fi

    if [ -z "$CLAUDE_API_KEY" ]; then
        echo "错误: 请设置 CLAUDE_API_KEY 环境变量"
        echo "例如: export CLAUDE_API_KEY='your-claude-api-key'"
        exit 1
    fi

    if [ -z "$WEBHOOK_SECRET" ]; then
        echo "错误: 请设置 WEBHOOK_SECRET 环境变量"
        echo "例如: export WEBHOOK_SECRET='your-webhook-secret'"
        exit 1
    fi
}

# 显示配置信息（不显示敏感信息）
show_config() {
    echo "=== XGo Agent 配置信息 ==="
    echo "GitHub Token: ${GITHUB_TOKEN:0:10}..."
    echo "Claude API Key: ${CLAUDE_API_KEY:0:10}..."
    echo "Webhook Secret: ${WEBHOOK_SECRET:0:10}..."
    echo "Port: ${PORT:-8888}"
    echo "=========================="
}

# 主函数
main() {
    echo "启动 XGo Agent..."
    
    # 检查配置
    check_required_config
    
    # 显示配置信息
    show_config
    
    # 启动服务
    echo "正在启动服务..."
    go run ./cmd/server \
        --github-token "$GITHUB_TOKEN" \
        --claude-api-key "$CLAUDE_API_KEY" \
        --webhook-secret "$WEBHOOK_SECRET" \
        --port "${PORT:-8888}"
}

# 运行主函数
main "$@" 