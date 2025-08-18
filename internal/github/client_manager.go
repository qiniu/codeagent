package github

import (
	"context"
	"fmt"
	"sync"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/github/auth"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
)

// ClientManagerInterface 管理GitHub客户端，支持根据仓库动态获取合适的客户端
type ClientManagerInterface interface {
	// GetClient 根据仓库信息获取GitHub客户端
	GetClient(ctx context.Context, repo *models.Repository) (*Client, error)

	// GetDefaultClient 获取默认客户端（仅在无repo信息时使用，如初始化阶段）
	GetDefaultClient() *Client

	// Close 释放资源
	Close() error
}

// ClientManager 客户端管理器实现
type ClientManager struct {
	defaultClient *Client            // 默认客户端
	authenticator auth.Authenticator // 认证器
	config        *config.Config     // 配置
	clientCache   map[string]*Client // 客户端缓存，key为"owner/repo"
	cacheMutex    sync.RWMutex       // 缓存读写锁
}

// NewClientManager 创建客户端管理器
func NewClientManager(cfg *config.Config) (ClientManagerInterface, error) {
	// 创建默认认证器
	builder := auth.NewAuthenticatorBuilder(cfg)
	authenticator, err := builder.BuildAuthenticator()
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticator: %w", err)
	}

	// 创建默认客户端
	defaultClient, err := NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create default client: %w", err)
	}

	return &ClientManager{
		defaultClient: defaultClient,
		authenticator: authenticator,
		config:        cfg,
		clientCache:   make(map[string]*Client),
		cacheMutex:    sync.RWMutex{},
	}, nil
}

// GetClient 根据仓库信息获取GitHub客户端
func (m *ClientManager) GetClient(ctx context.Context, repo *models.Repository) (*Client, error) {
	// 如果没有仓库信息，返回默认客户端
	if repo == nil {
		return m.defaultClient, nil
	}

	// 构建缓存键：组织+仓库
	cacheKey := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)

	// 检查缓存
	m.cacheMutex.RLock()
	if cachedClient, exists := m.clientCache[cacheKey]; exists {
		m.cacheMutex.RUnlock()
		return cachedClient, nil
	}
	m.cacheMutex.RUnlock()

	// 获取写锁，双重检查
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	if cachedClient, exists := m.clientCache[cacheKey]; exists {
		return cachedClient, nil
	}

	// 尝试为特定组织+仓库创建客户端
	client, err := m.createClientForRepo(ctx, repo)
	if err != nil {
		// 创建失败，返回默认客户端
		return m.defaultClient, nil
	}

	// 缓存新创建的客户端
	m.clientCache[cacheKey] = client
	return client, nil
}

// GetDefaultClient 获取默认客户端
func (m *ClientManager) GetDefaultClient() *Client {
	return m.defaultClient
}

// createClientForRepo 为特定仓库创建客户端
func (m *ClientManager) createClientForRepo(ctx context.Context, repo *models.Repository) (*Client, error) {
	authInfo := m.authenticator.GetAuthInfo()

	if authInfo.Type == auth.AuthTypeApp {
		// GitHub App模式：查找组织的安装
		installationID, err := m.findInstallationForOrg(ctx, repo.Owner)
		if err != nil {
			return nil, fmt.Errorf("failed to find installation for org %s: %w", repo.Owner, err)
		}

		// 获取安装客户端
		githubClient, err := m.authenticator.GetInstallationClient(ctx, installationID)
		if err != nil {
			return nil, fmt.Errorf("failed to get installation client for %s/%s: %w", repo.Owner, repo.Name, err)
		}

		return &Client{
			client: githubClient,
		}, nil
	}

	// PAT模式：直接返回默认客户端
	return m.defaultClient, nil
}

// findInstallationForOrg 查找组织对应的GitHub App安装ID
func (m *ClientManager) findInstallationForOrg(ctx context.Context, owner string) (int64, error) {
	// 获取App客户端
	appClient, err := m.authenticator.GetClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get app client: %w", err)
	}

	// 列出所有安装
	installations, _, err := appClient.Apps.ListInstallations(ctx, &github.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to list installations: %w", err)
	}

	// 查找匹配的组织安装
	for _, installation := range installations {
		if installation.Account != nil && installation.Account.GetLogin() == owner {
			return installation.GetID(), nil
		}
	}

	return 0, fmt.Errorf("no installation found for organization: %s", owner)
}

// Close 释放资源
func (m *ClientManager) Close() error {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	// 清空缓存
	m.clientCache = make(map[string]*Client)
	return nil
}
