# CodeAgent v0.4 - 基于 Git Worktree 的工作空间管理设计

## 概述

CodeAgent v0.4 采用极简的工作空间管理方案，完全基于 Git worktree 机制。所有工作空间仅通过目录名唯一标识，无需任何额外的映射文件或持久化元数据，系统可在重启后自动恢复所有状态。

## 设计要点

1. **目录唯一性**：每个工作空间目录名唯一，包含关键信息（如 repo、issue/pr 编号、时间戳）。
2. **无映射/无额外元数据**：所有状态仅靠目录名表达，无需任何映射文件或数据库。
3. **极简恢复**：系统启动时只需扫描目录名即可恢复全部工作空间。
4. **目录隔离**：所有 worktree 目录与仓库目录同级，仓库内部结构始终整洁。

## 工作空间生命周期

### 1. Issue 工作空间

- 创建时目录名格式：`{repo}-issue-{issue-number}-{timestamp}`
- 例如：`codeagent-issue-123-1703123456789`

### 2. PR 工作空间

- PR 创建后，目录名格式：`{repo}-pr-{pr-number}-{timestamp}`
- 例如：`codeagent-pr-91-1703123456789`

Session 目录统一为：`{repo}-session-{pr-number}-{timestamp}`

### 3. 目录结构示例

```
basedir/
├── qbox/
│   ├── codeagent/                  # 仓库目录
│   │   ├── .git/                   # 共享的 Git 仓库
│   ├── codeagent-issue-124-1703123456790/   # Issue 工作空间
│   ├── codeagent-pr-91-1703123456789/       # PR 工作空间
│   ├── codeagent-session-issue-124-1703123456790/  # Issue session 目录
│   ├── codeagent-session-pr-91-1703123456789/      # PR session 目录
```

## 恢复与清理机制

- **恢复**：系统启动时递归扫描所有组织/仓库目录下的 worktree 目录（`{repo}-issue-*`、`{repo}-pr-*`）和 session 目录（`{repo}-session-issue-*`、`{repo}-session-pr-*`），通过目录名解析出 issue/pr 编号、时间戳，自动恢复所有工作空间。
- **清理**：只需根据目录名和业务逻辑判断是否过期，直接删除对应 worktree 目录和 session 目录。

## 主要优势

- **极简**：无任何多余元数据，目录即状态。
- **健壮**：即使异常重启，目录结构不变，所有工作空间都能恢复。
- **高性能**：充分利用 Git worktree 的原生能力，无需重复 clone。
- **易维护**：目录结构清晰，便于人工排查和自动化脚本处理。

## 总结

CodeAgent v0.4 的工作空间管理方案，彻底抛弃了映射、move、数据库等复杂机制，完全依赖 Git worktree 和目录名唯一性，实现了极致简洁、健壮、可恢复的多工作空间管理。
