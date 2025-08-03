# Claude 变体支持

CodeAgent 现在支持多种 Claude 部署方式，包括原生 Claude 和 Kimi 平台部署的 Claude。

## 配置方式

### 1. 环境变量配置

```bash
# 默认 Claude 配置
export CLAUDE_API_KEY="your-default-claude-api-key"

# Kimi 变体配置
export CLAUDE_KIMI_API_KEY="your-kimi-claude-api-key"
export CLAUDE_KIMI_BASE_URL="https://kimi.moonshot.cn/api"
export CLAUDE_KIMI_IMAGE="anthropic/claude-code:latest"
```

### 2. 配置文件方式

```yaml
claude:
  api_key: "your-default-claude-api-key"
  container_image: "anthropic/claude-code:latest"
  timeout: 30m
  variants:
    kimi:
      api_key: "your-kimi-claude-api-key"
      base_url: "https://kimi.moonshot.cn/api"
      container_image: "anthropic/claude-code:latest"
      timeout: 30m
      description: "Claude deployed on Kimi platform"
```

## 使用方法

### 1. 全局默认设置

```bash
# 使用原生 Claude
export CODE_PROVIDER="claude"

# 使用 Kimi 变体
export CODE_PROVIDER="claude:kimi"
```

### 2. 工作空间级别指定

在 Issue 评论中指定 AI 模型：

```
@codeagent claude:kimi 请帮我实现这个功能
```

### 3. 分支命名约定

工作空间会根据分支名自动选择 AI 模型：

- `claude-xxx` -> 原生 Claude
- `claude-kimi-xxx` -> Kimi 变体

## 变体类型

### 1. Native (原生)

- **标识符**: `claude` 或 `claude:native`
- **描述**: 使用官方 Claude API
- **配置**: 使用默认的 `CLAUDE_API_KEY`

### 2. Kimi

- **标识符**: `claude:kimi`
- **描述**: 使用 Kimi 平台部署的 Claude
- **配置**: 使用 `CLAUDE_KIMI_API_KEY` 和 `CLAUDE_KIMI_BASE_URL`

## 容器命名规则

为了区分不同的变体，容器命名规则已更新：

- 原生 Claude: `claude-native-{org}-{repo}-{pr}`
- Kimi 变体: `claude-kimi-{org}-{repo}-{pr}`

## 环境变量

### 必需环境变量

| 变量名                 | 描述                | 示例                           |
| ---------------------- | ------------------- | ------------------------------ |
| `CLAUDE_API_KEY`       | 默认 Claude API Key | `sk-ant-...`                   |
| `CLAUDE_KIMI_API_KEY`  | Kimi Claude API Key | `your-kimi-key`                |
| `CLAUDE_KIMI_BASE_URL` | Kimi API 基础 URL   | `https://kimi.moonshot.cn/api` |

### 可选环境变量

| 变量名              | 描述              | 默认值                         |
| ------------------- | ----------------- | ------------------------------ |
| `CLAUDE_KIMI_IMAGE` | Kimi 变体容器镜像 | `anthropic/claude-code:latest` |

## 最佳实践

1. **配置优先级**: 环境变量 > 配置文件 > 默认值
2. **安全性**: 敏感信息（如 API Key）建议使用环境变量
3. **容器隔离**: 不同变体使用独立的容器，避免冲突
4. **监控**: 可以通过容器名称区分不同变体的使用情况

## 故障排除

### 1. 变体未找到错误

```
Claude variant 'kimi' not found in configuration
```

**解决方案**: 确保在配置文件中定义了 `kimi` 变体，或设置了相应的环境变量。

### 2. 容器启动失败

检查 Docker 日志：

```bash
docker logs claude-kimi-{org}-{repo}-{pr}
```

### 3. API 连接失败

检查网络连接和 API Key 配置：

```bash
curl -H "Authorization: Bearer $CLAUDE_KIMI_API_KEY" "$CLAUDE_KIMI_BASE_URL/v1/models"
```
