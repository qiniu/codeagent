# CodeAgent Makefile

# Variable definitions
BINARY_NAME=codeagent
BUILD_DIR=bin
MAIN_PATH=./cmd/server

# Default target
.PHONY: all
all: build

# Build target
.PHONY: build
build:
	@echo "Building CodeAgent..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean target
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean completed"

# Run target
.PHONY: run
run: build
	@echo "Running CodeAgent..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Test target
.PHONY: test
test:
	@echo "Running tests..."
	go test ./...

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build  - Build the CodeAgent binary"
	@echo "  clean  - Clean build artifacts"
	@echo "  run    - Build and run CodeAgent"
	@echo "  test   - Run tests"
	@echo "  help   - Show this help message"
