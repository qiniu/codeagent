# CodeAgent Makefile

# 变量定义
BINARY_NAME=codeagent
BUILD_DIR=bin
MAIN_PATH=./cmd/server

# 默认目标
.PHONY: all
all: build

# 构建目标
.PHONY: build
build:
	@echo "Building CodeAgent..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# 清理目标
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean completed"

# 运行目标
.PHONY: run
run: build
	@echo "Running CodeAgent..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# 测试目标
.PHONY: test
test:
	@echo "Running tests..."
	go test ./...

# 帮助目标
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build  - Build the CodeAgent binary"
	@echo "  clean  - Clean build artifacts"
	@echo "  run    - Build and run CodeAgent"
	@echo "  test   - Run tests"
	@echo "  help   - Show this help message"
