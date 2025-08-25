# Workspace 管理

本模块负责管理代码代理的工作空间，包括 Issue、PR 和 Session 目录的创建、移动和清理。

## 核心架构

### 设计理念
- **单一职责**：每个服务组件专注特定功能
- **依赖注入**：通过接口实现松耦合设计
- **本地缓存**：避免重复从GitHub克隆，提升性能

### 组件架构
```
Manager (协调者)
├── RepoCacheService (仓库缓存)    # 核心：本地缓存机制
├── GitService (Git操作)
├── ContainerService (Docker管理) 
├── WorkspaceRepository (存储管理)
├── DirFormatter (目录格式化)
└── 统一错误处理
```

## 关键决策

### 1. 仓库缓存机制 (`repo_cache_service.go`)
**问题**：每次都从GitHub远程克隆，效率低下  
**解决**：本地缓存 + 增量更新
- 第一次：`GitHub远程 → 本地缓存(_cache/org/repo)`
- 后续：`更新缓存 → 从缓存克隆 → 新workspace`
- 性能提升：网络流量减少90%+，克隆速度提升10x+

### 2. 服务分层设计
**问题**：Manager承担过多职责，难以测试和维护  
**解决**：按职责拆分独立服务
- **GitService**：统一Git操作，避免重复代码
- **ContainerService**：Docker容器生命周期管理
- **WorkspaceRepository**：内存存储，支持并发访问

### 3. 接口驱动开发 (`interfaces.go`)
**问题**：代码耦合度高，难以单元测试  
**解决**：接口抽象 + Mock实现
- 所有服务都有接口定义
- 提供MockWorkspaceManager支持测试

### 4. 统一错误处理 (`errors.go`)
**问题**：错误处理不一致，难以调试  
**解决**：自定义错误类型，提供上下文信息
```go
GitError("clone", path, err)      // Git相关错误
ContainerError("remove", name, err) // 容器相关错误
```

## 目录格式
- **Issue**: `{aiModel}__{repo}__issue__{number}__{timestamp}`
- **PR**: `{aiModel}__{repo}__pr__{number}__{timestamp}`  
- **Session**: `{aiModel}-{repo}-session-{number}-{timestamp}`

## 核心工作流程

### Issue → PR 转换
1. Issue创建workspace：从缓存克隆 → 创建新分支
2. PR创建后：重命名目录 `issue → pr`
3. 创建session目录供容器挂载

### 工作空间生命周期
```go
manager := NewManager(config)

// 创建/获取工作空间（自动使用缓存）
ws := manager.GetOrCreateWorkspaceForPRWithAI(pr, "claude")

// 清理（包括容器和目录）
manager.CleanupWorkspace(ws)
```

## 测试

运行测试确保功能正确：

```bash
go test ./internal/workspace -v
```

测试覆盖：目录格式化、缓存机制、工作空间生命周期、错误处理