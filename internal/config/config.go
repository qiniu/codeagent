package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	Token      string `yaml:"token"`
	WebhookURL string `yaml:"webhook_url"`
	GHToken    string `yaml:"gh_token"`
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

	return &Config{
		Server: ServerConfig{
			Port:          port,
			WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		},
		GitHub: GitHubConfig{
			Token:      os.Getenv("GITHUB_TOKEN"),
			WebhookURL: os.Getenv("WEBHOOK_URL"),
			GHToken:    os.Getenv("GH_TOKEN"),
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
