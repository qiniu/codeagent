package code

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/qiniu/x/log"
)

// isContainerRunning 检查指定名称的容器是否在运行
func isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to check container status: %v", err)
		return false
	}

	// 检查输出是否包含容器名称
	return strings.TrimSpace(string(output)) == containerName
}

// extractRepoName 从仓库URL中提取仓库名
func extractRepoName(repoURL string) string {
	// 处理 GitHub URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			repo := strings.TrimSuffix(parts[len(parts)-1], ".git")
			return repo
		}
	}

	// 如果不是标准格式，返回一个安全的名称
	return "repo"
}
