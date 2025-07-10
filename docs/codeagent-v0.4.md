# CodeAgent v0.4 - 精确工作空间映射设计

## 概述

CodeAgent v0.4 版本重新设计了工作空间管理机制，通过 Git worktree 和精确的映射文件实现：

1. **精确的工作空间映射**：使用 `.git/info/exclude` 目录存储 Issue 到 PR 的映射关系
2. **Git Worktree 性能优化**：继续使用 Git worktree 技术提升性能
3. **持久化映射信息**：确保工作空间信息在系统重启后能准确恢复

## 核心设计理念

### 问题分析

原有设计存在的问题：

1. **映射关系丢失**：Issue 创建的工作空间在转换为 PR 后，映射关系容易丢失
2. **恢复不准确**：系统重启后无法准确恢复 Issue 和 PR 的对应关系
3. **工作空间混乱**：多个 Issue 或 PR 的工作空间容易混淆

### 解决方案

通过 `.git/info/exclude` 目录存储映射文件，实现：

1. **持久化映射**：Issue 到 PR 的映射关系持久化存储
2. **精确恢复**：系统重启后能准确恢复所有工作空间
3. **清晰对应**：每个工作空间都有明确的标识和映射关系

## 详细设计

### 1. 工作空间创建流程

#### 1.1 Issue 阶段创建工作空间

当通过 Issue 的 `/code` 命令创建时：

```bash
# 工作空间目录格式
codeagent/issue-{number}-{timestamp}

# 示例
codeagent/issue-123-1703123456789
```

#### 1.2 创建初始 PR 和 Commit

Agent 创建初始 PR 后，获取到 PR 号，然后：

1. **创建映射文件**：在 `.git/info/exclude` 目录下创建映射文件
2. **文件命名**：`issue-{number}-{timestamp}.txt`
3. **文件内容**：对应的 PR 号

```bash
# 映射文件示例
.git/info/exclude/issue-123-1703123456789.txt
# 文件内容：91
```

#### 1.3 映射文件结构

```
.git/info/exclude/
├── issue-123-1703123456789.txt    # 内容：91
├── issue-124-1703123456790.txt    # 内容：92
└── issue-125-1703123456791.txt    # 内容：93
```

### 2. 工作空间恢复机制

#### 2.1 恢复流程

```go
func (m *Manager) recoverExistingWorkspacesWithMapping() {
    // 1. 扫描所有仓库目录
    for _, repoEntry := range entries {
        repoPath := filepath.Join(m.baseDir, repoEntry.Name())

        // 2. 检查是否有 .git 目录
        gitDir := filepath.Join(repoPath, ".git")
        if _, err := os.Stat(gitDir); os.IsNotExist(err) {
            continue
        }

        // 3. 读取映射文件
        mappingDir := filepath.Join(gitDir, "info", "exclude")
        mappings := m.readMappingFiles(mappingDir)

        // 4. 恢复每个工作空间
        for issueDir, prNumber := range mappings {
            m.recoverWorkspace(repoPath, issueDir, prNumber)
        }
    }
}
```

#### 2.2 映射文件读取

```go
func (m *Manager) readMappingFiles(mappingDir string) map[string]int {
    mappings := make(map[string]int)

    entries, err := os.ReadDir(mappingDir)
    if err != nil {
        return mappings
    }

    for _, entry := range entries {
        if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txt") {
            // 解析文件名获取 Issue 信息
            issueInfo := m.parseIssueFileName(entry.Name())
            if issueInfo.Number > 0 {
                // 读取文件内容获取 PR 号
                prNumber := m.readPRNumber(filepath.Join(mappingDir, entry.Name()))
                if prNumber > 0 {
                    mappings[issueInfo.DirName] = prNumber
                }
            }
        }
    }

    return mappings
}
```

### 3. 目录结构设计

#### 3.1 新的目录结构

```
basedir/
├── codeagent/
│   ├── .git/                      # 共享的 Git 仓库
│   │   ├── info/
│   │   │   └── exclude/           # 映射文件目录
│   │   │       ├── issue-123-1703123456789.txt
│   │   │       └── issue-124-1703123456790.txt
│   │   └── worktrees/             # Git worktree 信息
│   ├── issue-123-1703123456789/   # Issue 123 的 worktree
│   ├── issue-124-1703123456790/   # Issue 124 的 worktree
│   ├── session-91/                # PR 91 的 session 目录
│   └── session-92/                # PR 92 的 session 目录
└── other-repo/
    ├── .git/
    │   └── info/
    │       └── exclude/
    │           └── issue-125-1703123456791.txt
    ├── issue-125-1703123456791/   # Issue 125 的 worktree
    └── session-93/                # PR 93 的 session 目录
```

#### 3.2 命名规则

- **Issue 工作空间目录**：`issue-{number}-{timestamp}`
- **映射文件**：`issue-{number}-{timestamp}.txt`
- **Session 目录**：`session-{PR号}`
- **仓库目录**：从仓库 URL 提取的仓库名

### 4. 实现细节

#### 4.1 映射文件管理

```go
type MappingManager struct {
    mappingDir string
    mutex      sync.RWMutex
}

// CreateMapping 创建 Issue 到 PR 的映射
func (m *MappingManager) CreateMapping(issueDir string, prNumber int) error {
    mappingFile := filepath.Join(m.mappingDir, issueDir+".txt")

    m.mutex.Lock()
    defer m.mutex.Unlock()

    // 确保目录存在
    if err := os.MkdirAll(m.mappingDir, 0755); err != nil {
        return err
    }

    // 写入 PR 号
    return os.WriteFile(mappingFile, []byte(strconv.Itoa(prNumber)), 0644)
}

// GetPRNumber 根据 Issue 目录获取 PR 号
func (m *MappingManager) GetPRNumber(issueDir string) (int, error) {
    mappingFile := filepath.Join(m.mappingDir, issueDir+".txt")

    m.mutex.RLock()
    defer m.mutex.RUnlock()

    data, err := os.ReadFile(mappingFile)
    if err != nil {
        return 0, err
    }

    return strconv.Atoi(strings.TrimSpace(string(data)))
}
```

#### 4.2 工作空间管理器更新

```go
type Manager struct {
    baseDir      string
    workspaces   map[int]*models.Workspace  // PR号 -> 工作空间
    issueMapping map[string]int             // Issue目录 -> PR号
    repoManagers map[string]*RepoManager
    mutex        sync.RWMutex
    config       *config.Config
}

// CreateWorkspaceFromIssue 从 Issue 创建工作空间
func (m *Manager) CreateWorkspaceFromIssue(issue *github.Issue) *models.Workspace {
    // 1. 生成 Issue 工作空间目录名
    timestamp := time.Now().Unix()
    issueDir := fmt.Sprintf("issue-%d-%d", issue.GetNumber(), timestamp)

    // 2. 创建 worktree
    repoManager := m.getOrCreateRepoManager(repoURL)
    worktree, err := repoManager.CreateWorktree(issueDir, branchName, true)
    if err != nil {
        return nil
    }

    // 3. 创建工作空间对象
    ws := &models.Workspace{
        ID:          issueDir,
        Path:        worktree.Path,
        SessionPath: filepath.Join(repoManager.GetRepoPath(), fmt.Sprintf("session-%d", issue.GetNumber())),
        Repository:  repoURL,
        Branch:      worktree.Branch,
        CreatedAt:   worktree.CreatedAt,
        Issue:       issue,
    }

    // 4. 注册到 Issue 映射
    m.mutex.Lock()
    m.issueMapping[issueDir] = 0 // 临时标记，等待 PR 创建后更新
    m.mutex.Unlock()

    return ws
}

// UpdateIssueToPRMapping 更新 Issue 到 PR 的映射
func (m *Manager) UpdateIssueToPRMapping(issueDir string, prNumber int) error {
    // 1. 更新内存映射
    m.mutex.Lock()
    m.issueMapping[issueDir] = prNumber
    m.mutex.Unlock()

    // 2. 创建映射文件
    repoName := m.extractRepoNameFromIssueDir(issueDir)
    repoManager := m.repoManagers[repoName]
    if repoManager == nil {
        return fmt.Errorf("repo manager not found for %s", repoName)
    }

    mappingManager := repoManager.GetMappingManager()
    return mappingManager.CreateMapping(issueDir, prNumber)
}

// GetWorkspaceByPR 根据 PR 号获取工作空间
func (m *Manager) GetWorkspaceByPR(prNumber int) *models.Workspace {
    m.mutex.RLock()
    defer m.mutex.RUnlock()

    return m.workspaces[prNumber]
}

// GetWorkspaceByIssue 根据 Issue 号获取工作空间
func (m *Manager) GetWorkspaceByIssue(issueNumber int) *models.Workspace {
    m.mutex.RLock()
    defer m.mutex.RUnlock()

    // 查找对应的 Issue 目录
    for issueDir, prNumber := range m.issueMapping {
        if strings.HasPrefix(issueDir, fmt.Sprintf("issue-%d-", issueNumber)) {
            return m.workspaces[prNumber]
        }
    }

    return nil
}
```

#### 4.3 Agent 流程更新

```go
// ProcessIssueComment 处理 Issue 评论事件
func (a *Agent) ProcessIssueComment(event *github.IssueCommentEvent) error {
    // 1. 创建 Issue 工作空间
    ws := a.workspace.CreateWorkspaceFromIssue(event.Issue)
    if ws == nil {
        return fmt.Errorf("failed to create workspace from issue")
    }

    // 2. 创建分支并推送
    if err := a.github.CreateBranch(ws); err != nil {
        return err
    }

    // 3. 创建初始 PR
    pr, err := a.github.CreatePullRequest(ws)
    if err != nil {
        return err
    }

    // 4. 更新映射关系
    issueDir := ws.ID // issue-{number}-{timestamp}
    if err := a.workspace.UpdateIssueToPRMapping(issueDir, pr.GetNumber()); err != nil {
        log.Errorf("Failed to update mapping: %v", err)
    }

    // 5. 注册工作空间到 PR 映射
    ws.PullRequest = pr
    a.workspace.RegisterWorkspace(ws, pr)

    // 6. 执行代码修改
    // ... 后续处理逻辑
}
```

### 5. 恢复机制

#### 5.1 启动时恢复

```go
func (m *Manager) recoverExistingWorkspacesWithMapping() {
    log.Infof("Starting to recover existing workspaces with mapping from %s", m.baseDir)

    entries, err := os.ReadDir(m.baseDir)
    if err != nil {
        log.Errorf("Failed to read base directory: %v", err)
        return
    }

    recoveredCount := 0
    for _, repoEntry := range entries {
        if !repoEntry.IsDir() {
            continue
        }

        repoPath := filepath.Join(m.baseDir, repoEntry.Name())

        // 检查是否有 .git 目录
        gitDir := filepath.Join(repoPath, ".git")
        if _, err := os.Stat(gitDir); os.IsNotExist(err) {
            continue
        }

        // 读取映射文件
        mappingDir := filepath.Join(gitDir, "info", "exclude")
        mappings := m.readMappingFiles(mappingDir)

        // 恢复每个工作空间
        for issueDir, prNumber := range mappings {
            if err := m.recoverWorkspace(repoPath, issueDir, prNumber); err != nil {
                log.Errorf("Failed to recover workspace %s: %v", issueDir, err)
                continue
            }
            recoveredCount++
        }
    }

    log.Infof("Mapping recovery completed. Recovered %d workspaces", recoveredCount)
}
```

#### 5.2 工作空间恢复

```go
func (m *Manager) recoverWorkspace(repoPath, issueDir string, prNumber int) error {
    // 1. 检查 worktree 是否存在
    worktreePath := filepath.Join(repoPath, issueDir)
    if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
        return fmt.Errorf("worktree not found: %s", worktreePath)
    }

    // 2. 获取远程仓库 URL
    remoteURL, err := m.getRemoteURL(repoPath)
    if err != nil {
        return err
    }

    // 3. 创建 session 目录
    sessionPath := filepath.Join(repoPath, fmt.Sprintf("session-%d", prNumber))
    if err := os.MkdirAll(sessionPath, 0755); err != nil {
        return err
    }

    // 4. 获取当前分支
    branch, err := m.getCurrentBranch(worktreePath)
    if err != nil {
        return err
    }

    // 5. 恢复工作空间对象
    ws := &models.Workspace{
        ID:          issueDir,
        Path:        worktreePath,
        SessionPath: sessionPath,
        Repository:  remoteURL,
        Branch:      branch,
        PullRequest: &github.PullRequest{Number: github.Int(prNumber)},
        CreatedAt:   time.Now(),
    }

    // 6. 注册到内存映射
    m.mutex.Lock()
    m.workspaces[prNumber] = ws
    m.issueMapping[issueDir] = prNumber
    m.mutex.Unlock()

    log.Infof("Recovered workspace: Issue=%s, PR=%d, Path=%s", issueDir, prNumber, worktreePath)
    return nil
}
```

### 6. 优势分析

#### 6.1 精确映射

- **持久化存储**：映射关系存储在 `.git/info/exclude` 中，不会丢失
- **精确对应**：每个 Issue 工作空间都有明确的 PR 对应关系
- **快速查找**：通过映射文件快速定位工作空间

#### 6.2 简化恢复

- **自动恢复**：系统启动时自动恢复所有工作空间
- **准确恢复**：基于映射文件，恢复准确率 100%
- **无需猜测**：不需要从目录名或分支名推断关系

#### 6.3 性能优化

- **继续使用 worktree**：保持 Git worktree 的性能优势
- **减少扫描**：只需要扫描映射文件，不需要扫描所有目录
- **快速定位**：O(1) 时间复杂度的映射查找

### 7. 迁移策略

#### 7.1 向后兼容

1. **保留原有恢复逻辑**：在恢复时同时检查新旧格式
2. **渐进式迁移**：新创建的工作空间使用新格式
3. **映射文件创建**：为现有的工作空间创建映射文件

#### 7.2 迁移工具

```go
func (m *Manager) migrateExistingWorkspaces() {
    // 1. 扫描现有的工作空间
    // 2. 为每个工作空间创建映射文件
    // 3. 更新内存映射
    // 4. 清理旧的恢复逻辑
}
```

### 8. 注意事项

#### 8.1 文件权限

- 确保 `.git/info/exclude` 目录有写权限
- 映射文件使用 644 权限

#### 8.2 并发安全

- 映射文件操作需要加锁
- 避免同时创建相同 Issue 的映射

#### 8.3 错误处理

- 映射文件损坏时的恢复机制
- 网络问题导致 PR 创建失败的处理

## 总结

新的设计通过 `.git/info/exclude` 映射文件实现了：

1. **精确映射**：Issue 和 PR 的一一对应关系
2. **持久化存储**：映射关系不会因系统重启而丢失
3. **快速恢复**：系统启动时能准确恢复所有工作空间
4. **性能优化**：继续使用 Git worktree 技术
5. **向后兼容**：支持渐进式迁移

这个设计从根本上解决了工作空间映射的问题，确保每个 Issue 和 PR 都有明确的工作空间对应关系。
