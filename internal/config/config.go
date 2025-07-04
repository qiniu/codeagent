package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig    `yaml:"server"`
	GitHub       GitHubConfig    `yaml:"github"`
	Workspace    WorkspaceConfig `yaml:"workspace"`
	Claude       ClaudeConfig    `yaml:"claude"`
	Gemini       GeminiConfig    `yaml:"gemini"`
	Docker       DockerConfig    `yaml:"docker"`
	CodeProvider string          `yaml:"code_provider"`
}

type GeminiConfig struct {
	APIKey         string        `yaml:"api_key"`
	BinPath        string        `yaml:"bin_path"`
	Timeout        time.Duration `yaml:"timeout"`
	ContainerImage string        `yaml:"container_image"`
}

type ServerConfig struct {
	Port          int    `yaml:"port"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type GitHubConfig struct {
	Token      string `yaml:"token"`
	WebhookURL string `yaml:"webhook_url"`
}

type WorkspaceConfig struct {
	BaseDir      string        `yaml:"base_dir"`
	CleanupAfter time.Duration `yaml:"cleanup_after"`
}

type ClaudeConfig struct {
	APIKey         string        `yaml:"api_key"`
	ContainerImage string        `yaml:"container_image"`
	Timeout        time.Duration `yaml:"timeout"`
	BinPath        string        `yaml:"bin_path"`
}

type DockerConfig struct {
	Socket  string `yaml:"socket"`
	Network string `yaml:"network"`
}

func Load(configPath string) (*Config, error) {
	// 首先尝试从文件加载
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		var config Config
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		// 从环境变量覆盖敏感配置
		config.loadFromEnv()

		return &config, nil
	}

	// 如果文件不存在，从环境变量创建配置
	return loadFromEnv(), nil
}

func (c *Config) loadFromEnv() {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		c.GitHub.Token = token
	}
	if apiKey := os.Getenv("CLAUDE_API_KEY"); apiKey != "" {
		c.Claude.APIKey = apiKey
	}
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		c.Gemini.APIKey = apiKey
	}
	if provider := os.Getenv("CODE_PROVIDER"); provider != "" {
		c.CodeProvider = provider
	}
	if secret := os.Getenv("WEBHOOK_SECRET"); secret != "" {
		c.Server.WebhookSecret = secret
	}
	if portStr := os.Getenv("PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			c.Server.Port = port
		}
	}
}

func loadFromEnv() *Config {
	port := 8080
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	return &Config{
		Server: ServerConfig{
			Port:          port,
			WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		},
		GitHub: GitHubConfig{
			Token:      os.Getenv("GITHUB_TOKEN"),
			WebhookURL: os.Getenv("WEBHOOK_URL"),
		},
		Workspace: WorkspaceConfig{
			BaseDir:      getEnvOrDefault("WORKSPACE_BASE_DIR", "/tmp/xgo-agent"),
			CleanupAfter: 24 * time.Hour,
		},
		Claude: ClaudeConfig{
			APIKey:         os.Getenv("CLAUDE_API_KEY"),
			ContainerImage: getEnvOrDefault("CLAUDE_IMAGE", "anthropic/claude-code:latest"),
			Timeout:        30 * time.Minute,
		},
		Gemini: GeminiConfig{
			APIKey:  os.Getenv("GEMINI_API_KEY"),
			Timeout: 30 * time.Minute,
		},
		Docker: DockerConfig{
			Socket:  getEnvOrDefault("DOCKER_SOCKET", "unix:///var/run/docker.sock"),
			Network: getEnvOrDefault("DOCKER_NETWORK", "bridge"),
		},
		CodeProvider: getEnvOrDefault("CODE_PROVIDER", "claude"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
