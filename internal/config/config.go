package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	Token      string           `yaml:"token"`       // Existing PAT support
	WebhookURL string           `yaml:"webhook_url"` // Existing webhook URL
	App        GitHubAppConfig  `yaml:"app"`         // New GitHub App configuration
	AuthMode   string           `yaml:"auth_mode"`   // "token" | "app" | "auto"
}

type GitHubAppConfig struct {
	AppID           int64  `yaml:"app_id"`
	PrivateKeyPath  string `yaml:"private_key_path"`
	PrivateKeyEnv   string `yaml:"private_key_env"`
	PrivateKey      string `yaml:"private_key"`      // Direct content (not recommended for production)
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
		config.resolvePaths(filepath.Dir(configPath))

		return &config, nil
	}

	// 如果文件不存在，从环境变量创建配置
	config := loadFromEnv()
	// 将相对路径转换为绝对路径（相对于当前工作目录）
	config.resolvePaths(".")
	return config, nil
}

func (c *Config) loadFromEnv() {
	// Existing GitHub PAT configuration
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		c.GitHub.Token = token
	}
	
	// New GitHub App configuration
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
	if authMode := os.Getenv("GITHUB_AUTH_MODE"); authMode != "" {
		c.GitHub.AuthMode = authMode
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
}

func loadFromEnv() *Config {
	port := 8080
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	config := &Config{
		Server: ServerConfig{
			Port:          port,
			WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		},
		GitHub: GitHubConfig{
			Token:      os.Getenv("GITHUB_TOKEN"),
			WebhookURL: os.Getenv("WEBHOOK_URL"),
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
		CodeProvider: getEnvOrDefault("CODE_PROVIDER", "claude"),
		UseDocker:    getEnvBoolOrDefault("USE_DOCKER", true),
	}

	// Load additional environment variables including GitHub App config
	config.loadFromEnv()

	return config
}

// resolvePaths 将配置中的相对路径转换为绝对路径
func (c *Config) resolvePaths(configDir string) {
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

// GitHub App configuration validation and helpers

// AuthMode constants for GitHub authentication
const (
	AuthModeToken = "token" // Personal Access Token
	AuthModeApp   = "app"   // GitHub App
	AuthModeAuto  = "auto"  // Automatic detection
)

// ValidateGitHubConfig validates the GitHub configuration
func (c *Config) ValidateGitHubConfig() error {
	github := &c.GitHub

	// Normalize auth mode - auto-detect when empty or explicitly set to auto
	if github.AuthMode == "" || github.AuthMode == AuthModeAuto {
		// Auto-detect based on available configuration
		if github.App.AppID > 0 && (github.App.PrivateKeyPath != "" || github.App.PrivateKeyEnv != "" || github.App.PrivateKey != "") {
			github.AuthMode = AuthModeApp
		} else if github.Token != "" {
			github.AuthMode = AuthModeToken
		} else {
			github.AuthMode = AuthModeAuto
		}
	}

	// Validate based on auth mode
	switch github.AuthMode {
	case AuthModeToken:
		if github.Token == "" {
			return fmt.Errorf("GitHub token is required when auth_mode is 'token'")
		}
	case AuthModeApp:
		if github.App.AppID <= 0 {
			return fmt.Errorf("GitHub App ID is required when auth_mode is 'app'")
		}
		if github.App.PrivateKeyPath == "" && github.App.PrivateKeyEnv == "" && github.App.PrivateKey == "" {
			return fmt.Errorf("GitHub App private key source is required when auth_mode is 'app'")
		}
	case AuthModeAuto:
		// Auto mode requires at least one authentication method
		hasToken := github.Token != ""
		hasApp := github.App.AppID > 0 && (github.App.PrivateKeyPath != "" || github.App.PrivateKeyEnv != "" || github.App.PrivateKey != "")
		
		if !hasToken && !hasApp {
			return fmt.Errorf("GitHub authentication is required: either provide token or app configuration")
		}
	default:
		return fmt.Errorf("invalid GitHub auth_mode: %s (valid options: token, app, auto)", github.AuthMode)
	}

	return nil
}

// IsGitHubAppConfigured returns whether GitHub App is properly configured
func (c *Config) IsGitHubAppConfigured() bool {
	app := &c.GitHub.App
	return app.AppID > 0 && (app.PrivateKeyPath != "" || app.PrivateKeyEnv != "" || app.PrivateKey != "")
}

// IsGitHubTokenConfigured returns whether GitHub PAT is properly configured
func (c *Config) IsGitHubTokenConfigured() bool {
	return c.GitHub.Token != ""
}

// GetGitHubAuthMode returns the effective authentication mode
func (c *Config) GetGitHubAuthMode() string {
	if c.GitHub.AuthMode != "" {
		return c.GitHub.AuthMode
	}

	// Auto-detect
	if c.IsGitHubAppConfigured() {
		return AuthModeApp
	} else if c.IsGitHubTokenConfigured() {
		return AuthModeToken
	}

	return AuthModeAuto
}

// SetDefaults sets default values for configuration fields
func (c *Config) SetDefaults() {
	// Set default auth mode if not specified
	if c.GitHub.AuthMode == "" {
		c.GitHub.AuthMode = AuthModeAuto
	}

	// Set default workspace cleanup after if not specified
	if c.Workspace.CleanupAfter == 0 {
		c.Workspace.CleanupAfter = 24 * time.Hour
	}

	// Set default base directory if not specified
	if c.Workspace.BaseDir == "" {
		c.Workspace.BaseDir = "/tmp/codeagent"
	}

	// Set default server port if not specified
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
}

// Validate validates the entire configuration
func (c *Config) Validate() error {
	// Set defaults first
	c.SetDefaults()

	// Validate GitHub configuration
	if err := c.ValidateGitHubConfig(); err != nil {
		return fmt.Errorf("GitHub configuration error: %w", err)
	}

	// Validate server configuration
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Server.WebhookSecret == "" {
		return fmt.Errorf("webhook secret is required")
	}

	// Validate code provider
	if c.CodeProvider == "" {
		return fmt.Errorf("code provider is required")
	}

	validProviders := map[string]bool{
		"claude": true,
		"gemini": true,
	}
	if !validProviders[c.CodeProvider] {
		return fmt.Errorf("invalid code provider: %s (valid options: claude, gemini)", c.CodeProvider)
	}

	return nil
}
