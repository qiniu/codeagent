# CodeAgent v0.6 - 自定义命令、Subagents 支持

## 背景认知

通过调研，我们知道要想让 AI 写代码效果更好，应该:

- 优化指令，让 AI 做事更有针对性，比如一个阶段只敢一件事
- 使用 Extended Thinking 模式，主动让 AI 更充分的思考(claude)
- 使用 Subagents ，构建专业 Agent，让其做特定的事
- 基于实际场景，使用一些被证明行之有效的工作流
  - explore, plan,code , commit
  - Write tests, commit; code, iterate, commit
  - Write code, screenshot result, iterate
- 使用高级的模型，如 Opus 4.1

结合上述行为在 claude code 侧的实际设计(PS: [gemini 相关能力能力还在开发中](https://github.com/google-gemini/gemini-cli/issues/3132) )，v0.6 计划引入了**自定义命令系统**和**Subagents 支持**，以充分利用 claude code 的能力。

设计要点:

- 在 github 交互中用到的 slash 命令，就类比 claude code 的命令。其最终对应的 prompt 细节都会映射到目录文件，类似/code 命令，会映射到 .codeagent/commands/code.md

- 命令格式，采用 YAML frontmatter + markdown 格式, 整体语法跟 claude code 一致

- 支持在命令里引入 GITHUB 上下文，该信息会统一由 CodeAgents 自动注入

- 支持 Claude Code 的 subagents 设计，最终 agents 目录也会被挂载到合适的位置

- 支持各仓库自定义命令，如果自定义命令跟系统默认重名，优先选择自定义命令

有了上述能力，我们就可以并行的，更加充分的调试 Prompt，探索最佳 AI 工作流实践。

## 命令示例

/analyze

```markdown
---
allowed-tools: all
description: 需求深度分析
model: ...
---

## 当前 Issue 信息

- 仓库: $REPOSITORY
- Issue: #$ISSUE_NUMBER - $ISSUE_TITLE
- 提交者: $TRIGGER_USERNAME

## Issue 描述

$ISSUE_BODY

## 历史讨论

$ISSUE_COMMENTS

## 分析任务

请使用 Extended Thinking 模式对 Issue 进行深度分析
```

#### Agent 定义格式示例

requirements-analyst agent:

```markdown
---
name: requirements-analyst
description: "专业的需求分析专家"
model: ...
tools: ["read", "grep"]
---

# 需求分析专家

你是一个经验丰富的需求分析专家，专注于理解和分析软件需求...

## 专业技能

- 深度需求理解和澄清
- 技术可行性评估
- 实现风险识别

## 工作方法

使用结构化分析方法，提供清晰准确的需求总结和实现建议
```

对于 agent 的定义， claude code 要求:

| Field       | Required | Description                                                                                 |
| ----------- | -------- | ------------------------------------------------------------------------------------------- |
| name        | Yes      | Unique identifier using lowercase letters and hyphens                                       |
| description | Yes      | Natural language description of the subagent's purpose                                      |
| tools       | No       | Comma-separated list of specific tools. If omitted, inherits all tools from the main thread |

### 2. 目录结构

#### 2.1 配置文件

```yaml
# config.yaml
commands:
  global_path: "/opt/codeagent/.codeagent" # 保存agent默认的全局命令和Agent定义目录

# 其他配置...
```

#### 2.2 目录合并机制

CodeAgent 在运行时将两个 `.codeagent` 目录**物理合并**成一个临时目录，然后挂载到容器：

**合并流程**：

```bash
# 1. 创建临时合并目录
mkdir /tmp/codeagent-merged-${repo}-${timestamp}

# 2. 先拷贝全局配置
cp -r /opt/codeagent/.codeagent/* /tmp/codeagent-merged-${repo}/

# 3. 仓库配置覆盖全局配置（cp -rf 语义）
if [ -d "${workspace}/.codeagent" ]; then
    cp -rf ${workspace}/.codeagent/* /tmp/codeagent-merged-${repo}/
fi

# 4. 挂载到容器
docker run -v /tmp/codeagent-merged-${repo}/commands:/root/.claude/commands \
           -v /tmp/codeagent-merged-${repo}/agents:/root/.claude/agents \
           ...
```

**实际效果**：

- 全局有 `analyze.md`, `plan.md`, `code.md`
- 仓库有 `analyze.md`, `custom.md`
- 合并后：`analyze.md`(仓库版本), `plan.md`(全局版本), `code.md`(全局版本), `custom.md`(仓库版本)

#### 2.3 目录层级

```
# 全局配置目录（通过config.yaml指定）
/opt/codeagent/.codeagent/
├── commands/          # 命令定义文件 (.md)
│   ├── analyze.md
│   ├── plan.md
│   ├── code.md
│   ├── plan-and-code.md
│   └── continue.md
└── agents/           # Agent 定义文件 (.md)
    ├── requirements-analyst.md
    ├── solution-architect.md
    ├── implementation-expert.md
    └── code-reviewer.md

# 仓库级配置目录（仓库根目录下，可选）
{workspace}/.codeagent/
├── commands/         # 项目特定命令（与全局合并，仓库级覆盖全局级）
└── agents/          # 项目特定 agents（与全局合并，仓库级覆盖全局级）

# 运行时挂载
Docker 模式：
  .codeagent/commands → ~/.claude/commands
  .codeagent/agents   → ~/.claude/agents

```

### 3. 核心实现：GitHub 上下文注入器

**设计理念**：提供智能的模板引擎，支持简单变量替换和 Go Template 语法，适配不同事件类型的精确上下文。

#### 3.1 变量命名规范

采用 `GITHUB_` 前缀，类似如下：

```go
// 核心变量
$GITHUB_REPOSITORY      // owner/repo
$GITHUB_EVENT_TYPE      // issues|pull_request|pull_request_review_comment
$GITHUB_TRIGGER_USER    // 触发用户

// Issue变量
$GITHUB_ISSUE_NUMBER    // 123
$GITHUB_ISSUE_TITLE     // Issue标题
$GITHUB_ISSUE_BODY      // Issue内容

// PR变量
$GITHUB_PR_NUMBER       // 456
$GITHUB_PR_TITLE        // PR标题
$GITHUB_BRANCH_NAME     // feature-branch
$GITHUB_BASE_BRANCH     // main

// Review Comment变量（高精度行级上下文）
$GITHUB_REVIEW_FILE_PATH      // src/main.go
$GITHUB_REVIEW_LINE_RANGE     // "行号：123" 或 "行号范围：45-67"
$GITHUB_REVIEW_COMMENT_BODY   // 评论内容
$GITHUB_REVIEW_DIFF_HUNK      // 差异片段
$GITHUB_REVIEW_FILE_CONTENT   // 目标行周围代码上下文
```

#### 3.4 模板语法示例

```markdown
# 智能事件分析

## 基本信息

- 仓库: {{.GITHUB_REPOSITORY}}
- 事件: {{.GITHUB_EVENT_TYPE}}

## 上下文

{{if .GITHUB_IS_PR}}

### PR 信息

- PR #{{.GITHUB_PR_NUMBER}}: {{.GITHUB_PR_TITLE}}
  {{if gt (len .GITHUB_CHANGED_FILES) 10}}
- 变更文件: {{len .GITHUB_CHANGED_FILES}}个（显示前 10 个）
  {{else}}
- 变更文件: {{len .GITHUB_CHANGED_FILES}}个
  {{end}}
  {{else if .GITHUB_IS_ISSUE}}

### Issue 信息

- Issue #{{.GITHUB_ISSUE_NUMBER}}: {{.GITHUB_ISSUE_TITLE}}
- 标签: {{join .GITHUB_ISSUE_LABELS ", "}}
  {{end}}
```

GITHUb 上下文会基于后续需求，不断补充和调整。

## 内置命令设计

CodeAgent v0.6 预计会提供以下核心命令，支持从需求分析到代码实现的完整工作流：

### Issue 阶段命令

#### `/analyze` - 深度需求分析

- **用途**: 深入理解 Issue 需求，识别技术挑战和实现风险
- **Subagent**: requirements-analyst
- **GitHub 变量**: Issue 相关上下文（标题、内容、评论、标签等）
- **输出**: 结构化的需求分析报告，包含技术难点和实施建议

#### `/plan` - 技术方案设计

- **用途**: 制定详细的技术实现方案和分步执行计划
- **Subagent**: solution-architect
- **GitHub 变量**: Issue 上下文 + 代码库结构信息
- **输出**: 完整的技术方案文档，包含架构设计和实施清单

#### `/code` - 直接代码实现

- **用途**: 完整实现 Issue 需求，包含编码、测试、文档
- **Subagent**: implementation-expert
- **GitHub 变量**: Issue 上下文
- **输出**: 可运行的代码实现 + 测试 + 文档

### PR 阶段命令

#### `/continue` - PR 协作开发

- **用途**: 在 PR 中处理反馈、迭代改进、解决问题
- **Subagent**: pr-collaborator
- **GitHub 变量**: PR 完整上下文（变更文件、评论历史、Review 反馈等）
- **输出**: 改进后的代码 + 反馈回应 + 变更说明

## 参考资料

- https://www.anthropic.com/engineering/claude-code-best-practices
- https://docs.anthropic.com/en/docs/claude-code/sub-agents
- https://docs.anthropic.com/en/docs/claude-code/slash-commands
- https://github.com/cexll/myclaude
- https://github.com/anthropics/claude-code-security-review
- https://github.com/anthropics/claude-code-action
- https://github.com/Pimzino/claude-code-spec-workflow
