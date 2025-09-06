package workspace

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// ContainerService handles Docker container operations
type ContainerService interface {
	CleanupWorkspaceContainers(ws *models.Workspace) error
	RemoveContainer(containerName string) error
	ContainerExists(containerName string) (bool, error)
	GenerateContainerNames(ws *models.Workspace) []string
	GetCodeAgentContainers() ([]ContainerInfo, error)
	CleanupOrphanedContainers(maxAge time.Duration) error
}

// ContainerInfo holds information about a container
type ContainerInfo struct {
	ID      string
	Name    string
	Created time.Time
	Status  string
	AIModel string
	Org     string
	Repo    string
	Type    string // "pr" or "issue"
	Number  int
}

type containerService struct{}

// NewContainerService creates a new container service instance
func NewContainerService() ContainerService {
	return &containerService{}
}

// CleanupWorkspaceContainers cleans up all containers related to a workspace
func (c *containerService) CleanupWorkspaceContainers(ws *models.Workspace) error {
	if ws == nil {
		return nil
	}

	containerNames := c.GenerateContainerNames(ws)
	removedCount := 0
	var errors []error

	for _, containerName := range containerNames {
		exists, err := c.ContainerExists(containerName)
		if err != nil {
			log.Errorf("Failed to check container %s: %v", containerName, err)
			continue
		}

		if exists {
			if err := c.RemoveContainer(containerName); err != nil {
				log.Errorf("Failed to remove container %s: %v", containerName, err)
				errors = append(errors, err)
			} else {
				removedCount++
				log.Infof("Successfully removed container: %s", containerName)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove %d containers", len(errors))
	}

	log.Infof("Container cleanup completed. Removed %d containers for workspace %s", removedCount, ws.Path)
	return nil
}

// RemoveContainer removes a Docker container by name
func (c *containerService) RemoveContainer(containerName string) error {
	cmd := exec.Command("docker", "rm", "-f", containerName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove container %s: %w, output: %s", containerName, err, string(output))
	}
	return nil
}

// ContainerExists checks if a Docker container exists and is running
func (c *containerService) ContainerExists(containerName string) (bool, error) {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check container status: %w", err)
	}

	containerStatus := strings.TrimSpace(string(output))
	return containerStatus != "", nil
}

// GenerateContainerNames generates all possible container names for a workspace
func (c *containerService) GenerateContainerNames(ws *models.Workspace) []string {
	var containerNames []string

	// Generate container names based on AI model
	switch ws.AIModel {
	case "claude":
		if ws.PRNumber > 0 {
			containerNames = append(containerNames, fmt.Sprintf("claude__%s__%s__pr__%d", ws.Org, ws.Repo, ws.PRNumber))
		}
		if ws.Issue != nil {
			containerNames = append(containerNames, fmt.Sprintf("claude__%s__%s__issue__%d", ws.Org, ws.Repo, ws.Issue.GetNumber()))
		}
		// Interactive container variant
		containerNames = append(containerNames, fmt.Sprintf("claude__interactive__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))

	case "gemini":
		if ws.PRNumber > 0 {
			containerNames = append(containerNames, fmt.Sprintf("gemini__%s__%s__pr__%d", ws.Org, ws.Repo, ws.PRNumber))
		}
		if ws.Issue != nil {
			containerNames = append(containerNames, fmt.Sprintf("gemini__%s__%s__issue__%d", ws.Org, ws.Repo, ws.Issue.GetNumber()))
		}

	default:
		// If AI model is unknown, try all possible patterns
		if ws.PRNumber > 0 {
			containerNames = append(containerNames,
				fmt.Sprintf("claude__%s__%s__pr__%d", ws.Org, ws.Repo, ws.PRNumber),
				fmt.Sprintf("gemini__%s__%s__pr__%d", ws.Org, ws.Repo, ws.PRNumber),
			)
		}
		if ws.Issue != nil {
			containerNames = append(containerNames,
				fmt.Sprintf("claude__%s__%s__issue__%d", ws.Org, ws.Repo, ws.Issue.GetNumber()),
				fmt.Sprintf("gemini__%s__%s__issue__%d", ws.Org, ws.Repo, ws.Issue.GetNumber()),
			)
		}
		containerNames = append(containerNames, fmt.Sprintf("claude__interactive__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
	}

	return containerNames
}

// GetCodeAgentContainers retrieves information about all CodeAgent containers
func (c *containerService) GetCodeAgentContainers() ([]ContainerInfo, error) {
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name=claude__", "--filter", "name=gemini__", "--format", "{{.ID}}|{{.Names}}|{{.CreatedAt}}|{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var containers []ContainerInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Regular expression to parse container names
	// Format: aimodel__org__repo__type__number
	namePattern := regexp.MustCompile(`^(claude|gemini)__([^_]+)__([^_]+)__(pr|issue)__(\d+)(?:__.*)?$`)

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			continue
		}

		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		createdStr := strings.TrimSpace(parts[2])
		status := strings.TrimSpace(parts[3])

		// Parse creation time
		created, err := time.Parse("2006-01-02 15:04:05 -0700 MST", createdStr)
		if err != nil {
			log.Warnf("Failed to parse container creation time %s: %v", createdStr, err)
			created = time.Now() // fallback
		}

		// Parse container name to extract components
		matches := namePattern.FindStringSubmatch(name)
		if len(matches) != 6 {
			log.Warnf("Container name %s doesn't match expected pattern", name)
			continue
		}

		aiModel := matches[1]
		org := matches[2]
		repo := matches[3]
		containerType := matches[4]
		number, err := strconv.Atoi(matches[5])
		if err != nil {
			log.Warnf("Failed to parse number from container name %s: %v", name, err)
			continue
		}

		containers = append(containers, ContainerInfo{
			ID:      id,
			Name:    name,
			Created: created,
			Status:  status,
			AIModel: aiModel,
			Org:     org,
			Repo:    repo,
			Type:    containerType,
			Number:  number,
		})
	}

	return containers, nil
}

// CleanupOrphanedContainers removes CodeAgent containers older than maxAge
func (c *containerService) CleanupOrphanedContainers(maxAge time.Duration) error {
	containers, err := c.GetCodeAgentContainers()
	if err != nil {
		return fmt.Errorf("failed to get CodeAgent containers: %w", err)
	}

	now := time.Now()
	removedCount := 0
	var errors []error

	for _, container := range containers {
		// Check if container is older than maxAge
		if now.Sub(container.Created) > maxAge {
			log.Infof("Removing orphaned container: %s (created: %s, age: %s)",
				container.Name,
				container.Created.Format(time.RFC3339),
				now.Sub(container.Created).String())

			if err := c.RemoveContainer(container.Name); err != nil {
				log.Errorf("Failed to remove orphaned container %s: %v", container.Name, err)
				errors = append(errors, err)
			} else {
				removedCount++
				log.Infof("Successfully removed orphaned container: %s", container.Name)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove %d orphaned containers", len(errors))
	}

	log.Infof("Orphaned container cleanup completed. Removed %d/%d containers", removedCount, len(containers))
	return nil
}
