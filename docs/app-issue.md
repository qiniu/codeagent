### GitHub App 基础认证功能实现计划

#### 任务清单
- [x] 分析之前的需求文档结构
- [x] 将需求按逻辑模块拆分为独立 PR
- [x] 为每个 PR 生成详细的架构文档
- [x] 定义 PR 之间的依赖关系和实施顺序
- [x] 完成基础功能的 PR 拆分计划

---

## 📋 PR 拆分方案概览

基于需求分析，将 GitHub App 基础认证功能拆分为 **4 个独立的 PR**，实现核心认证功能：

### 🏗️ 核心基础设施层 (PR 1-4)
- **PR-1**: JWT 生成和签名模块
- **PR-2**: Installation Token 管理 
- **PR-3**: 配置系统扩展
- **PR-4**: 认证抽象层重构

### 🎯 核心目标
实现 GitHub App 认证功能，当配置了 GitHub App 时，GitHub Client 自动使用 App 认证方式，保持与现有 PAT 认证的兼容性。

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
- **后续依赖**: PR-5 (Client自动切换)

### PR-5: GitHub Client 自动认证切换

#### 🎯 **功能范围**
- 在配置了 GitHub App 时，GitHub Client 自动使用 App 认证
- 保持与现有 PAT 认证的完全兼容性
- 运行时认证方式自动检测和切换
- 简化的客户端获取接口

#### 🏛️ **架构设计**
```go
// 在现有 GitHub Client 基础上添加认证方式检测
type GitHubClientManager struct {
    config        *Config
    authenticator Authenticator  // PR-4 中定义的接口
}

func (m *GitHubClientManager) GetClient(ctx context.Context) (*github.Client, error) {
    // 自动检测配置的认证方式
    if m.config.GitHub.App.AppID != 0 {
        // 使用 GitHub App 认证
        return m.authenticator.GetInstallationClient(ctx, m.getInstallationID(ctx))
    }
    // 回退到 PAT 认证
    return m.authenticator.GetClient(ctx)
}
```

#### 🔧 **核心组件**

**认证方式检测器**
```go
type AuthModeDetector struct {
    config *Config
}

func (d *AuthModeDetector) DetectAuthMode() AuthMode {
    if d.config.GitHub.App.AppID != 0 && d.config.GitHub.App.PrivateKeyPath != "" {
        return AuthModeApp
    }
    return AuthModePAT
}
```

#### ⚙️ **实现原理**
1. **配置驱动**: 基于配置文件自动选择认证方式
2. **优雅降级**: App 认证失败时自动回退到 PAT
3. **透明切换**: 业务代码无需修改，保持原有接口
4. **Installation ID 获取**: 从 Webhook 上下文中提取 Installation ID

#### 🔗 **依赖关系**
- **前置依赖**: PR-4 (认证抽象层)
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
PR-5 (Client自动切换)
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

#### 🏃‍♂️ **Sprint 3 (Week 5): 自动切换**
- PR-5: GitHub Client 自动认证切换
- 集成测试和验证

---

## 💡 实施建议

### 🔄 **开发策略**
1. **测试驱动**: 每个 PR 都包含完整的单元测试和集成测试
2. **渐进式发布**: 每个 PR 合并后进行部分功能验证
3. **文档先行**: 核心 PR (1-5) 都包含详细的 API 文档
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

本计划将 GitHub App 基础认证功能合理拆分为 **5 个独立且有序的 PR**，每个 PR 都有：
- ✅ 明确的功能边界和职责范围  
- ✅ 详细的架构设计和实现原理
- ✅ 清晰的依赖关系和实施顺序
- ✅ 完整的测试和文档要求

通过这种拆分方式，可以实现：
- 🚀 **渐进式交付**: 每个 PR 可独立开发、测试和部署
- 🔒 **风险可控**: 问题范围明确，便于快速定位和修复  
- 👥 **团队协作**: 多人可并行开发不同模块
- 📈 **质量保证**: 每个组件都有充分的测试和文档

**核心目标**: 实现 GitHub App 基础认证功能，当配置了 GitHub App 时，GitHub Client 自动使用 App 认证，保持与现有 PAT 认证的完全兼容性。

预计总体实施周期为 **5 周**，可根据团队资源和优先级适当调整。

---

## 📚 相关资源

- [GitHub Apps Documentation](https://docs.github.com/en/developers/apps)
- [JWT RFC 7519](https://tools.ietf.org/html/rfc7519)
- [Go JWT Library](https://github.com/golang-jwt/jwt)
- [GitHub REST API](https://docs.github.com/en/rest)

---

*该计划文档版本: v1.0 | 最后更新: 2025-08-11*

---