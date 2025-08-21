package code

import (
	"fmt"
	"strings"

	"github.com/qiniu/codeagent/pkg/models"
)

// ContainerNaming 统一的容器命名工具
type ContainerNaming struct{}

// NewContainerNaming 创建容器命名工具
func NewContainerNaming() *ContainerNaming {
	return &ContainerNaming{}
}

// WorkspaceType 工作空间类型
type WorkspaceType int

const (
	WorkspaceTypePR WorkspaceType = iota
	WorkspaceTypeIssue
	WorkspaceTypeGeneric
)

// ContainerType 容器类型
type ContainerType int

const (
	ContainerTypeStandard ContainerType = iota
	ContainerTypeInteractive
)

// NameSpec 命名规格
type NameSpec struct {
	Provider      string        // claude, gemini
	Org           string        // 组织名
	Repo          string        // 仓库名
	WorkspaceType WorkspaceType // 工作空间类型
	ContainerType ContainerType // 容器类型
	Number        int           // PR号或Issue号
	Timestamp     int64         // 时间戳（可选，仅用于fallback）
}

// GenerateContainerName 生成容器名称
func (cn *ContainerNaming) GenerateContainerName(spec NameSpec) string {
	parts := []string{spec.Provider}

	// 如果是交互式容器，添加interactive标识
	if spec.ContainerType == ContainerTypeInteractive {
		parts = append(parts, "interactive")
	}

	parts = append(parts, spec.Org, spec.Repo)

	// 根据工作空间类型添加后缀
	switch spec.WorkspaceType {
	case WorkspaceTypePR:
		parts = append(parts, "pr", fmt.Sprintf("%d", spec.Number))
	case WorkspaceTypeIssue:
		parts = append(parts, "issue", fmt.Sprintf("%d", spec.Number))
	case WorkspaceTypeGeneric:
		if spec.Timestamp > 0 {
			parts = append(parts, "workspace", fmt.Sprintf("%d", spec.Timestamp))
		} else {
			parts = append(parts, "workspace")
		}
	}

	return strings.Join(parts, "__")
}

// GenerateConfigDirName 生成配置目录名称
func (cn *ContainerNaming) GenerateConfigDirName(spec NameSpec) string {
	parts := []string{"." + spec.Provider, spec.Org, spec.Repo}

	switch spec.WorkspaceType {
	case WorkspaceTypePR:
		parts = append(parts, "pr", fmt.Sprintf("%d", spec.Number))
	case WorkspaceTypeIssue:
		parts = append(parts, "issue", fmt.Sprintf("%d", spec.Number))
	case WorkspaceTypeGeneric:
		if spec.Timestamp > 0 {
			parts = append(parts, "workspace", fmt.Sprintf("%d", spec.Timestamp))
		} else {
			parts = append(parts, "workspace")
		}
	}

	return strings.Join(parts, "-")
}

// SpecFromWorkspace 从工作空间创建命名规格
func (cn *ContainerNaming) SpecFromWorkspace(provider string, workspace *models.Workspace, containerType ContainerType) NameSpec {
	spec := NameSpec{
		Provider:      provider,
		Org:           workspace.Org,
		Repo:          workspace.Repo,
		ContainerType: containerType,
		Timestamp:     workspace.CreatedAt.Unix(),
	}

	if workspace.PRNumber > 0 {
		spec.WorkspaceType = WorkspaceTypePR
		spec.Number = workspace.PRNumber
	} else if workspace.Issue != nil {
		spec.WorkspaceType = WorkspaceTypeIssue
		spec.Number = workspace.Issue.GetNumber()
	} else {
		spec.WorkspaceType = WorkspaceTypeGeneric
	}

	return spec
}

// GenerateAllPossibleNames 生成所有可能的容器名（用于查找现有容器）
func (cn *ContainerNaming) GenerateAllPossibleNames(provider string, workspace *models.Workspace) []string {
	var names []string

	repoName := extractRepoName(workspace.Repository)
	
	if workspace.PRNumber > 0 {
		// PR 容器的所有可能命名
		baseSpec := NameSpec{
			Provider:      provider,
			Org:           workspace.Org,
			Repo:          repoName,
			WorkspaceType: WorkspaceTypePR,
			Number:        workspace.PRNumber,
		}

		// 标准容器
		names = append(names, cn.GenerateContainerName(baseSpec))
		
		// 交互式容器（仅限Claude）
		if provider == "claude" {
			baseSpec.ContainerType = ContainerTypeInteractive
			names = append(names, cn.GenerateContainerName(baseSpec))
		}

		// 旧格式兼容（用连字符分隔）
		names = append(names, 
			fmt.Sprintf("%s-%s-%s-%d", provider, workspace.Org, repoName, workspace.PRNumber),
			fmt.Sprintf("%s-interactive-%s-%s-%d", provider, workspace.Org, repoName, workspace.PRNumber),
		)

	} else if workspace.Issue != nil {
		// Issue 容器的所有可能命名
		baseSpec := NameSpec{
			Provider:      provider,
			Org:           workspace.Org,
			Repo:          repoName,
			WorkspaceType: WorkspaceTypeIssue,
			Number:        workspace.Issue.GetNumber(),
		}

		// 标准容器
		names = append(names, cn.GenerateContainerName(baseSpec))

		// 交互式容器（仅限Claude）
		if provider == "claude" {
			baseSpec.ContainerType = ContainerTypeInteractive
			names = append(names, cn.GenerateContainerName(baseSpec))
		}
	}

	return names
}