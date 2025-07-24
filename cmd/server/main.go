package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"encoding/json"

	"github.com/google/go-github/v62/github"
	"github.com/jicarl/codeagent/internal/agent"
	"github.com/jicarl/codeagent/internal/code"
	"github.com/jicarl/codeagent/internal/config"
	gh "github.com/jicarl/codeagent/internal/github"
	"github.com/jicarl/codeagent/internal/webhook"
	"github.com/jicarl/codeagent/internal/workspace"
	"github.com/jicarl/codeagent/pkg/signature"
	"go.uber.org/zap"

	"github.com/qiniu/x/log"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 打印加载的配置
	configBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config to JSON: %v", err)
	}
	log.Printf("Loaded configuration:\n%s", string(configBytes))

	// Set up logger
	var logger *zap.Logger
	if os.Getenv("APP_ENV") == "production" {
