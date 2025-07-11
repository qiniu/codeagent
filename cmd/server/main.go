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
	port := flag.Int("port", 0, "服务器端口 (也可以通过 PORT 环境变量设置)")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 环境变量会覆盖配置文件中的设置
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		cfg.GitHub.Token = token
	}
	if key := os.Getenv("CLAUDE_API_KEY"); key != "" {
		cfg.Claude.APIKey = key
	}
	if secret := os.Getenv("WEBHOOK_SECRET"); secret != "" {
		cfg.Server.WebhookSecret = secret
	}
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Server.Port = p
		}
	}

	// 命令行参数优先级最高
	if *port > 0 {
		cfg.Server.Port = *port
	}

	// 验证必需的配置
	if cfg.GitHub.Token == "" {
		log.Fatalf("GitHub Token is required. Please set it via config file or GITHUB_TOKEN environment variable")
	}
	if cfg.Server.WebhookSecret == "" {
		log.Fatalf("Webhook Secret is required. Please set it via config file or WEBHOOK_SECRET environment variable")
	}

	log.Infof("Configuration validated successfully")

	// 打印加载的配置 (for debugging)
	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Warnf("Could not marshal config to JSON for printing: %v", err)
	} else {
		log.Infof("Loaded configuration:\n%s", string(configJSON))
	}

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
		Addr:         ":" + strconv.Itoa(cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second, // 防止慢速客户端攻击
		WriteTimeout: 15 * time.Second, // 防止慢速客户端攻击
		IdleTimeout:  60 * time.Second, // 释放空闲连接
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
