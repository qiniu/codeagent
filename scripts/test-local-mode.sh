#!/bin/bash

# Script to test local CLI mode

set -e

echo "🧪 Testing local CLI mode"

# Check required environment variables
if [ -z "$GITHUB_TOKEN" ]; then
    echo "❌ Error: Please set GITHUB_TOKEN environment variable"
    exit 1
fi

if [ -z "$CLAUDE_API_KEY" ] && [ -z "$GEMINI_API_KEY" ]; then
    echo "❌ Error: Please set CLAUDE_API_KEY or GEMINI_API_KEY environment variable"
    exit 1
fi

if [ -z "$WEBHOOK_SECRET" ]; then
    echo "❌ Error: Please set WEBHOOK_SECRET environment variable"
    exit 1
fi

# Set local mode
export USE_DOCKER=false

# Check CLI tools availability
if [ "$CODE_PROVIDER" = "gemini" ] || [ -z "$CODE_PROVIDER" ]; then
    if ! command -v gemini &> /dev/null; then
        echo "⚠️  Warning: gemini CLI not found, please install first"
        echo "   Installation: https://github.com/google-gemini/gemini-cli"
    else
        echo "✅ gemini CLI installed"
    fi
fi

if [ "$CODE_PROVIDER" = "claude" ] || [ -z "$CODE_PROVIDER" ]; then
    if ! command -v claude &> /dev/null; then
        echo "⚠️  Warning: claude CLI not found, please install first"
        echo "   Installation: https://github.com/anthropics/anthropic-claude-code"
    else
        echo "✅ claude CLI installed"
    fi
fi

echo "🚀 Starting local mode server..."

# Start server
go run ./cmd/server &
SERVER_PID=$!

# Wait for server to start
sleep 3

# Check if server started successfully
if ! curl -s http://localhost:8888/health > /dev/null; then
    echo "❌ Server startup failed"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

echo "✅ Server started successfully"

# Cleanup function
cleanup() {
    echo "🧹 Cleaning up resources..."
    kill $SERVER_PID 2>/dev/null || true
    wait $SERVER_PID 2>/dev/null || true
}

# Set cleanup on exit
trap cleanup EXIT

echo "📋 Test information:"
echo "   - Mode: Local CLI"
echo "   - Code provider: ${CODE_PROVIDER:-claude}"
echo "   - Port: 8888"
echo "   - Health check: http://localhost:8888/health"

echo ""
echo "🎯 Server is running, press Ctrl+C to stop testing"
echo "   You can test by:"
echo "   1. Commenting in GitHub Issue: /code implement a simple Hello World"
echo "   2. Or sending test webhook to: http://localhost:8888/hook"

# Wait for user interrupt
wait $SERVER_PID 