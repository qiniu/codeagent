# CodeAgent 产品设计文档

## 1. 项目概述

### 1.1 项目背景

基于现有的 CodeAgent 项目，扩展为一个功能完整的 AI 服务代理平台，支持多种 AI 模型、账号池管理、负载均衡等功能，为开发者提供统一的 AI 服务接口。

### 1.2 核心目标

1. **支持 Classfile 编码**: 扩展 AI 模型支持，包括 Claude、Gemini 等
2. **融合 CLI 支持**: 智能选择最适合的 CLI 工具，优化成本
3. **多渠道账号池管理**: 统一管理多个 AI 服务商的账号
4. **账号状态监控**: 实时监控账号状态和用量
5. **OpenAI 范式 API**: 提供标准化的 API 接口
6. **智能负载均衡**: 基于用量和成本的负载均衡策略

### 1.3 技术架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Client Apps   │    │   CodeAgent     │    │   AI Providers  │
│   (OpenAI API)  │───▶│   Gateway       │───▶│   (Claude/Gemini)│
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌─────────────────┐
                       │   Account Pool  │
                       │   Management    │
                       └─────────────────┘
```

## 2. 功能模块设计

### 2.1 Classfile 编码支持模块

#### 2.1.1 功能描述

扩展现有的 AI 模型支持，增加对 Classfile 格式的编码支持，提供统一的模型接口。

#### 2.1.2 技术实现

**模型接口设计**:
```go
// models/classfile.go
type ClassfileProvider interface {
    Encode(input []byte) ([]byte, error)
    Decode(input []byte) ([]byte, error)
    Validate(input []byte) error
}

type ClassfileManager struct {
    providers map[string]ClassfileProvider
    config    *config.ClassfileConfig
}

func (cm *ClassfileManager) EncodeWithProvider(provider string, input []byte) ([]byte, error) {
    if p, exists := cm.providers[provider]; exists {
        return p.Encode(input)
    }
    return nil, fmt.Errorf("provider %s not found", provider)
}
```

**配置结构**:
```yaml
classfile:
  providers:
    claude:
      enabled: true
      encoding_format: "base64"
      max_input_size: "10MB"
    gemini:
      enabled: true
      encoding_format: "hex"
      max_input_size: "5MB"
  default_provider: "claude"
  fallback_provider: "gemini"
```

#### 2.1.3 API 接口

```http
POST /api/v1/classfile/encode
Content-Type: application/json

{
  "provider": "claude",
  "input": "base64_encoded_data",
  "format": "base64"
}

Response:
{
  "success": true,
  "data": "encoded_data",
  "provider": "claude",
  "encoding_time": "1.2s"
}
```

### 2.2 融合 CLI 模块

#### 2.2.1 功能描述

智能选择最适合的 CLI 工具来处理不同类型的任务，优化成本和性能。

#### 2.2.2 任务分类

**编码任务**:
- 代码生成
- 代码审查
- 代码重构
- 单元测试生成

**非编码任务**:
- 文档生成
- 问题解答
- 文本总结
- 翻译任务

#### 2.2.3 CLI 选择策略

```go
// internal/cli/selector.go
type TaskType string

const (
    TaskTypeCodeGeneration TaskType = "code_generation"
    TaskTypeCodeReview     TaskType = "code_review"
    TaskTypeDocumentation  TaskType = "documentation"
    TaskTypeTranslation    TaskType = "translation"
    TaskTypeQnA           TaskType = "qa"
)

type CLISelector struct {
    costMatrix map[TaskType]map[string]float64
    performanceMatrix map[TaskType]map[string]float64
}

func (cs *CLISelector) SelectCLI(taskType TaskType, budget float64) (string, error) {
    // 基于任务类型、预算、性能要求选择最优 CLI
    candidates := cs.getCandidates(taskType)
    
    // 成本优化
    costOptimized := cs.optimizeByCost(candidates, budget)
    
    // 性能优化
    performanceOptimized := cs.optimizeByPerformance(costOptimized)
    
    return cs.selectBest(performanceOptimized)
}
```

**选择策略**:
```go
func (cs *CLISelector) getSelectionStrategy(taskType TaskType) SelectionStrategy {
    switch taskType {
    case TaskTypeCodeGeneration:
        return &CodeGenerationStrategy{
            preferredCLIs: []string{"claude-code", "gemini-code"},
            fallbackCLIs:  []string{"claude", "gemini"},
        }
    case TaskTypeDocumentation:
        return &DocumentationStrategy{
            preferredCLIs: []string{"gemini", "claude"},
            costWeight:    0.7,
            qualityWeight: 0.3,
        }
    case TaskTypeQnA:
        return &QnAStrategy{
            preferredCLIs: []string{"gemini", "claude"},
            responseTimeWeight: 0.8,
            costWeight:         0.2,
        }
    }
}
```

#### 2.2.4 成本优化算法

```go
type CostOptimizer struct {
    pricing map[string]PricingInfo
}

type PricingInfo struct {
    InputTokenCost  float64
    OutputTokenCost float64
    BaseCost        float64
}

func (co *CostOptimizer) EstimateCost(cli string, inputTokens, outputTokens int) float64 {
    pricing := co.pricing[cli]
    return pricing.BaseCost + 
           float64(inputTokens)*pricing.InputTokenCost + 
           float64(outputTokens)*pricing.OutputTokenCost
}

func (co *CostOptimizer) OptimizeForBudget(task Task, budget float64) []CLIOption {
    var options []CLIOption
    
    for cli, pricing := range co.pricing {
        estimatedCost := co.EstimateCost(cli, task.EstimatedInputTokens, task.EstimatedOutputTokens)
        if estimatedCost <= budget {
            options = append(options, CLIOption{
                CLI:   cli,
                Cost:  estimatedCost,
                Score: co.calculateScore(cli, task),
            })
        }
    }
    
    // 按性价比排序
    sort.Slice(options, func(i, j int) bool {
        return options[i].Score/options[i].Cost > options[j].Score/options[j].Cost
    })
    
    return options
}
```

### 2.3 多渠道账号池管理模块

#### 2.3.1 功能描述

统一管理多个 AI 服务商的账号，包括账号状态监控、用量统计、负载均衡等功能。

#### 2.3.2 数据模型

```go
// models/account.go
type Account struct {
    ID           string    `json:"id"`
    Provider     string    `json:"provider"`     // claude, gemini, openai
    AccountName  string    `json:"account_name"`
    AccessToken  string    `json:"access_token"`
    Status       string    `json:"status"`       // active, inactive, suspended
    Quota        Quota     `json:"quota"`
    Usage        Usage     `json:"usage"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
    LastUsedAt   time.Time `json:"last_used_at"`
}

type Quota struct {
    MonthlyLimit    int64   `json:"monthly_limit"`
    DailyLimit      int64   `json:"daily_limit"`
    RateLimit       int     `json:"rate_limit"`      // requests per minute
    CostPerToken    float64 `json:"cost_per_token"`
    Currency        string  `json:"currency"`
}

type Usage struct {
    MonthlyUsed     int64   `json:"monthly_used"`
    DailyUsed       int64   `json:"daily_used"`
    TotalCost       float64 `json:"total_cost"`
    LastResetDate   time.Time `json:"last_reset_date"`
}
```

#### 2.3.3 账号池管理器

```go
// internal/account/pool.go
type AccountPool struct {
    accounts map[string]*Account
    mutex    sync.RWMutex
    config   *config.AccountPoolConfig
}

func (ap *AccountPool) GetAccount(provider string, requirements AccountRequirements) (*Account, error) {
    ap.mutex.RLock()
    defer ap.mutex.RUnlock()
    
    var candidates []*Account
    
    for _, account := range ap.accounts {
        if account.Provider == provider && ap.matchesRequirements(account, requirements) {
            candidates = append(candidates, account)
        }
    }
    
    if len(candidates) == 0 {
        return nil, fmt.Errorf("no available account for provider %s", provider)
    }
    
    // 负载均衡选择
    return ap.selectAccount(candidates, requirements)
}

func (ap *AccountPool) selectAccount(candidates []*Account, requirements AccountRequirements) (*Account, error) {
    // 基于用量的负载均衡
    var bestAccount *Account
    var bestScore float64
    
    for _, account := range candidates {
        score := ap.calculateScore(account, requirements)
        if score > bestScore {
            bestScore = score
            bestAccount = account
        }
    }
    
    return bestAccount, nil
}

func (ap *AccountPool) calculateScore(account *Account, requirements AccountRequirements) float64 {
    // 计算账号评分
    usageRatio := float64(account.Usage.MonthlyUsed) / float64(account.Quota.MonthlyLimit)
    costEfficiency := 1.0 / account.Quota.CostPerToken
    availability := 1.0 - usageRatio
    
    return usageRatio*0.3 + costEfficiency*0.4 + availability*0.3
}
```

#### 2.3.4 账号状态监控

```go
// internal/account/monitor.go
type AccountMonitor struct {
    pool    *AccountPool
    ticker  *time.Ticker
    config  *config.MonitorConfig
}

func (am *AccountMonitor) Start() {
    am.ticker = time.NewTicker(am.config.CheckInterval)
    go am.monitorLoop()
}

func (am *AccountMonitor) monitorLoop() {
    for range am.ticker.C {
        am.checkAccountStatus()
        am.updateUsage()
        am.validateTokens()
    }
}

func (am *AccountMonitor) checkAccountStatus() {
    am.pool.mutex.RLock()
    defer am.pool.mutex.RUnlock()
    
    for _, account := range am.pool.accounts {
        // 检查账号状态
        status, err := am.checkProviderStatus(account)
        if err != nil {
            log.Errorf("Failed to check status for account %s: %v", account.ID, err)
            account.Status = "error"
        } else {
            account.Status = status
        }
        
        // 更新用量信息
        usage, err := am.getProviderUsage(account)
        if err == nil {
            account.Usage = usage
        }
    }
}

func (am *AccountMonitor) validateTokens() {
    for _, account := range am.pool.accounts {
        if am.isTokenExpired(account) {
            // 尝试刷新 token
            if err := am.refreshToken(account); err != nil {
                log.Errorf("Failed to refresh token for account %s: %v", account.ID, err)
                account.Status = "token_expired"
            }
        }
    }
}
```

### 2.4 OpenAI 范式 API 模块

#### 2.4.1 功能描述

提供标准化的 OpenAI 兼容 API 接口，支持 API Key 认证、QPS 限制、用量统计等功能。

#### 2.4.2 API 接口设计

**聊天完成接口**:
```http
POST /v1/chat/completions
Authorization: Bearer sk-xxxxxxxxxxxxxxxx
Content-Type: application/json

{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ],
  "max_tokens": 100,
  "temperature": 0.7
}

Response:
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "gpt-3.5-turbo",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}
```

#### 2.4.3 API Key 管理

```go
// internal/api/key_manager.go
type APIKey struct {
    ID          string    `json:"id"`
    Key         string    `json:"key"`
    UserID      string    `json:"user_id"`
    Permissions []string  `json:"permissions"`
    RateLimit   RateLimit `json:"rate_limit"`
    CreatedAt   time.Time `json:"created_at"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type RateLimit struct {
    RequestsPerMinute int `json:"requests_per_minute"`
    RequestsPerHour   int `json:"requests_per_hour"`
    RequestsPerDay    int `json:"requests_per_day"`
}

type KeyManager struct {
    keys map[string]*APIKey
    mutex sync.RWMutex
}

func (km *KeyManager) ValidateKey(key string) (*APIKey, error) {
    km.mutex.RLock()
    defer km.mutex.RUnlock()
    
    if apiKey, exists := km.keys[key]; exists {
        if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
            return nil, fmt.Errorf("API key expired")
        }
        return apiKey, nil
    }
    
    return nil, fmt.Errorf("invalid API key")
}
```

#### 2.4.4 QPS 限制和用量统计

```go
// internal/api/rate_limiter.go
type RateLimiter struct {
    limits map[string]*TokenBucket
    mutex  sync.RWMutex
}

type TokenBucket struct {
    tokens     int
    capacity   int
    refillRate float64
    lastRefill time.Time
    mutex      sync.Mutex
}

func (rl *RateLimiter) Allow(key string) bool {
    rl.mutex.RLock()
    bucket, exists := rl.limits[key]
    rl.mutex.RUnlock()
    
    if !exists {
        rl.mutex.Lock()
        bucket = &TokenBucket{
            tokens:     100,
            capacity:  100,
            refillRate: 1.0, // tokens per second
            lastRefill: time.Now(),
        }
        rl.limits[key] = bucket
        rl.mutex.Unlock()
    }
    
    bucket.mutex.Lock()
    defer bucket.mutex.Unlock()
    
    // 补充令牌
    now := time.Now()
    elapsed := now.Sub(bucket.lastRefill).Seconds()
    tokensToAdd := int(elapsed * bucket.refillRate)
    
    if tokensToAdd > 0 {
        bucket.tokens = min(bucket.capacity, bucket.tokens+tokensToAdd)
        bucket.lastRefill = now
    }
    
    // 检查是否有可用令牌
    if bucket.tokens > 0 {
        bucket.tokens--
        return true
    }
    
    return false
}
```

### 2.5 负载均衡模块

#### 2.5.1 功能描述

基于账号用量、成本、性能等因素进行智能负载均衡，确保资源的最优利用。

#### 2.5.2 负载均衡策略

```go
// internal/loadbalancer/balancer.go
type LoadBalancer struct {
    accounts *account.AccountPool
    metrics  *MetricsCollector
    config   *config.LoadBalancerConfig
}

type LoadBalancingStrategy interface {
    SelectAccount(accounts []*Account, request Request) (*Account, error)
}

type RoundRobinStrategy struct {
    current int
    mutex   sync.Mutex
}

type WeightedStrategy struct {
    weights map[string]float64
}

type CostOptimizedStrategy struct {
    budget float64
}

func (lb *LoadBalancer) SelectAccount(request Request) (*Account, error) {
    // 获取可用账号
    accounts := lb.getAvailableAccounts(request.Provider)
    
    // 根据策略选择账号
    var strategy LoadBalancingStrategy
    
    switch lb.config.Strategy {
    case "round_robin":
        strategy = &RoundRobinStrategy{}
    case "weighted":
        strategy = &WeightedStrategy{weights: lb.config.Weights}
    case "cost_optimized":
        strategy = &CostOptimizedStrategy{budget: request.Budget}
    default:
        strategy = &RoundRobinStrategy{}
    }
    
    return strategy.SelectAccount(accounts, request)
}
```

#### 2.5.3 成本优化算法

```go
func (cos *CostOptimizedStrategy) SelectAccount(accounts []*Account, request Request) (*Account, error) {
    var candidates []*Account
    
    // 筛选在预算内的账号
    for _, account := range accounts {
        estimatedCost := cos.estimateCost(account, request)
        if estimatedCost <= cos.budget {
            candidates = append(candidates, account)
        }
    }
    
    if len(candidates) == 0 {
        return nil, fmt.Errorf("no account available within budget")
    }
    
    // 选择成本最低的账号
    var bestAccount *Account
    var lowestCost float64 = math.MaxFloat64
    
    for _, account := range candidates {
        cost := cos.estimateCost(account, request)
        if cost < lowestCost {
            lowestCost = cost
            bestAccount = account
        }
    }
    
    return bestAccount, nil
}

func (cos *CostOptimizedStrategy) estimateCost(account *Account, request Request) float64 {
    // 基于 token 数量估算成本
    estimatedTokens := request.EstimatedTokens
    costPerToken := account.Quota.CostPerToken
    
    return float64(estimatedTokens) * costPerToken
}
```

## 3. 数据库设计

### 3.1 核心表结构

```sql
-- 账号表
CREATE TABLE accounts (
    id VARCHAR(36) PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    account_name VARCHAR(100) NOT NULL,
    access_token TEXT NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    quota_monthly_limit BIGINT,
    quota_daily_limit BIGINT,
    quota_rate_limit INT,
    quota_cost_per_token DECIMAL(10,6),
    quota_currency VARCHAR(10),
    usage_monthly_used BIGINT DEFAULT 0,
    usage_daily_used BIGINT DEFAULT 0,
    usage_total_cost DECIMAL(10,2) DEFAULT 0,
    usage_last_reset_date TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

-- API Key 表
CREATE TABLE api_keys (
    id VARCHAR(36) PRIMARY KEY,
    key_hash VARCHAR(255) NOT NULL UNIQUE,
    user_id VARCHAR(36) NOT NULL,
    permissions JSON,
    rate_limit_requests_per_minute INT DEFAULT 60,
    rate_limit_requests_per_hour INT DEFAULT 1000,
    rate_limit_requests_per_day INT DEFAULT 10000,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL,
    INDEX idx_user_id (user_id),
    INDEX idx_key_hash (key_hash)
);

-- 请求日志表
CREATE TABLE request_logs (
    id VARCHAR(36) PRIMARY KEY,
    api_key_id VARCHAR(36),
    account_id VARCHAR(36),
    provider VARCHAR(50),
    model VARCHAR(100),
    request_type VARCHAR(50),
    input_tokens INT,
    output_tokens INT,
    total_tokens INT,
    cost DECIMAL(10,6),
    duration_ms INT,
    status VARCHAR(20),
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_api_key_id (api_key_id),
    INDEX idx_account_id (account_id),
    INDEX idx_created_at (created_at)
);

-- 用量统计表
CREATE TABLE usage_stats (
    id VARCHAR(36) PRIMARY KEY,
    account_id VARCHAR(36),
    date DATE,
    provider VARCHAR(50),
    total_requests INT DEFAULT 0,
    total_tokens INT DEFAULT 0,
    total_cost DECIMAL(10,2) DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_account_date (account_id, date),
    INDEX idx_provider_date (provider, date)
);
```

## 4. 配置管理

### 4.1 主配置文件

```yaml
# config.yaml
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: "30s"
  write_timeout: "30s"

database:
  driver: "mysql"
  dsn: "user:password@tcp(localhost:3306)/codeagent?parseTime=true"
  max_open_conns: 100
  max_idle_conns: 10
  conn_max_lifetime: "1h"

api:
  enable_openai_compatibility: true
  default_model: "gpt-3.5-turbo"
  max_tokens_per_request: 4000
  rate_limit_enabled: true
  cors_enabled: true
  cors_origins: ["*"]

account_pool:
  auto_refresh_tokens: true
  refresh_interval: "1h"
  max_concurrent_requests: 100
  health_check_interval: "5m"

load_balancer:
  strategy: "cost_optimized"  # round_robin, weighted, cost_optimized
  weights:
    claude: 0.6
    gemini: 0.4
  cost_budget_percentage: 0.8

classfile:
  providers:
    claude:
      enabled: true
      encoding_format: "base64"
      max_input_size: "10MB"
    gemini:
      enabled: true
      encoding_format: "hex"
      max_input_size: "5MB"

cli_selector:
  enable_cost_optimization: true
  enable_performance_optimization: true
  task_classification:
    code_generation:
      preferred_clis: ["claude-code", "gemini-code"]
      cost_weight: 0.4
      quality_weight: 0.6
    documentation:
      preferred_clis: ["gemini", "claude"]
      cost_weight: 0.7
      quality_weight: 0.3

monitoring:
  enable_metrics: true
  metrics_port: 9090
  enable_tracing: true
  log_level: "info"
```

## 5. API 接口设计

### 5.1 OpenAI 兼容接口

#### 5.1.1 聊天完成接口

```http
POST /v1/chat/completions
Authorization: Bearer sk-xxxxxxxxxxxxxxxx
Content-Type: application/json

{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "max_tokens": 100,
  "temperature": 0.7,
  "top_p": 1,
  "frequency_penalty": 0,
  "presence_penalty": 0
}

Response:
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "gpt-3.5-turbo",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}
```

### 5.2 管理接口

#### 5.2.1 账号管理接口

```http
# 获取账号列表
GET /api/v1/accounts
Authorization: Bearer admin-token

Response:
{
  "accounts": [
    {
      "id": "acc-123",
      "provider": "claude",
      "account_name": "claude-account-1",
      "status": "active",
      "usage": {
        "monthly_used": 1000000,
        "monthly_limit": 10000000,
        "daily_used": 50000,
        "daily_limit": 100000
      },
      "cost": {
        "total_cost": 15.50,
        "currency": "USD"
      }
    }
  ]
}

# 添加账号
POST /api/v1/accounts
Authorization: Bearer admin-token
Content-Type: application/json

{
  "provider": "claude",
  "account_name": "claude-account-2",
  "access_token": "sk-ant-api03-...",
  "quota": {
    "monthly_limit": 10000000,
    "daily_limit": 100000,
    "rate_limit": 100,
    "cost_per_token": 0.000015
  }
}
```

#### 5.2.2 用量统计接口

```http
# 获取用量统计
GET /api/v1/usage?start_date=2024-01-01&end_date=2024-01-31
Authorization: Bearer admin-token

Response:
{
  "period": {
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
  },
  "total_requests": 15000,
  "total_tokens": 5000000,
  "total_cost": 75.00,
  "currency": "USD",
  "by_provider": {
    "claude": {
      "requests": 8000,
      "tokens": 2500000,
      "cost": 37.50
    },
    "gemini": {
      "requests": 7000,
      "tokens": 2500000,
      "cost": 37.50
    }
  },
  "by_account": [
    {
      "account_id": "acc-123",
      "account_name": "claude-account-1",
      "provider": "claude",
      "requests": 4000,
      "tokens": 1250000,
      "cost": 18.75
    }
  ]
}
```

## 6. 监控和运维

### 6.1 监控指标

#### 6.1.1 系统指标

- **请求量**: QPS、总请求数、成功率
- **延迟**: 平均响应时间、P95、P99
- **错误率**: 4xx、5xx 错误率
- **资源使用**: CPU、内存、磁盘、网络

#### 6.1.2 业务指标

- **账号使用情况**: 各账号的请求量、用量、成本
- **模型性能**: 各模型的响应时间、成功率
- **成本控制**: 总成本、成本趋势、预算使用率
- **负载均衡**: 各账号的负载分布

### 6.2 告警规则

```yaml
alerts:
  - name: "high_error_rate"
    condition: "error_rate > 0.05"
    duration: "5m"
    severity: "critical"
    
  - name: "high_latency"
    condition: "p95_latency > 10s"
    duration: "5m"
    severity: "warning"
    
  - name: "account_quota_exceeded"
    condition: "account_usage > account_quota * 0.9"
    duration: "1m"
    severity: "warning"
    
  - name: "cost_budget_exceeded"
    condition: "daily_cost > daily_budget"
    duration: "1h"
    severity: "critical"
```

## 7. 部署方案

### 7.1 Docker 部署

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o codeagent ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/codeagent .
COPY --from=builder /app/config.yaml .

EXPOSE 8080
CMD ["./codeagent"]
```

```yaml
# docker-compose.yml
version: '3.8'

services:
  codeagent:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DB_DSN=user:password@tcp(mysql:3306)/codeagent
      - API_ENABLE_OPENAI_COMPATIBILITY=true
    depends_on:
      - mysql
      - redis
    volumes:
      - ./config.yaml:/root/config.yaml
      - ./logs:/root/logs

  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: codeagent
      MYSQL_USER: codeagent
      MYSQL_PASSWORD: password
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana

volumes:
  mysql_data:
  redis_data:
  grafana_data:
```

## 8. 开发计划

### 8.1 第一阶段 (4周)

**目标**: 基础架构搭建
- [ ] 数据库设计和实现
- [ ] 账号池管理模块
- [ ] 基础 API 接口
- [ ] 配置管理系统

**交付物**:
- 数据库 schema
- 账号管理 API
- 基础配置系统

### 8.2 第二阶段 (4周)

**目标**: 核心功能实现
- [ ] OpenAI 兼容 API
- [ ] 负载均衡模块
- [ ] QPS 限制和用量统计
- [ ] Classfile 编码支持

**交付物**:
- OpenAI 兼容 API
- 负载均衡系统
- 用量统计功能

### 8.3 第三阶段 (3周)

**目标**: 高级功能
- [ ] 融合 CLI 选择器
- [ ] 成本优化算法
- [ ] 账号状态监控
- [ ] Token 自动刷新

**交付物**:
- CLI 选择器
- 成本优化系统
- 监控告警系统

### 8.4 第四阶段 (3周)

**目标**: 完善和优化
- [ ] 性能优化
- [ ] 安全加固
- [ ] 文档完善
- [ ] 测试覆盖

**交付物**:
- 性能测试报告
- 安全审计报告
- 完整文档
- 测试用例

## 9. 风险评估

### 9.1 技术风险

1. **AI 服务商 API 变更**: 建立适配层，支持快速切换
2. **性能瓶颈**: 实施缓存和异步处理
3. **数据一致性**: 使用事务和分布式锁

### 9.2 业务风险

1. **成本控制**: 实施预算限制和告警
2. **服务可用性**: 多账号备份和故障转移
3. **合规性**: 数据加密和访问控制

### 9.3 缓解措施

1. **监控告警**: 实时监控关键指标
2. **自动恢复**: 故障自动检测和恢复
3. **降级策略**: 服务降级和备用方案
