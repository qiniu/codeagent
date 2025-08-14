package command

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CommandDefinition represents a parsed command with YAML frontmatter and markdown content
type CommandDefinition struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Model        string   `yaml:"model"`
	Temperature  *float64 `yaml:"temperature,omitempty"`
	Tools        []string `yaml:"tools,omitempty"`
	AllowedTools string   `yaml:"allowed-tools,omitempty"`
	Subagent     string   `yaml:"subagent,omitempty"`

	// Markdown content (everything after frontmatter)
	Content string `yaml:"-"`

	// Metadata
	FilePath string `yaml:"-"`
	Source   string `yaml:"-"` // "global" or "repository"
}

// AgentDefinition represents a parsed subagent definition
type AgentDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Model       string   `yaml:"model,omitempty"`
	Temperature *float64 `yaml:"temperature,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`

	// Markdown content (agent system prompt)
	Content string `yaml:"-"`

	// Metadata
	FilePath string `yaml:"-"`
	Source   string `yaml:"-"` // "global" or "repository"
}

// CommandLoader handles loading and parsing of command and agent definitions
type CommandLoader struct {
	globalPath     string
	repositoryPath string
}

// NewCommandLoader creates a new command loader
func NewCommandLoader(globalPath, repositoryPath string) *CommandLoader {
	return &CommandLoader{
		globalPath:     globalPath,
		repositoryPath: repositoryPath,
	}
}

// LoadCommand loads a specific command by name, with repository overriding global
func (cl *CommandLoader) LoadCommand(name string) (*CommandDefinition, error) {
	// First try repository-specific command
	if cl.repositoryPath != "" {
		repoCmd, err := cl.loadCommandFromPath(name, cl.repositoryPath, "repository")
		if err == nil {
			return repoCmd, nil
		}
		// If error is not "file not found", return the error
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error loading repository command %s: %w", name, err)
		}
	}

	// Fallback to global command
	globalCmd, err := cl.loadCommandFromPath(name, cl.globalPath, "global")
	if err != nil {
		return nil, fmt.Errorf("command %s not found in global or repository configs: %w", name, err)
	}

	return globalCmd, nil
}

// LoadAgent loads a specific agent by name, with repository overriding global
func (cl *CommandLoader) LoadAgent(name string) (*AgentDefinition, error) {
	// First try repository-specific agent
	if cl.repositoryPath != "" {
		repoAgent, err := cl.loadAgentFromPath(name, cl.repositoryPath, "repository")
		if err == nil {
			return repoAgent, nil
		}
		// If error is not "file not found", return the error
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error loading repository agent %s: %w", name, err)
		}
	}

	// Fallback to global agent
	globalAgent, err := cl.loadAgentFromPath(name, cl.globalPath, "global")
	if err != nil {
		return nil, fmt.Errorf("agent %s not found in global or repository configs: %w", name, err)
	}

	return globalAgent, nil
}

// ListCommands returns all available commands (repository overrides global)
func (cl *CommandLoader) ListCommands() (map[string]*CommandDefinition, error) {
	commands := make(map[string]*CommandDefinition)

	// Load global commands first
	if err := cl.loadCommandsFromDirectory(filepath.Join(cl.globalPath, "commands"), "global", commands); err != nil {
		return nil, fmt.Errorf("error loading global commands: %w", err)
	}

	// Load repository commands (overrides global)
	if cl.repositoryPath != "" {
		if err := cl.loadCommandsFromDirectory(filepath.Join(cl.repositoryPath, "commands"), "repository", commands); err != nil {
			// Repository commands are optional, so we don't fail if directory doesn't exist
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error loading repository commands: %w", err)
			}
		}
	}

	return commands, nil
}

// ListAgents returns all available agents (repository overrides global)
func (cl *CommandLoader) ListAgents() (map[string]*AgentDefinition, error) {
	agents := make(map[string]*AgentDefinition)

	// Load global agents first
	if err := cl.loadAgentsFromDirectory(filepath.Join(cl.globalPath, "agents"), "global", agents); err != nil {
		return nil, fmt.Errorf("error loading global agents: %w", err)
	}

	// Load repository agents (overrides global)
	if cl.repositoryPath != "" {
		if err := cl.loadAgentsFromDirectory(filepath.Join(cl.repositoryPath, "agents"), "repository", agents); err != nil {
			// Repository agents are optional, so we don't fail if directory doesn't exist
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error loading repository agents: %w", err)
			}
		}
	}

	return agents, nil
}

// loadCommandFromPath loads a command from a specific path
func (cl *CommandLoader) loadCommandFromPath(name, basePath, source string) (*CommandDefinition, error) {
	filePath := filepath.Join(basePath, "commands", name+".md")
	return cl.parseCommandFile(filePath, source)
}

// loadAgentFromPath loads an agent from a specific path
func (cl *CommandLoader) loadAgentFromPath(name, basePath, source string) (*AgentDefinition, error) {
	filePath := filepath.Join(basePath, "agents", name+".md")
	return cl.parseAgentFile(filePath, source)
}

// loadCommandsFromDirectory loads all commands from a directory
func (cl *CommandLoader) loadCommandsFromDirectory(dirPath, source string, commands map[string]*CommandDefinition) error {
	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		cmd, err := cl.parseCommandFile(path, source)
		if err != nil {
			return fmt.Errorf("error parsing command file %s: %w", path, err)
		}

		// Use filename (without extension) as command name if not specified in frontmatter
		if cmd.Name == "" {
			cmd.Name = strings.TrimSuffix(d.Name(), ".md")
		}

		commands[cmd.Name] = cmd
		return nil
	})
}

// loadAgentsFromDirectory loads all agents from a directory
func (cl *CommandLoader) loadAgentsFromDirectory(dirPath, source string, agents map[string]*AgentDefinition) error {
	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		agent, err := cl.parseAgentFile(path, source)
		if err != nil {
			return fmt.Errorf("error parsing agent file %s: %w", path, err)
		}

		// Use filename (without extension) as agent name if not specified in frontmatter
		if agent.Name == "" {
			agent.Name = strings.TrimSuffix(d.Name(), ".md")
		}

		agents[agent.Name] = agent
		return nil
	})
}

// parseCommandFile parses a command file with YAML frontmatter and markdown content
func (cl *CommandLoader) parseCommandFile(filePath, source string) (*CommandDefinition, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	frontmatter, markdown, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("error parsing frontmatter in %s: %w", filePath, err)
	}

	var cmd CommandDefinition
	if len(frontmatter) > 0 {
		if err := yaml.Unmarshal([]byte(frontmatter), &cmd); err != nil {
			return nil, fmt.Errorf("error parsing YAML frontmatter in %s: %w", filePath, err)
		}
	}

	cmd.Content = markdown
	cmd.FilePath = filePath
	cmd.Source = source

	return &cmd, nil
}

// parseAgentFile parses an agent file with YAML frontmatter and markdown content
func (cl *CommandLoader) parseAgentFile(filePath, source string) (*AgentDefinition, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	frontmatter, markdown, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("error parsing frontmatter in %s: %w", filePath, err)
	}

	var agent AgentDefinition
	if len(frontmatter) > 0 {
		if err := yaml.Unmarshal([]byte(frontmatter), &agent); err != nil {
			return nil, fmt.Errorf("error parsing YAML frontmatter in %s: %w", filePath, err)
		}
	}

	agent.Content = markdown
	agent.FilePath = filePath
	agent.Source = source

	return &agent, nil
}

// parseFrontmatter extracts YAML frontmatter and markdown content from a file
func parseFrontmatter(content string) (frontmatter, markdown string, err error) {
	lines := strings.Split(content, "\n")

	// Check if file starts with frontmatter delimiter
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		// No frontmatter, entire content is markdown
		return "", content, nil
	}

	// Find end of frontmatter
	var frontmatterLines []string
	var markdownStartIdx int

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			markdownStartIdx = i + 1
			break
		}
		frontmatterLines = append(frontmatterLines, lines[i])
	}

	if markdownStartIdx == 0 {
		return "", "", fmt.Errorf("frontmatter not properly closed with '---'")
	}

	frontmatter = strings.Join(frontmatterLines, "\n")

	// Join remaining lines as markdown content
	if markdownStartIdx < len(lines) {
		markdown = strings.Join(lines[markdownStartIdx:], "\n")
	}

	return frontmatter, markdown, nil
}

// ValidateCommand validates a command definition
func (cmd *CommandDefinition) ValidateCommand() error {
	if cmd.Description == "" {
		return fmt.Errorf("command description is required")
	}

	if cmd.Content == "" {
		return fmt.Errorf("command content is required")
	}

	return nil
}

// ValidateAgent validates an agent definition
func (agent *AgentDefinition) ValidateAgent() error {
	if agent.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	if agent.Description == "" {
		return fmt.Errorf("agent description is required")
	}

	if agent.Content == "" {
		return fmt.Errorf("agent content is required")
	}

	return nil
}
