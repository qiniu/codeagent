package code

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/qiniu/x/log"
)

// isContainerRunning checks if container with specified name is running
func isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to check container status: %v", err)
		return false
	}

	// Check if output contains container name
	return strings.TrimSpace(string(output)) == containerName
}

// extractRepoName extracts repository name from repository URL
func extractRepoName(repoURL string) string {
	// Handle GitHub URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			repo := strings.TrimSuffix(parts[len(parts)-1], ".git")
			return repo
		}
	}

	// If not standard format, return a safe name
	return "repo"
}
