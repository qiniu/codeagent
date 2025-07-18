# Workspace 管理

本模块负责管理代码代理的工作空间，包括 Issue、PR 和 Session 目录的创建、移动和清理。

## 目录格式规范

所有目录都遵循统一的命名格式，包含 AI 模型信息以便区分不同的 AI 处理会话。

### Issue 目录格式

- **格式**: `{aiModel}-{repo}-issue-{issueNumber}-{timestamp}`
- **示例**: `gemini-codeagent-issue-123-1752829201`

### PR 目录格式

- **格式**: `{aiModel}-{repo}-pr-{prNumber}-{timestamp}`
- **示例**: `gemini-codeagent-pr-161-1752829201`

### Session 目录格式

- **格式**: `{aiModel}-{repo}-session-{prNumber}-{timestamp}`
- **示例**: `gemini-codeagent-session-161-1752829201`

## 核心功能

### 1. 目录格式管理 (`format.go`)

提供统一的目录格式生成和解析功能，作为 `Manager` 的内部组件：

- `generateIssueDirName()` - 生成 Issue 目录名
- `generatePRDirName()` - 生成 PR 目录名
- `generateSessionDirName()` - 生成 Session 目录名
- `parsePRDirName()` - 解析 PR 目录名
- `extractSuffixFromPRDir()` - 从 PR 目录名提取后缀

### 2. 工作空间管理 (`manager.go`)

负责工作空间的完整生命周期管理，并提供目录格式的公共接口：

#### 目录格式公共方法

- `GenerateIssueDirName()` - 生成 Issue 目录名
- `GeneratePRDirName()` - 生成 PR 目录名
- `GenerateSessionDirName()` - 生成 Session 目录名
- `ParsePRDirName()` - 解析 PR 目录名
- `ExtractSuffixFromPRDir()` - 从 PR 目录名提取后缀
- `ExtractSuffixFromIssueDir()` - 从 Issue 目录名提取后缀

#### 工作空间生命周期管理

- **创建**: 从 Issue 或 PR 创建工作空间
- **移动**: 将 Issue 工作空间移动到 PR 工作空间
- **清理**: 清理过期的工作空间和资源
- **Session 管理**: 创建和管理 AI 会话目录

#### 主要方法

##### 工作空间创建

- `CreateWorkspaceFromIssueWithAI()` - 从 Issue 创建工作空间
- `GetOrCreateWorkspaceForPRWithAI()` - 获取或创建 PR 工作空间

##### 工作空间操作

- `MoveIssueToPR()` - 将 Issue 工作空间移动到 PR
- `CreateSessionPath()` - 创建 Session 目录
- `CleanupWorkspace()` - 清理工作空间

##### 工作空间查询

- `GetAllWorkspacesByPR()` - 获取 PR 的所有工作空间
- `GetExpiredWorkspaces()` - 获取过期的工作空间

## 使用示例

```go
// 创建工作空间管理器
manager := NewManager(config)

// 通过 Manager 调用目录格式功能
prDirName := manager.GeneratePRDirName("gemini", "codeagent", 161, 1752829201)
// 结果: "gemini-codeagent-pr-161-1752829201"

// 解析 PR 目录名
prInfo, err := manager.ParsePRDirName("gemini-codeagent-pr-161-1752829201")
if err == nil {
    fmt.Printf("AI Model: %s, Repo: %s, PR: %d\n",
        prInfo.AIModel, prInfo.Repo, prInfo.PRNumber)
}

// 从 Issue 创建工作空间
ws := manager.CreateWorkspaceFromIssueWithAI(issue, "gemini")

// 移动到 PR
err = manager.MoveIssueToPR(ws, prNumber)

// 创建 Session 目录
sessionPath, err := manager.CreateSessionPath(ws.Path, "gemini", "codeagent", prNumber, "1752829201")
```

## 设计原则

1. **封装性**: `dirFormatter` 作为 `Manager` 的内部组件，不直接暴露给外部
2. **统一接口**: 所有目录格式功能通过 `Manager` 的公共方法调用
3. **统一格式**: 所有目录都遵循相同的命名规范
4. **AI 模型区分**: 通过 AI 模型信息区分不同的处理会话
5. **时间戳标识**: 使用时间戳确保目录名唯一性
6. **生命周期管理**: 完整的工作空间创建、移动、清理流程
7. **错误处理**: 完善的错误处理和日志记录

## 测试

运行测试确保功能正确：

```bash
go test ./internal/workspace -v
```

测试覆盖了以下功能：

- 目录名生成
- 目录名解析（包括错误处理）
- 后缀提取
- 工作空间创建和移动
- Session 目录管理
