package prompt

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/qbox/codeagent/pkg/models"
)

// CustomConfigInfo 自定义配置信息
type CustomConfigInfo struct {
	Exists bool `json:"exists"`
}

// ConfigCache 配置缓存
type ConfigCache struct {
	cache map[string]*CustomConfigInfo
	mu    sync.RWMutex
	ttl   time.Duration
}

// Detector 自定义配置检测器
type Detector struct {
	cache *ConfigCache
}

// NewDetector 创建新的自定义配置检测器
func NewDetector() *Detector {
	return &Detector{
		cache: &ConfigCache{
			cache: make(map[string]*CustomConfigInfo),
			ttl:   1 * time.Hour,
		},
	}
}

// GetCODEAGENTFile 检测仓库中是否存在 CODEAGENT.md 文件
func (d *Detector) GetCODEAGENTFile(ctx context.Context, workspace *models.Workspace) (*CustomConfigInfo, error) {
	if workspace == nil {
		return &CustomConfigInfo{Exists: false}, nil
	}

	// 生成缓存键
	cacheKey := d.generateCacheKey(workspace)

	// 先检查缓存
	if cached := d.cache.Get(cacheKey); cached != nil {
		return cached, nil
	}

	// 检查文件是否存在
	configPath := filepath.Join(workspace.Path, "CODEAGENT.md")
	_, err := os.Stat(configPath)

	configInfo := &CustomConfigInfo{
		Exists: err == nil,
	}

	// 缓存结果
	d.cache.Set(cacheKey, configInfo)

	return configInfo, nil
}

// generateCacheKey 生成缓存键
func (d *Detector) generateCacheKey(workspace *models.Workspace) string {
	return workspace.Org + "/" + workspace.Repo
}

// ConfigCache 方法实现
func (cc *ConfigCache) Get(key string) *CustomConfigInfo {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.cache[key]
}

func (cc *ConfigCache) Set(key string, info *CustomConfigInfo) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.cache[key] = info
}

func (cc *ConfigCache) Delete(key string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.cache, key)
}

// ClearCache 清除缓存
func (d *Detector) ClearCache() {
	d.cache.mu.Lock()
	defer d.cache.mu.Unlock()
	d.cache.cache = make(map[string]*CustomConfigInfo)
}
