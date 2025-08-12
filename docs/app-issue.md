### GitHub App 认证需求拆分为 PR 计划

#### 任务清单
- [x] 分析之前的需求文档结构
- [x] 将需求按逻辑模块拆分为独立 PR
- [x] 为每个 PR 生成详细的架构文档
- [x] 定义 PR 之间的依赖关系和实施顺序
- [x] 完成最终的 PR 拆分计划

---

## 📋 PR 拆分方案概览

基于之前的需求分析，我将把 GitHub App 认证功能拆分为 **9 个独立的 PR**，按照依赖关系和逻辑模块划分：

### 🏗️ 核心基础设施层 (PR 1-4)
- **PR-1**: JWT 生成和签名模块
- **PR-2**: Installation Token 管理 
- **PR-3**: 配置系统扩展
- **PR-4**: 认证抽象层重构

### 🏢 多租户支持层 (PR 5-6) 
- **PR-5**: Installation 管理和发现
- **PR-6**: 多租户工作空间隔离

### 🔄 兼容性和迁移层 (PR 7-8)
- **PR-7**: 向后兼容性实现
- **PR-8**: 配置迁移工具

### 🚀 增强功能层 (PR 9)
- **PR-9**: 监控、权限验证和文档

---

## 📖 详细 PR 架构文档

### PR-1: JWT 生成和签名模块

#### 🎯 **功能范围**
- 实现 GitHub App JWT 生成和签名功能
- 支持 RS256 签名算法
- Private Key 管理和加载
- JWT Claims 构建和验证

#### 🏛️ **架构设计**
```
internal/github/app/
├── jwt.go              # JWT 生成核心逻辑
├── private_key.go      # Private Key 管理
├── claims.go          # JWT Claims 构建
└── jwt_test.go        # 单元测试
```

#### 🔧 **核心组件**

**JWT Generator**
```go
type JWTGenerator struct {
    appID      int64
    privateKey *rsa.PrivateKey
}

func (j *JWTGenerator) GenerateJWT(ctx context.Context) (string, error)
```

**Private Key Loader**
```go
type PrivateKeyLoader interface {
    LoadFromFile(path string) (*rsa.PrivateKey, error)
    LoadFromEnv(envVar string) (*rsa.PrivateKey, error)
    LoadFromBytes(data []byte) (*rsa.PrivateKey, error)
}
```

#### ⚙️ **实现原理**
1. **JWT 标准**: 实现 RFC 7519 JWT 标准，使用 RS256 算法
2. **Claims 构建**: 包含 `iss` (App ID), `iat` (issued at), `exp` (expiration)
3. **安全性**: Private Key 内存安全处理，避免泄露
4. **错误处理**: 详细的错误分类和处理机制

#### 🔗 **依赖关系**
- **前置依赖**: 无
- **后续依赖**: PR-2 (Installation Token 管理)

---

### PR-2: Installation Token 管理

#### 🎯 **功能范围**
- Installation Access Token 获取和缓存
- Token 自动刷新机制
- Token 过期检测和处理
- Installation ID 到 Token 的映射管理

#### 🏛️ **架构设计**
```
internal/github/app/
├── installation.go      # Installation Token 管理
├── cache.go            # Token 缓存实现
├── refresh.go          # Token 刷新逻辑
└── installation_test.go # 单元测试
```

#### 🔧 **核心组件**

**Installation Token Manager**
```go
type InstallationTokenManager struct {
    jwtGenerator *JWTGenerator
    httpClient   *http.Client
    cache        TokenCache
}

func (m *InstallationTokenManager) GetToken(ctx context.Context, installationID int64) (*Token, error)
```

**Token Cache**
```go
type TokenCache interface {
    Get(installationID int64) (*Token, bool)
    Set(installationID int64, token *Token)
    Delete(installationID int64)
}
```

#### ⚙️ **实现原理**
1. **Token 获取流程**: JWT → GitHub API → Installation Token
2. **缓存策略**: 基于过期时间的 LRU 缓存，预留 5 分钟安全边际
3. **并发安全**: 使用 sync.RWMutex 保证线程安全
4. **自动刷新**: 后台 goroutine 定期检查和刷新即将过期的 token

#### 🔗 **依赖关系**
- **前置依赖**: PR-1 (JWT 生成模块)
- **后续依赖**: PR-4 (认证抽象层)

---

### PR-3: 配置系统扩展

#### 🎯 **功能范围**
- 扩展现有配置结构支持 GitHub App
- 多种 Private Key 加载方式
- 配置验证和默认值设置
- 环境变量映射

#### 🏛️ **架构设计**
```go
// 扩展现有的配置结构
type GitHubConfig struct {
    Token     string           `yaml:"token"`      // 现有 PAT 支持
    App       GitHubAppConfig  `yaml:"app"`        // 新增 App 配置
    AuthMode  string           `yaml:"auth_mode"`  // "token" | "app"
}

type GitHubAppConfig struct {
    AppID           int64  `yaml:"app_id"`
    PrivateKeyPath  string `yaml:"private_key_path"`
    PrivateKeyEnv   string `yaml:"private_key_env"`
    PrivateKey      string `yaml:"private_key"`     // Direct content (不推荐)
}
```

#### 🔧 **核心组件**

**配置加载器**
```go
type ConfigLoader struct {
    configPath string
}

func (l *ConfigLoader) LoadConfig() (*Config, error)
func (l *ConfigLoader) ValidateConfig(cfg *Config) error
```

#### ⚙️ **实现原理**
1. **向后兼容**: 保持现有 PAT 配置方式不变
2. **优先级顺序**: 环境变量 > 配置文件 > 默认值
3. **安全考虑**: Private Key 内容不记录到日志
4. **验证机制**: 启动时验证配置完整性和有效性

#### 🔗 **依赖关系**
- **前置依赖**: 无
- **后续依赖**: PR-4 (认证抽象层)

---

### PR-4: 认证抽象层重构

#### 🎯 **功能范围**
- 设计统一的认证接口
- 实现 PAT 和 GitHub App 两种认证器
- 客户端工厂模式实现
- 认证方式运行时切换

#### 🏛️ **架构设计**
```go
type Authenticator interface {
    GetClient(ctx context.Context) (*github.Client, error)
    GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error)
    GetAuthInfo() AuthInfo
}

type AuthInfo struct {
    Type         AuthType
    User         string  // PAT 用户或 App 名称
    Permissions  []string
}
```

#### 🔧 **核心组件**

**PAT Authenticator**
```go
type PATAuthenticator struct {
    token  string
    client *github.Client
}
```

**GitHub App Authenticator** 
```go
type GitHubAppAuthenticator struct {
    tokenManager *InstallationTokenManager
    httpClient   *http.Client
}
```

**Client Factory**
```go
type ClientFactory struct {
    authenticator Authenticator
}

func (f *ClientFactory) CreateClient(ctx context.Context, installationID int64) (*github.Client, error)
```

#### ⚙️ **实现原理**
1. **接口隔离**: 认证逻辑与业务逻辑分离
2. **工厂模式**: 统一客户端创建接口，隐藏认证复杂性
3. **可扩展性**: 易于添加新的认证方式（如 OAuth App）
4. **错误统一**: 标准化的错误类型和处理

#### 🔗 **依赖关系**
- **前置依赖**: PR-1, PR-2, PR-3
- **后续依赖**: PR-5 (Installation 管理)

---

### PR-5: Installation 管理和发现

#### 🎯 **功能范围**
- GitHub App Installation 发现和枚举
- Installation 元数据管理
- Webhook 到 Installation 的映射
- Installation 权限验证

#### 🏛️ **架构设计**
```
internal/installation/
├── manager.go          # Installation 管理器
├── discovery.go        # Installation 发现
├── metadata.go         # 元数据管理  
└── webhook_mapper.go   # Webhook 映射
```

#### 🔧 **核心组件**

**Installation Manager**
```go
type InstallationManager struct {
    authenticator github.Authenticator
    cache        InstallationCache
}

type Installation struct {
    ID           int64
    AccountType  string  // "Organization" | "User"
    AccountLogin string
    Permissions  map[string]string
    Events       []string
}
```

#### ⚙️ **实现原理**
1. **发现机制**: 定期调用 GitHub API 发现新的 Installation
2. **Webhook 映射**: 基于 Repository Owner 或 Installation ID 确定目标 Installation
3. **权限检查**: 验证 Installation 是否具有必要的权限
4. **缓存策略**: 缓存 Installation 信息减少 API 调用

#### 🔗 **依赖关系**
- **前置依赖**: PR-4 (认证抽象层)
- **后续依赖**: PR-6 (工作空间隔离)

---

### PR-6: 多租户工作空间隔离

#### 🎯 **功能范围**
- 扩展 Workspace 模型支持 Installation ID
- 基于 Installation 的数据隔离
- 工作空间清理和管理
- 多租户安全检查

#### 🏛️ **架构设计**
```go
// 扩展现有 Workspace 结构
type Workspace struct {
    ID             string
    InstallationID int64     // 新增字段
    RepoOwner      string
    RepoName       string
    Branch         string
    WorkDir        string
    CreatedAt      time.Time
}

type MultiTenantWorkspaceManager struct {
    baseManager    *workspace.Manager
    installations  *installation.Manager
}
```

#### ⚙️ **实现原理**
1. **隔离策略**: 基于 Installation ID 的文件夹隔离
2. **安全检查**: 确保工作空间只能访问对应 Installation 的仓库
3. **资源管理**: 按 Installation 统计和限制资源使用
4. **清理机制**: 基于 Installation 状态清理无效工作空间

#### 🔗 **依赖关系**
- **前置依赖**: PR-5 (Installation 管理)
- **后续依赖**: PR-7 (向后兼容实现)

---

### PR-7: 向后兼容性实现

#### 🎯 **功能范围**
- 同时支持 PAT 和 GitHub App 两种认证方式
- 平滑的认证方式切换
- 现有 API 接口保持不变
- 运行时认证方式检测

#### 🏛️ **架构设计**
```go
type HybridAuthenticator struct {
    patAuth    *PATAuthenticator
    appAuth    *GitHubAppAuthenticator
    authMode   AuthMode
}

func (h *HybridAuthenticator) GetClient(ctx context.Context) (*github.Client, error) {
    switch h.authMode {
    case AuthModePAT:
        return h.patAuth.GetClient(ctx)
    case AuthModeApp:
        // App 模式需要 Installation ID，从上下文获取
        return h.getAppClient(ctx)
    default:
        return h.autoDetectAndGetClient(ctx)
    }
}
```

#### ⚙️ **实现原理**
1. **适配器模式**: 包装现有认证实现，提供统一接口
2. **自动检测**: 基于配置和上下文自动选择合适的认证方式  
3. **优雅降级**: GitHub App 不可用时自动回退到 PAT
4. **兼容性保证**: 现有代码无需修改即可工作

#### 🔗 **依赖关系**
- **前置依赖**: PR-6 (多租户工作空间隔离)
- **后续依赖**: PR-8 (配置迁移工具)

---

### PR-8: 配置迁移工具

#### 🎯 **功能范围**
- PAT 到 GitHub App 配置转换工具
- 配置验证和测试工具
- 迁移指南和文档
- 配置备份和回滚机制

#### 🏛️ **架构设计**
```
cmd/migrate/
├── main.go            # 迁移工具入口
├── pat_to_app.go      # PAT 转 App 逻辑
├── validate.go        # 配置验证
└── backup.go          # 备份和回滚
```

#### 🔧 **核心组件**

**Migration Tool**
```go
type MigrationTool struct {
    configPath   string
    backupPath   string
    dryRun       bool
}

func (m *MigrationTool) MigratePATToApp(appConfig GitHubAppConfig) error
func (m *MigrationTool) ValidateConfiguration() error
func (m *MigrationTool) TestConnectivity() error
```

#### ⚙️ **实现原理**
1. **安全第一**: 迁移前自动备份现有配置
2. **验证机制**: 迁移后测试新配置的连接性和权限
3. **Dry Run**: 支持预览模式，不实际修改配置
4. **详细日志**: 记录迁移过程，便于问题诊断

#### 🔗 **依赖关系**
- **前置依赖**: PR-7 (向后兼容实现)
- **后续依赖**: PR-9 (监控和文档)

---

### PR-9: 监控、权限验证和文档

#### 🎯 **功能范围**
- 认证相关的监控指标
- 细粒度权限验证中间件
- 完整的部署和使用文档
- 故障排查手册

#### 🏛️ **架构设计**
```
internal/middleware/
├── auth_middleware.go    # 认证中间件
├── permission_check.go   # 权限检查
└── metrics.go           # 监控指标

docs/
├── github-app-setup.md  # GitHub App 创建指南
├── deployment.md        # 部署文档
└── troubleshooting.md   # 故障排查
```

#### 🔧 **核心组件**

**权限验证中间件**
```go
type PermissionMiddleware struct {
    requiredPermissions []string
}

func (m *PermissionMiddleware) CheckPermissions(ctx context.Context, installationID int64) error
```

**监控指标**
```go
type AuthMetrics struct {
    TokenRefreshCount      prometheus.Counter
    AuthenticationFailures prometheus.Counter
    InstallationCount     prometheus.Gauge
}
```

#### ⚙️ **实现原理**
1. **可观测性**: Prometheus 指标 + 结构化日志
2. **权限最小化**: 运行时验证实际权限需求
3. **用户体验**: 友好的错误消息和解决建议
4. **文档驱动**: 完整的操作手册和最佳实践

#### 🔗 **依赖关系**
- **前置依赖**: PR-8 (配置迁移工具)
- **后续依赖**: 无

---

## 🚀 实施时间线和依赖图

### 依赖关系图
```
PR-1 (JWT) ────────────────────────┐
                                   ▼
PR-2 (Token管理) ──────────────────┐
                                   ▼
PR-3 (配置扩展) ──────────────────┐ ▼
                                   ▼ ▼
PR-4 (认证抽象层) ◄──────────────────┘
         │
         ▼
PR-5 (Installation管理)
         │
         ▼
PR-6 (工作空间隔离)
         │
         ▼
PR-7 (向后兼容)
         │
         ▼
PR-8 (迁移工具)
         │
         ▼
PR-9 (监控文档)
```

### 建议实施顺序

#### 🏃‍♂️ **Sprint 1 (Week 1-2): 基础设施**
- PR-1: JWT 生成和签名模块
- PR-3: 配置系统扩展
- 并行开发，无依赖冲突

#### 🏃‍♂️ **Sprint 2 (Week 3-4): 认证核心**  
- PR-2: Installation Token 管理
- PR-4: 认证抽象层重构
- 顺序开发，PR-2 完成后开始 PR-4

#### 🏃‍♂️ **Sprint 3 (Week 5-6): 多租户支持**
- PR-5: Installation 管理和发现
- PR-6: 多租户工作空间隔离  
- 顺序开发

#### 🏃‍♂️ **Sprint 4 (Week 7-8): 兼容性和迁移**
- PR-7: 向后兼容性实现
- PR-8: 配置迁移工具
- 顺序开发，可部分并行

#### 🏃‍♂️ **Sprint 5 (Week 9): 完善和文档**
- PR-9: 监控、权限验证和文档
- 最终集成测试和优化

---

## 💡 实施建议

### 🔄 **开发策略**
1. **测试驱动**: 每个 PR 都包含完整的单元测试和集成测试
2. **渐进式发布**: 每个 PR 合并后进行部分功能验证
3. **文档先行**: 核心 PR (1-6) 都包含详细的 API 文档
4. **向后兼容**: 确保每个 PR 不破坏现有功能

### 🎯 **质量保证**  
1. **代码审查**: 每个 PR 至少 2 人审查，重点关注安全性
2. **自动测试**: CI/CD 流水线覆盖单元测试、集成测试、安全扫描
3. **性能测试**: Token 缓存和并发处理的性能验证
4. **安全审计**: Private Key 处理和权限验证的安全审查

### 📋 **风险控制**
1. **回滚方案**: 每个 PR 都准备对应的回滚计划
2. **监控告警**: 关键指标的实时监控和告警
3. **灰度发布**: 先在测试环境验证，再逐步推广到生产
4. **紧急响应**: 建立快速响应和修复机制

---

## 📊 **总结**

本计划将 GitHub App 认证功能合理拆分为 **9 个独立且有序的 PR**，每个 PR 都有：
- ✅ 明确的功能边界和职责范围  
- ✅ 详细的架构设计和实现原理
- ✅ 清晰的依赖关系和实施顺序
- ✅ 完整的测试和文档要求

通过这种拆分方式，可以实现：
- 🚀 **渐进式交付**: 每个 PR 可独立开发、测试和部署
- 🔒 **风险可控**: 问题范围明确，便于快速定位和修复  
- 👥 **团队协作**: 多人可并行开发不同模块
- 📈 **质量保证**: 每个组件都有充分的测试和文档

预计总体实施周期为 **9 周**，可根据团队资源和优先级适当调整。

---

## 📚 相关资源

- [GitHub Apps Documentation](https://docs.github.com/en/developers/apps)
- [JWT RFC 7519](https://tools.ietf.org/html/rfc7519)
- [Go JWT Library](https://github.com/golang-jwt/jwt)
- [GitHub REST API](https://docs.github.com/en/rest)

---

*该计划文档版本: v1.0 | 最后更新: 2025-08-11*

---