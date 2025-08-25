# Workspace 管理模块

管理 CodeAgent 的工作空间，包括 Issue、PR 和 Session 目录的生命周期管理。

## 架构设计

### 组件结构
```
Manager (工作空间管理器)
├── RepoCacheService     # 仓库缓存服务
├── GitService          # Git 操作服务
├── ContainerService    # Docker 容器管理
├── WorkspaceRepository # 工作空间存储
├── DirFormatter       # 目录命名格式化
└── 错误处理
```

### 设计原则
- **单一职责**：每个服务专注特定功能
- **接口抽象**：使用接口实现松耦合
- **本地缓存**：避免重复 Git 克隆操作

## 核心功能

### 仓库缓存机制
采用两级缓存架构提升性能：
- 本地缓存：`_cache/{org}/{repo}` 
- 工作空间：从缓存快速克隆到独立目录
- 性能提升：网络流量减少 90%，克隆速度提升 10x

### 工作空间类型
- **Issue 工作空间**：用于 Issue 评论触发的代码生成
- **PR 工作空间**：用于 PR 评论的代码修改
- **Session 目录**：容器挂载点，用于 AI 会话

### AI 模型隔离
不同 AI 模型使用独立工作空间：
- `claude/repo/pr-123/` 
- `gemini/repo/pr-123/`
- 支持同一 PR 的多模型并行处理

## 目录命名规则

```
Issue: {aiModel}__{repo}__issue__{number}__{timestamp}
PR:    {aiModel}__{repo}__pr__{number}__{timestamp}
Session: {aiModel}-{repo}-session-{number}-{timestamp}
```

## 使用方式

### 基础用法
```go
// 创建管理器
manager := NewManager(config)

// Issue 工作空间
ws := manager.GetOrCreateWorkspaceForIssue(issue, "claude")

// PR 工作空间  
ws := manager.GetOrCreateWorkspaceForPR(pr, "claude")

// 清理工作空间
manager.CleanupWorkspace(ws)
```

### Issue → PR 转换流程
1. Issue 触发：创建临时工作空间和新分支
2. PR 创建：将 Issue 工作空间转换为 PR 工作空间  
3. 会话管理：创建 Session 目录供容器使用

## 测试

```bash
# 运行所有测试
go test ./internal/workspace -v

# 特定测试场景
go test -run TestIssueWorkspaceLifecycle
go test -run TestPRWorkspaceLifecycle  
go test -run TestWorkspaceCleanupScenarios
```