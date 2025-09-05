package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig    `yaml:"server"`
	GitHub       GitHubConfig    `yaml:"github"`
	Workspace    WorkspaceConfig `yaml:"workspace"`
	Claude       ClaudeConfig    `yaml:"claude"`
	Gemini       GeminiConfig    `yaml:"gemini"`
	Docker       DockerConfig    `yaml:"docker"`
	CodeProvider string          `yaml:"code_provider"`
	UseDocker    bool            `yaml:"use_docker"`

	// v0.6 Configuration
	Commands CommandsConfig `yaml:"commands"`
	// AI Mention Configuration
	Mention MentionConfig `yaml:"mention"`
	// Review Configuration
	Review ReviewConfig `yaml:"review"`
}

type GeminiConfig struct {
	APIKey             string        `yaml:"api_key"`
	Timeout            time.Duration `yaml:"timeout"`
	ContainerImage     string        `yaml:"container_image"`
	GoogleCloudProject string        `yaml:"google_cloud_project"`
}

type ServerConfig struct {
	Port          int    `yaml:"port"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type GitHubConfig struct {
	Token      string          `yaml:"token"`
	WebhookURL string          `yaml:"webhook_url"`
	GHToken    string          `yaml:"gh_token"`
	App        GitHubAppConfig `yaml:"app"`
	API        GitHubAPIConfig `yaml:"api"`
}

type GitHubAPIConfig struct {
	// GraphQL configuration
	UseGraphQL         bool `yaml:"use_graphql"`          // Whether to use GraphQL API instead of REST
	GraphQLFallback    bool `yaml:"graphql_fallback"`     // Fallback to REST API if GraphQL fails
	RateLimitThreshold int  `yaml:"rate_limit_threshold"` // Warn when remaining requests below this threshold
	// Rate limiting
	EnableRateMonitoring bool `yaml:"enable_rate_monitoring"` // Enable rate limit monitoring and logging
}

type GitHubAppConfig struct {
	AppID          int64  `yaml:"app_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	PrivateKeyEnv  string `yaml:"private_key_env"`
	PrivateKey     string `yaml:"private_key"`
}

type WorkspaceConfig struct {
	BaseDir      string        `yaml:"base_dir"`
	CleanupAfter time.Duration `yaml:"cleanup_after"`
}

type ClaudeConfig struct {
	APIKey         string        `yaml:"api_key"`
	AuthToken      string        `yaml:"auth_token"`
	BaseURL        string        `yaml:"base_url"`
	ContainerImage string        `yaml:"container_image"`
	Timeout        time.Duration `yaml:"timeout"`
	Interactive    bool          `yaml:"interactive"`
}

type DockerConfig struct {
	Socket  string `yaml:"socket"`
	Network string `yaml:"network"`
}

type CommandsConfig struct {
	GlobalPath string `yaml:"global_path"`
}

type MentionConfig struct {
	// 可配置的mention目标，支持多个
	Triggers []string `yaml:"triggers"`
	// 默认的mention目标（向后兼容）
	DefaultTrigger string `yaml:"default_trigger"`
}

type ReviewConfig struct {
	// 自动审查的排除账号，支持多个
	ExcludedAccounts []string `yaml:"excluded_accounts"`
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

		// 将相对路径转换为绝对路径
		if err := config.resolvePaths(filepath.Dir(configPath)); err != nil {
			return nil, fmt.Errorf("failed to resolve paths: %w", err)
		}

		return &config, nil
	}

	// 如果文件不存在，从环境变量创建配置
	config := loadFromEnv()
	// 将相对路径转换为绝对路径（相对于当前工作目录）
	if err := config.resolvePaths("."); err != nil {
		return nil, fmt.Errorf("failed to resolve paths: %w", err)
	}
	return config, nil
}

func (c *Config) loadFromEnv() {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		c.GitHub.Token = token
	}
	if ghToken := os.Getenv("GH_TOKEN"); ghToken != "" {
		c.GitHub.GHToken = ghToken
	}
	if apiKey := os.Getenv("CLAUDE_API_KEY"); apiKey != "" {
		c.Claude.APIKey = apiKey
	}
	if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
		c.Claude.BaseURL = baseURL
	}
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		c.Claude.APIKey = apiKey
	}
	if authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN"); authToken != "" {
		c.Claude.AuthToken = authToken
	}
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		c.Gemini.APIKey = apiKey
	}
	if project := os.Getenv("GOOGLE_CLOUD_PROJECT"); project != "" {
		c.Gemini.GoogleCloudProject = project
	}
	if provider := os.Getenv("CODE_PROVIDER"); provider != "" {
		c.CodeProvider = provider
	} else {
		// 必须要存在一个 provider，这里默认使用 gemini
		c.CodeProvider = "gemini"
	}
	if secret := os.Getenv("WEBHOOK_SECRET"); secret != "" {
		c.Server.WebhookSecret = secret
	}
	// GitHub App configuration from environment
	if appIDStr := os.Getenv("GITHUB_APP_ID"); appIDStr != "" {
		if appID, err := strconv.ParseInt(appIDStr, 10, 64); err == nil {
			c.GitHub.App.AppID = appID
		}
	}
	if privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); privateKeyPath != "" {
		c.GitHub.App.PrivateKeyPath = privateKeyPath
	}
	if privateKeyEnv := os.Getenv("GITHUB_APP_PRIVATE_KEY_ENV"); privateKeyEnv != "" {
		c.GitHub.App.PrivateKeyEnv = privateKeyEnv
	}
	if privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY"); privateKey != "" {
		c.GitHub.App.PrivateKey = privateKey
	}
	if portStr := os.Getenv("PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			c.Server.Port = port
		}
	}
	if useDockerStr := os.Getenv("USE_DOCKER"); useDockerStr != "" {
		if useDocker, err := strconv.ParseBool(useDockerStr); err == nil {
			c.UseDocker = useDocker
		}
	}
	if globalPath := os.Getenv("GLOBAL_COMMANDS_PATH"); globalPath != "" {
		c.Commands.GlobalPath = globalPath
	}
	// Mention configuration from environment
	if mentionTrigger := os.Getenv("MENTION_TRIGGER"); mentionTrigger != "" {
		c.Mention.DefaultTrigger = mentionTrigger
	}
	// Review configuration from environment
	if excludedAccounts := os.Getenv("REVIEW_EXCLUDED_ACCOUNTS"); excludedAccounts != "" {
		c.Review.ExcludedAccounts = strings.Split(excludedAccounts, ",")
	}
	// GitHub API configuration from environment
	if useGraphQLStr := os.Getenv("GITHUB_USE_GRAPHQL"); useGraphQLStr != "" {
		if useGraphQL, err := strconv.ParseBool(useGraphQLStr); err == nil {
			c.GitHub.API.UseGraphQL = useGraphQL
		}
	}
	if graphQLFallbackStr := os.Getenv("GITHUB_GRAPHQL_FALLBACK"); graphQLFallbackStr != "" {
		if graphQLFallback, err := strconv.ParseBool(graphQLFallbackStr); err == nil {
			c.GitHub.API.GraphQLFallback = graphQLFallback
		}
	}
	if rateLimitThresholdStr := os.Getenv("GITHUB_RATE_LIMIT_THRESHOLD"); rateLimitThresholdStr != "" {
		if rateLimitThreshold, err := strconv.Atoi(rateLimitThresholdStr); err == nil {
			c.GitHub.API.RateLimitThreshold = rateLimitThreshold
		}
	}
	if enableRateMonitoringStr := os.Getenv("GITHUB_ENABLE_RATE_MONITORING"); enableRateMonitoringStr != "" {
		if enableRateMonitoring, err := strconv.ParseBool(enableRateMonitoringStr); err == nil {
			c.GitHub.API.EnableRateMonitoring = enableRateMonitoring
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
			GHToken:    os.Getenv("GH_TOKEN"),
			API: GitHubAPIConfig{
				UseGraphQL:           getEnvBoolOrDefault("GITHUB_USE_GRAPHQL", false),
				GraphQLFallback:      getEnvBoolOrDefault("GITHUB_GRAPHQL_FALLBACK", true),
				RateLimitThreshold:   getEnvIntOrDefault("GITHUB_RATE_LIMIT_THRESHOLD", 1000),
				EnableRateMonitoring: getEnvBoolOrDefault("GITHUB_ENABLE_RATE_MONITORING", true),
			},
		},
		Workspace: WorkspaceConfig{
			BaseDir:      getEnvOrDefault("WORKSPACE_BASE_DIR", "/tmp/codeagent"),
			CleanupAfter: 24 * time.Hour,
		},
		Claude: ClaudeConfig{
			APIKey:         os.Getenv("ANTHROPIC_API_KEY"),
			AuthToken:      os.Getenv("ANTHROPIC_AUTH_TOKEN"),
			BaseURL:        os.Getenv("ANTHROPIC_BASE_URL"),
			ContainerImage: getEnvOrDefault("CLAUDE_IMAGE", "anthropic/claude-code:latest"),
			Timeout:        30 * time.Minute,
			Interactive:    getEnvBoolOrDefault("CLAUDE_INTERACTIVE", false),
		},
		Gemini: GeminiConfig{
			APIKey:             os.Getenv("GEMINI_API_KEY"),
			ContainerImage:     getEnvOrDefault("GEMINI_IMAGE", "google-gemini/gemini-cli:latest"),
			Timeout:            30 * time.Minute,
			GoogleCloudProject: os.Getenv("GOOGLE_CLOUD_PROJECT"),
		},
		Docker: DockerConfig{
			Socket:  getEnvOrDefault("DOCKER_SOCKET", "unix:///var/run/docker.sock"),
			Network: getEnvOrDefault("DOCKER_NETWORK", "bridge"),
		},
		Commands: CommandsConfig{
			GlobalPath: os.Getenv("GLOBAL_COMMANDS_PATH"),
		},
		Mention: MentionConfig{
			Triggers:       []string{getEnvOrDefault("MENTION_TRIGGER", "@qiniu-ci")},
			DefaultTrigger: getEnvOrDefault("MENTION_TRIGGER", "@qiniu-ci"),
		},
		Review: ReviewConfig{
			ExcludedAccounts: []string{},
		},
		CodeProvider: getEnvOrDefault("CODE_PROVIDER", "claude"),
		UseDocker:    getEnvBoolOrDefault("USE_DOCKER", true),
	}
}

// resolvePaths 将配置中的相对路径转换为绝对路径
func (c *Config) resolvePaths(configDir string) error {
	// 处理工作空间基础目录
	if c.Workspace.BaseDir != "" {
		// 如果路径不是绝对路径，则相对于配置文件目录解析
		if !filepath.IsAbs(c.Workspace.BaseDir) {
			absPath, err := filepath.Abs(filepath.Join(configDir, c.Workspace.BaseDir))
			if err == nil {
				c.Workspace.BaseDir = absPath
			}
		}
	}

	// 处理全局命令路径
	if c.Commands.GlobalPath != "" {
		// 如果路径不是绝对路径，则相对于配置文件目录解析
		if !filepath.IsAbs(c.Commands.GlobalPath) {
			absPath, err := filepath.Abs(filepath.Join(configDir, c.Commands.GlobalPath))
			if err == nil {
				c.Commands.GlobalPath = absPath
			}
		}

		// 确保路径存在
		if _, err := os.Stat(c.Commands.GlobalPath); os.IsNotExist(err) {
			return fmt.Errorf("global commands path does not exist: %s", c.Commands.GlobalPath)
		}
	}
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// IsGitHubTokenConfigured returns whether GitHub token is configured
func (c *Config) IsGitHubTokenConfigured() bool {
	return c.GitHub.Token != ""
}

// IsGitHubAppConfigured returns whether GitHub App is configured
func (c *Config) IsGitHubAppConfigured() bool {
	return c.GitHub.App.AppID > 0 &&
		(c.GitHub.App.PrivateKeyPath != "" ||
			c.GitHub.App.PrivateKeyEnv != "" ||
			c.GitHub.App.PrivateKey != "")
}

// ValidateGitHubConfig validates the GitHub configuration
func (c *Config) ValidateGitHubConfig() error {
	if !c.IsGitHubTokenConfigured() && !c.IsGitHubAppConfigured() {
		return fmt.Errorf("either GitHub token or GitHub App must be configured")
	}
	return nil
}
