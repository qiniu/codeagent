package prompt

import (
	"context"
	"os"
	"path/filepath"

	"github.com/qbox/codeagent/pkg/models"
)

// CustomConfigInfo 自定义配置信息
type CustomConfigInfo struct {
	Exists bool `json:"exists"`
}

// Detector 自定义配置检测器
type Detector struct{}

// NewDetector 创建新的自定义配置检测器
func NewDetector() *Detector {
	return &Detector{}
}

// GetAGENTFile 检测仓库中是否存在 AGENT.md 文件
func (d *Detector) GetAGENTFile(ctx context.Context, workspace *models.Workspace) (*CustomConfigInfo, error) {
	if workspace == nil {
		return &CustomConfigInfo{Exists: false}, nil
	}

	// 检查文件是否存在
	configPath := filepath.Join(workspace.Path, "AGENT.md")
	_, err := os.Stat(configPath)

	return &CustomConfigInfo{
		Exists: err == nil,
	}, nil
}
