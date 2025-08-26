package workspace

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// ContainerService handles Docker container operations
type ContainerService interface {
	CleanupWorkspaceContainers(ws *models.Workspace) error
	RemoveContainer(containerName string) error
	ContainerExists(containerName string) (bool, error)
	GenerateContainerNames(ws *models.Workspace) []string
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
			log.Debugf("Failed to check container %s: %v", containerName, err)
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
		// New naming format
		containerNames = append(containerNames, fmt.Sprintf("claude__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
		// Legacy naming format (backward compatibility)
		containerNames = append(containerNames, fmt.Sprintf("claude-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber))

		// Interactive container variants
		containerNames = append(containerNames, fmt.Sprintf("claude__interactive__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
		containerNames = append(containerNames, fmt.Sprintf("claude-interactive-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber))

	case "gemini":
		// New naming format
		containerNames = append(containerNames, fmt.Sprintf("gemini__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
		// Legacy naming format (backward compatibility)
		containerNames = append(containerNames, fmt.Sprintf("gemini-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber))

	default:
		// If AI model is unknown, try all possible patterns
		containerNames = append(containerNames,
			fmt.Sprintf("claude__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("gemini__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("claude__interactive__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("claude-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("gemini-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("claude-interactive-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber),
		)
	}

	return containerNames
}
