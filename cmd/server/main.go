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

	"github.com/qbox/codeagent/internal/agent"
	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/internal/webhook"
	"github.com/qbox/codeagent/internal/workspace"

	"github.com/qiniu/x/log"
)

func main() {
	// 定义命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	githubToken := flag.String("github-token", "", "GitHub Token (也可以通过 GITHUB_TOKEN 环境变量设置)")
	claudeAPIKey := flag.String("claude-api-key", "", "Claude API Key (也可以通过 CLAUDE_API_KEY 环境变量设置)")
	geminiAPIKey := flag.String("gemini-api-key", "", "Gemini API Key (也可以通过 GEMINI_API_KEY 环境变量设置)")
	webhookSecret := flag.String("webhook-secret", "", "Webhook Secret (也可以通过 WEBHOOK_SECRET 环境变量设置)")
	port := flag.Int("port", 0, "服务器端口 (也可以通过 PORT 环境变量设置)")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 命令行参数优先级高于环境变量和配置文件
	if *githubToken != "" {
		cfg.GitHub.Token = *githubToken
	}
	if *claudeAPIKey != "" {
		cfg.Claude.APIKey = *claudeAPIKey
	}
	if *geminiAPIKey != "" {
		cfg.Gemini.APIKey = *geminiAPIKey
	}
	if *webhookSecret != "" {
		cfg.Server.WebhookSecret = *webhookSecret
	}
	if *port > 0 {
		cfg.Server.Port = *port
	}

	// 打印最终配置用于调试（已脱敏）
	loggableConfig := *cfg
	if loggableConfig.GitHub.Token != "" {
		loggableConfig.GitHub.Token = "***REDACTED***"
	}
	if loggableConfig.Claude.APIKey != "" {
		loggableConfig.Claude.APIKey = "***REDACTED***"
	}
	if loggableConfig.Gemini.APIKey != "" {
		loggableConfig.Gemini.APIKey = "***REDACTED***"
	}
	if loggableConfig.Server.WebhookSecret != "" {
		loggableConfig.Server.WebhookSecret = "***REDACTED***"
	}
	configJSON, err := json.MarshalIndent(loggableConfig, "", "  ")
	if err != nil {
		log.Warnf("Could not marshal config to JSON for logging: %v", err)
	} else {
		log.Infof("Final configuration:\n%s", string(configJSON))
	}

	// 验证必需的配置
	if cfg.GitHub.Token == "" {
		log.Fatalf("GitHub Token is required. Please set it via --github-token flag or GITHUB_TOKEN environment variable")
	}
	if cfg.Server.WebhookSecret == "" {
		log.Fatalf("Webhook Secret is required. Please set it via --webhook-secret flag or WEBHOOK_SECRET environment variable")
	}
	switch cfg.CodeProvider {
	case "claude":
		if cfg.Claude.APIKey == "" {
			log.Fatalf("Claude API Key is required when code_provider is 'claude'. Please set it via --claude-api-key flag, CLAUDE_API_KEY environment variable, or in the config file.")
		}
	case "gemini":
		if cfg.Gemini.APIKey == "" {
			log.Fatalf("Gemini API Key is required when code_provider is 'gemini'. Please set it via --gemini-api-key flag, GEMINI_API_KEY environment variable, or in the config file.")
		}
	default:
		log.Fatalf("Invalid or missing code_provider: '%s'. Must be 'claude' or 'gemini'", cfg.CodeProvider)
	}

	log.Infof("Configuration validated successfully")

	// 初始化工作空间管理器
	workspaceManager := workspace.NewManager(cfg)

	// 初始化 Agent
	agent := agent.New(cfg, workspaceManager)

	// 初始化 Webhook 处理器
	webhookHandler := webhook.NewHandler(cfg, agent)

	// 设置路由
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", webhookHandler.HandleWebhook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// 返回健康状态和工作空间信息
		status := map[string]interface{}{
			"status":          "OK",
			"workspace_count": workspaceManager.GetWorkspaceCount(),
			"timestamp":       time.Now().Format(time.RFC3339),
		}

		json.NewEncoder(w).Encode(status)
	})

	// 创建 HTTP 服务器
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Server.Port),
		Handler: mux,
	}

	// 启动服务器
	go func() {
		log.Infof("Starting server on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Infof("Shutting down server...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Infof("Server exited")
}
