# CodeAgent 改进测试文档

## 改进内容

### 1. 真正的两次 Commit 策略

- **Initial Commit**: 在 `CreateBranch` 方法中创建空的初始提交
- **Final Commit**: 在 `CommitAndPush` 方法中直接提交代码变更（移除 AI 的 commit 步骤）

### 2. PR 描述的三部分结构

- **第一部分**: 改动总结（Issue 信息 + 主要变更 + 变更文件列表）
- **第二部分**: 详细执行过程（代码修改过程）
- **第三部分**: 错误信息（如果有的话）

## 新的处理流程

### 步骤 1: 创建 Issue

1. 在 GitHub 仓库中创建一个新的 Issue
2. 添加 `/code` 命令到 Issue 评论中

### 步骤 2: Initial Commit

- 创建包含 "Initial plan for Issue #X" 的空提交
- 创建分支和 PR

### 步骤 3: AI 分析

- AI 分析 Issue 内容并生成修改计划
- AI 执行代码修改

### 步骤 4: Final Commit

- 系统直接检测文件变更
- 使用标准的 conventional commits 格式提交
- 包含变更文件列表和 `Closes #X` 引用

### 步骤 5: 更新 PR 描述

- 构建结构化的 PR 描述
- 包含三个清晰的部分

## 预期结果

### Commit 历史

```
commit 1: "Initial plan for Issue #123: Add new feature"
commit 2: "feat: implement Issue #123 - Add new feature

- src/feature.js
- tests/feature.test.js

Closes #123"
```

### PR 描述示例

```markdown
## 改动总结

**Issue**: #123 - Add new feature

**主要变更**:

- 添加了新的功能模块
- 更新了相关测试
- 优化了性能

**变更文件**:
src/feature.js
tests/feature.test.js

---

## 详细执行过程

### 代码修改过程

[AI 的完整代码修改输出]

---
```

## 关键改进点

1. **简化流程**: 移除了 AI 的 commit 步骤，避免重复提交
2. **两次 Commit**: 真正的两次 commit 策略
   - Initial: 空提交，建立分支和 PR
   - Final: 包含实际代码变更的提交
3. **系统化 Commit**: 使用系统生成的标准化 commit 消息
4. **清晰结构**: PR 描述包含三个明确的部分

## 验证要点

1. **两次 Commit**: 确保只有两个 commit，初始空提交和最终代码提交
2. **无 AI Commit**: 验证没有 AI 执行的 commit 步骤
3. **PR 结构**: 验证 PR 描述包含三个清晰的部分
4. **文件列表**: 确认变更文件列表准确显示
5. **错误处理**: 如果有错误，确保在第三部分正确显示
6. **Commit 消息**: 验证使用标准的 conventional commits 格式
