#!/bin/bash

# CodeAgent 本地模式设置验证脚本

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

print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# 验证 Go 环境
check_go() {
    print_header "检查 Go 环境"
    
    if ! command -v go &> /dev/null; then
        print_error "Go 未安装或不在 PATH 中"
        return 1
    fi
    
    go_version=$(go version | grep -o 'go[0-9]\+\.[0-9]\+')
    print_success "Go 版本: $go_version"
    
    if ! go mod verify &> /dev/null; then
        print_warning "Go 模块验证失败，尝试下载依赖..."
        go mod download
    fi
    
    print_success "Go 环境检查通过"
}

# 验证 CLI 工具
check_cli_tools() {
    print_header "检查 CLI 工具"
    
    local claude_available=false
    local gemini_available=false
    
    if command -v claude &> /dev/null; then
        claude_available=true
        print_success "Claude CLI 已安装"
    else
        print_warning "Claude CLI 未安装"
        print_info "安装方法: npm install -g @anthropic-ai/claude-code"
    fi
    
    if command -v gemini &> /dev/null; then
        gemini_available=true
        print_success "Gemini CLI 已安装"
    else
        print_warning "Gemini CLI 未安装"
        print_info "安装方法: npm install -g @google/gemini-cli"
    fi
    
    if [ "$claude_available" = false ] && [ "$gemini_available" = false ]; then
        print_error "至少需要安装一个 CLI 工具 (Claude 或 Gemini)"
        return 1
    fi
    
    print_success "CLI 工具检查通过"
}

# 验证环境变量
check_env_vars() {
    print_header "检查环境变量"
    
    local missing_vars=()
    
    # 检查必需的环境变量
    if [ -z "$GITHUB_TOKEN" ]; then
        missing_vars+=("GITHUB_TOKEN")
    else
        print_success "GITHUB_TOKEN 已设置"
    fi
    
    if [ -z "$WEBHOOK_SECRET" ]; then
        missing_vars+=("WEBHOOK_SECRET")
    else
        print_success "WEBHOOK_SECRET 已设置"
    fi
    
    # 检查 API 密钥
    local has_claude_key=false
    local has_gemini_key=false
    
    if [ ! -z "$CLAUDE_API_KEY" ]; then
        has_claude_key=true
        print_success "CLAUDE_API_KEY 已设置"
    fi
    
    if [ ! -z "$GOOGLE_API_KEY" ] || [ ! -z "$GEMINI_API_KEY" ]; then
        has_gemini_key=true
        print_success "Gemini API Key 已设置"
    fi
    
    if [ "$has_claude_key" = false ] && [ "$has_gemini_key" = false ]; then
        missing_vars+=("CLAUDE_API_KEY 或 GOOGLE_API_KEY")
    fi
    
    # 显示可选的环境变量
    print_info "可选环境变量："
    print_info "  CODE_PROVIDER: ${CODE_PROVIDER:-claude} (默认)"
    print_info "  USE_DOCKER: ${USE_DOCKER:-false} (默认)"
    print_info "  PORT: ${PORT:-8888} (默认)"
    
    if [ ${#missing_vars[@]} -ne 0 ]; then
        print_error "缺少必需的环境变量: ${missing_vars[*]}"
        return 1
    fi
    
    print_success "环境变量检查通过"
}

# 验证配置文件
check_config() {
    print_header "检查配置文件"
    
    if [ -f "config.yaml" ]; then
        print_success "找到配置文件: config.yaml"
    else
        print_warning "未找到配置文件，将使用环境变量配置"
    fi
    
    if [ -f "cmd/server/config.yaml" ]; then
        print_success "找到示例配置文件: cmd/server/config.yaml"
    fi
    
    print_success "配置文件检查通过"
}

# 构建测试
check_build() {
    print_header "构建测试"
    
    print_info "开始构建..."
    if go build -o bin/codeagent-test ./cmd/server; then
        print_success "构建成功"
        rm -f bin/codeagent-test
    else
        print_error "构建失败"
        return 1
    fi
    
    print_success "构建测试通过"
}

# 生成启动命令
generate_startup_commands() {
    print_header "推荐启动命令"
    
    echo ""
    print_info "基于当前环境，推荐使用以下命令启动："
    echo ""
    
    # 确定代码提供者
    local provider="claude"
    if [ ! -z "$CODE_PROVIDER" ]; then
        provider="$CODE_PROVIDER"
    elif [ -z "$CLAUDE_API_KEY" ] && [ ! -z "$GOOGLE_API_KEY" ]; then
        provider="gemini"
    fi
    
    # 确定执行方式
    local use_docker="false"
    if [ ! -z "$USE_DOCKER" ]; then
        use_docker="$USE_DOCKER"
    fi
    
    echo -e "${GREEN}✓ 推荐命令：${NC}"
    if [ "$use_docker" = "true" ]; then
        echo "   ./scripts/start.sh -p $provider -d"
    else
        echo "   ./scripts/start.sh -p $provider"
    fi
    echo ""
    
    echo -e "${BLUE}ℹ 其他选项：${NC}"
    echo "   go run ./cmd/server"
    echo "   make build && ./bin/codeagent"
    echo "   ./scripts/test-local-mode.sh"
    echo ""
}

# 主函数
main() {
    print_header "CodeAgent 本地模式设置验证"
    echo ""
    
    local failed_checks=()
    
    # 执行各项检查
    if ! check_go; then
        failed_checks+=("Go环境")
    fi
    echo ""
    
    if ! check_cli_tools; then
        failed_checks+=("CLI工具")
    fi
    echo ""
    
    if ! check_env_vars; then
        failed_checks+=("环境变量")
    fi
    echo ""
    
    if ! check_config; then
        failed_checks+=("配置文件")
    fi
    echo ""
    
    if ! check_build; then
        failed_checks+=("构建测试")
    fi
    echo ""
    
    # 显示结果
    if [ ${#failed_checks[@]} -eq 0 ]; then
        print_success "所有检查都通过了！"
        generate_startup_commands
    else
        print_error "以下检查失败: ${failed_checks[*]}"
        print_error "请修复这些问题后再尝试启动 CodeAgent"
        return 1
    fi
}

# 运行主函数
main "$@"