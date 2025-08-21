package code

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// DockerManager 通用的Docker容器管理工具
type DockerManager struct{}

// NewDockerManager 创建Docker管理器
func NewDockerManager() *DockerManager {
	return &DockerManager{}
}

// ContainerSpec 容器规格
type ContainerSpec struct {
	Name         string
	Image        string
	WorkingDir   string
	Volumes      []VolumeMount
	EnvVars      []EnvVar
	Entrypoint   []string
	Args         []string
	Interactive  bool
	Detached     bool
	AutoRemove   bool
}

// VolumeMount 卷挂载
type VolumeMount struct {
	HostPath      string
	ContainerPath string
}

// EnvVar 环境变量
type EnvVar struct {
	Key   string
	Value string
}

// CreateContainer 创建并启动容器
func (dm *DockerManager) CreateContainer(spec ContainerSpec) error {
	args := []string{"run"}

	// 基本参数
	if spec.AutoRemove {
		args = append(args, "--rm")
	}
	if spec.Detached {
		args = append(args, "-d")
	}
	if spec.Interactive {
		args = append(args, "-i")
	}
	
	// 容器名称
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}

	// 工作目录
	if spec.WorkingDir != "" {
		args = append(args, "-w", spec.WorkingDir)
	}

	// 卷挂载
	for _, volume := range spec.Volumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s", volume.HostPath, volume.ContainerPath))
	}

	// 环境变量
	for _, env := range spec.EnvVars {
		if env.Value != "" {
			args = append(args, "-e", fmt.Sprintf("%s=%s", env.Key, env.Value))
		}
	}

	// 入口点
	if len(spec.Entrypoint) > 0 {
		args = append(args, "--entrypoint")
		args = append(args, strings.Join(spec.Entrypoint, " "))
	}

	// 镜像
	args = append(args, spec.Image)

	// 附加参数
	args = append(args, spec.Args...)

	log.Infof("Docker command: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start Docker container: %v", err)
		log.Errorf("Docker stderr: %s", stderr.String())
		return fmt.Errorf("failed to start Docker container: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		log.Errorf("Docker container failed: %v", err)
		log.Errorf("Docker stdout: %s", stdout.String())
		log.Errorf("Docker stderr: %s", stderr.String())
		return fmt.Errorf("docker container failed: %w", err)
	}

	log.Infof("Docker container started successfully: %s", spec.Name)
	return nil
}

// BuildCommonSpec 构建通用容器规格
func (dm *DockerManager) BuildCommonSpec(workspace *models.Workspace, provider string, cfg *config.Config) (ContainerSpec, error) {
	repoName := extractRepoName(workspace.Repository)
	naming := NewContainerNaming()
	spec := naming.SpecFromWorkspace(provider, workspace, ContainerTypeStandard)
	spec.Repo = repoName
	containerName := naming.GenerateContainerName(spec)

	// 确保路径存在
	workspacePath, err := filepath.Abs(workspace.Path)
	if err != nil {
		return ContainerSpec{}, fmt.Errorf("failed to get absolute workspace path: %w", err)
	}

	// 检查是否使用了/tmp目录
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	// 检查路径是否存在
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		return ContainerSpec{}, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	containerSpec := ContainerSpec{
		Name:        containerName,
		WorkingDir:  "/workspace",
		AutoRemove:  true,
		Detached:    true,
		Interactive: false,
		Volumes: []VolumeMount{
			{HostPath: workspacePath, ContainerPath: "/workspace"},
		},
		EnvVars: []EnvVar{},
	}

	// 挂载处理过的.codeagent目录
	if workspace.ProcessedCodeAgentPath != "" {
		if _, err := os.Stat(workspace.ProcessedCodeAgentPath); err == nil {
			containerSpec.Volumes = append(containerSpec.Volumes, 
				VolumeMount{
					HostPath: workspace.ProcessedCodeAgentPath, 
					ContainerPath: "/home/codeagent/.codeagent",
				},
			)
			log.Infof("Mounting processed .codeagent directory: %s -> /home/codeagent/.codeagent", workspace.ProcessedCodeAgentPath)

			// 挂载agents目录
			agentsPath := filepath.Join(workspace.ProcessedCodeAgentPath, "agents")
			if stat, err := os.Stat(agentsPath); err == nil && stat.IsDir() {
				containerSpec.Volumes = append(containerSpec.Volumes,
					VolumeMount{
						HostPath: agentsPath,
						ContainerPath: fmt.Sprintf("/home/codeagent/.%s/agents", provider),
					},
				)
				log.Infof("Mounting merged agents directory: %s -> /home/codeagent/.%s/agents", agentsPath, provider)
			}
		}
	}

	return containerSpec, nil
}