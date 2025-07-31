#!/bin/bash

# CodeAgent startup script - supports multiple configuration combinations

set -e

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored messages
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

# Show help information
show_help() {
    echo "CodeAgent startup script"
    echo ""
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -p, --provider PROVIDER    Code provider (claude|gemini) [default: gemini]"
    echo "  -d, --docker               Use Docker mode [default: local CLI mode]"
    echo "  -i, --interactive          Enable interactive Docker mode (Claude Docker only)"
    echo "  -h, --help                 Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                         # Gemini + local CLI mode"
    echo "  $0 -p claude -d            # Claude + Docker mode"
    echo "  $0 -p claude -d -i         # Claude + Docker interactive mode"
    echo "  $0 -p gemini -d            # Gemini + Docker mode"
    echo "  $0 -p claude               # Claude + local CLI mode"
}

# Parse command line arguments
parse_args() {
    PROVIDER="gemini"
    USE_DOCKER=false
    INTERACTIVE=false
    
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
            -i|--interactive)
                INTERACTIVE=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # Validate provider
    if [[ "$PROVIDER" != "claude" && "$PROVIDER" != "gemini" ]]; then
        print_error "Unsupported code provider: $PROVIDER"
        print_error "Supported options: claude, gemini"
        exit 1
    fi
    
    # Validate interactive mode only for Claude Docker
    if [[ "$INTERACTIVE" = true ]]; then
        if [[ "$PROVIDER" != "claude" ]]; then
            print_error "Interactive mode only supports Claude provider"
            exit 1
        fi
        if [[ "$USE_DOCKER" != true ]]; then
            print_error "Interactive mode requires Docker mode to be enabled"
            exit 1
        fi
    fi
}

# Check required environment variables
check_required_env() {
    local missing_vars=()
    
    if [ -z "$GITHUB_TOKEN" ]; then
        missing_vars+=("GITHUB_TOKEN")
    fi
    
    if [ -z "$WEBHOOK_SECRET" ]; then
        missing_vars+=("WEBHOOK_SECRET")
    fi
    
    # Check for corresponding API keys based on provider
    if [ "$PROVIDER" = "claude" ] && [ -z "$CLAUDE_API_KEY" ]; then
        missing_vars+=("CLAUDE_API_KEY")
    fi
    
    if [ "$PROVIDER" = "gemini" ] && [ -z "$GOOGLE_API_KEY" ]; then
        missing_vars+=("GOOGLE_API_KEY")
    fi
    
    if [ ${#missing_vars[@]} -ne 0 ]; then
        print_error "Missing required environment variables: ${missing_vars[*]}"
        echo ""
        echo "Please set the following environment variables:"
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

# Check CLI tools availability
check_cli_tools() {
    if [ "$USE_DOCKER" = false ]; then
        if [ "$PROVIDER" = "claude" ]; then
            print_info "Checking Claude CLI availability..."
            if ! command -v claude &> /dev/null; then
                print_error "Claude CLI not installed or not in PATH"
                echo ""
                echo "Please install Claude CLI:"
                echo "npm install -g @anthropic-ai/claude"
                exit 1
            fi
            print_success "Claude CLI available"
        else
            print_info "Checking Gemini CLI availability..."
            if ! command -v gemini &> /dev/null; then
                print_error "Gemini CLI not installed or not in PATH"
                echo ""
                echo "Please install Gemini CLI:"
                echo "npm install -g @google/generative-ai-cli"
                exit 1
            fi
            print_success "Gemini CLI available"
        fi
    else
        print_info "Checking Docker availability..."
        if ! command -v docker &> /dev/null; then
            print_error "Docker not installed or not in PATH"
            exit 1
        fi
        print_success "Docker available"
    fi
}

# Check Go environment
check_go_env() {
    print_info "Checking Go environment..."
    
    if ! command -v go &> /dev/null; then
        print_error "Go not installed or not in PATH"
        exit 1
    fi
    
    print_success "Go version: $(go version)"
}

# Set environment variables
set_env_vars() {
    export CODE_PROVIDER="$PROVIDER"
    export USE_DOCKER="$USE_DOCKER"
    export CLAUDE_INTERACTIVE="$INTERACTIVE"
    export PORT=${PORT:-8888}
    
    print_info "Setting environment variables:"
    print_info "  CODE_PROVIDER=$PROVIDER"
    print_info "  USE_DOCKER=$USE_DOCKER"
    print_info "  CLAUDE_INTERACTIVE=$INTERACTIVE"
    print_info "  PORT=$PORT"
}

# Start server
start_server() {
    print_info "Starting CodeAgent server..."
    
    # Build command
    cmd="go run ./cmd/server"
    
    # Add port parameter
    if [ ! -z "$PORT" ]; then
        cmd="$cmd --port $PORT"
    fi
    
    # Add config file parameter (if exists)
    if [ -f "config.yaml" ]; then
        cmd="$cmd --config config.yaml"
        print_info "Using config file: config.yaml"
    fi
    
    print_info "Executing command: $cmd"
    echo ""
    
    # Execute command
    eval $cmd
}

# Main function
main() {
    echo "=========================================="
    echo "  CodeAgent Launcher"
    echo "=========================================="
    echo ""
    
    # Parse command line arguments
    parse_args "$@"
    
    # Display configuration information
    print_info "Configuration:"
    print_info "  Code provider: $PROVIDER"
    print_info "  Execution mode: $([ "$USE_DOCKER" = true ] && echo "Docker" || echo "Local CLI")"
    if [[ "$INTERACTIVE" = true ]]; then
        print_info "  Mode: Interactive"
    fi
    echo ""
    
    # Check environment
    check_go_env
    check_cli_tools
    check_required_env
    
    # Set environment variables
    set_env_vars
    
    echo ""
    print_success "Environment check complete, preparing to start server..."
    echo ""
    
    # Start server
    start_server
}

# Run main function
main "$@" 