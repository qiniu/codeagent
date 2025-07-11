#!/bin/bash

# CodeAgent 启动脚本 - 支持多种配置组合

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助信息
show_help() {
    echo "CodeAgent 启动脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -p, --provider PROVIDER    代码提供者 (claude|gemini) [默认: gemini]"
    echo "  -d, --docker               使用 Docker 模式 [默认: 本地 CLI 模式]"
    echo "  -h, --help                 显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                          # Gemini + 本地 CLI 模式"
    echo "  $0 -p claude -d             # Claude + Docker 模式"
    echo "  $0 -p gemini -d             # Gemini + Docker 模式"
    echo "  $0 -p claude                # Claude + 本地 CLI 模式"
}

# 解析命令行参数
parse_args() {
    PROVIDER="gemini"
    USE_DOCKER=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -p|--provider)
                PROVIDER="$2"
                shift 2
                ;;
            -d|--docker)
                USE_DOCKER=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                print_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # 验证 provider
    if [[ "$PROVIDER" != "claude" && "$PROVIDER" != "gemini" ]]; then
        print_error "不支持的代码提供者: $PROVIDER"
        print_error "支持的选项: claude, gemini"
        exit 1
    fi
}

# 检查必需的环境变量
check_required_env() {
    local missing_vars=()
    
    if [ -z "$GITHUB_TOKEN" ]; then
        missing_vars+=("GITHUB_TOKEN")
    fi
    
    if [ -z "$WEBHOOK_SECRET" ]; then
        missing_vars+=("WEBHOOK_SECRET")
    fi
    
    # 根据 provider 检查相应的 API 密钥
    if [ "$PROVIDER" = "claude" ] && [ -z "$CLAUDE_API_KEY" ]; then
        missing_vars+=("CLAUDE_API_KEY")
    fi
    
    if [ "$PROVIDER" = "gemini" ] && [ -z "$GOOGLE_API_KEY" ]; then
        missing_vars+=("GOOGLE_API_KEY")
    fi
    
    if [ ${#missing_vars[@]} -ne 0 ]; then
        print_error "缺少必需的环境变量: ${missing_vars[*]}"
        echo ""
        echo "请设置以下环境变量:"
        echo "export GITHUB_TOKEN=\"your-github-token\""
        echo "export WEBHOOK_SECRET=\"your-webhook-secret\""
        if [ "$PROVIDER" = "claude" ]; then
            echo "export CLAUDE_API_KEY=\"your-claude-api-key\""
        else
            echo "export GOOGLE_API_KEY=\"your-google-api-key\""
        fi
        exit 1
    fi
}

# 检查 CLI 工具是否可用
check_cli_tools() {
    if [ "$USE_DOCKER" = false ]; then
        if [ "$PROVIDER" = "claude" ]; then
            print_info "检查 Claude CLI 是否可用..."
            if ! command -v claude &> /dev/null; then
                print_error "Claude CLI 未安装或不在 PATH 中"
                echo ""
                echo "请安装 Claude CLI:"
                echo "npm install -g @anthropic-ai/claude-code"
                exit 1
            fi
            print_success "Claude CLI 可用"
        else
            print_info "检查 Gemini CLI 是否可用..."
            if ! command -v gemini &> /dev/null; then
                print_error "Gemini CLI 未安装或不在 PATH 中"
                echo ""
                echo "请安装 Gemini CLI:"
                echo "npm install -g @google/gemini-cli"
                exit 1
            fi
            print_success "Gemini CLI 可用"
        fi
    else
        print_info "检查 Docker 是否可用..."
        if ! command -v docker &> /dev/null; then
            print_error "Docker 未安装或不在 PATH 中"
            exit 1
        fi
        print_success "Docker 可用"
    fi
}

# 检查 Go 环境
check_go_env() {
    print_info "检查 Go 环境..."
    
    if ! command -v go &> /dev/null; then
        print_error "Go 未安装或不在 PATH 中"
        exit 1
    fi
    
    print_success "Go 版本: $(go version)"
}

# 设置环境变量
set_env_vars() {
    export CODE_PROVIDER="$PROVIDER"
    export USE_DOCKER="$USE_DOCKER"
    export PORT=${PORT:-8888}
    
    print_info "设置环境变量:"
    print_info "  CODE_PROVIDER=$PROVIDER"
    print_info "  USE_DOCKER=$USE_DOCKER"
    print_info "  PORT=$PORT"
}

# 启动服务器
start_server() {
    print_info "启动 CodeAgent 服务器..."
    
    # 构建命令
    cmd="go run ./cmd/server"
    
    # 添加端口参数
    if [ ! -z "$PORT" ]; then
        cmd="$cmd --port $PORT"
    fi
    
    # 添加配置文件参数（如果存在）
    if [ -f "config.yaml" ]; then
        cmd="$cmd --config config.yaml"
        print_info "使用配置文件: config.yaml"
    fi
    
    print_info "执行命令: $cmd"
    echo ""
    
    # 执行命令
    eval $cmd
}

# 主函数
main() {
    echo "=========================================="
    echo "  CodeAgent 启动器"
    echo "=========================================="
    echo ""
    
    # 解析命令行参数
    parse_args "$@"
    
    # 显示配置信息
    print_info "配置信息:"
    print_info "  代码提供者: $PROVIDER"
    print_info "  执行方式: $([ "$USE_DOCKER" = true ] && echo "Docker" || echo "本地 CLI")"
    echo ""
    
    # 检查环境
    check_go_env
    check_cli_tools
    check_required_env
    
    # 设置环境变量
    set_env_vars
    
    echo ""
    print_success "环境检查完成，准备启动服务器..."
    echo ""
    
    # 启动服务器
    start_server
}

# 运行主函数
main "$@" 