# Webhook 签名验证测试指南

本文档提供了如何测试 GitHub Webhook 签名验证功能的详细指南。

## 测试环境准备

### 1. 启动测试服务器

```bash
# 设置测试环境变量
export GITHUB_TOKEN="your-test-token"
export WEBHOOK_SECRET="test-secret-123"
export PORT=8888

# 启动服务器
go run ./cmd/server
```

### 2. 生成测试签名

您可以使用以下 Go 代码生成有效的测试签名：

```go
package main

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
)

func main() {
    secret := "test-secret-123"
    payload := `{"action":"opened","number":1,"issue":{"title":"Test Issue"}}`
    
    // 生成 SHA-256 签名
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(payload))
    signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    
    fmt.Printf("Payload: %s\n", payload)
    fmt.Printf("Signature: %s\n", signature)
}
```

## 测试用例

### 1. 有效签名测试

```bash
# 生成有效签名的测试数据
PAYLOAD='{"action":"opened","number":1,"issue":{"title":"Test Issue"}}'
SECRET="test-secret-123"

# 使用 openssl 生成签名
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" -hex | cut -d' ' -f2)

# 发送有效签名的请求
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  -d "$PAYLOAD"
```

预期结果：返回 400 Bad Request （因为缺少 issue comment 的必要字段，但签名验证通过）

### 2. 无效签名测试

```bash
# 发送无效签名的请求
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -H "X-Hub-Signature-256: sha256=invalid-signature" \
  -d '{"action":"opened","number":1}'
```

预期结果：返回 401 Unauthorized，响应体为 "invalid signature"

### 3. 缺少签名测试

```bash
# 发送不包含签名的请求
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d '{"action":"opened","number":1}'
```

预期结果：返回 401 Unauthorized，响应体为 "missing signature"

### 4. 无密钥配置测试

```bash
# 停止服务器，重新启动时不设置 WEBHOOK_SECRET
unset WEBHOOK_SECRET
go run ./cmd/server &

# 发送无签名的请求
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d '{"action":"opened","number":1}'
```

预期结果：跳过签名验证，返回 400 Bad Request （因为缺少 issue comment 的必要字段）

## 自动化测试

项目包含了自动化测试，可以通过以下命令运行：

```bash
# 运行签名验证相关的所有测试
go test ./pkg/signature ./internal/webhook -v

# 运行特定的测试
go test ./pkg/signature -run TestValidateGitHubSignature -v
go test ./internal/webhook -run TestHandleWebhook_SignatureValidation -v
```

## 生产环境验证

在生产环境中，建议按以下步骤验证：

1. **配置强密码**: 使用至少 32 字符的随机密码
2. **测试 GitHub 集成**: 在 GitHub 仓库中配置 Webhook，确保 secret 一致
3. **监控日志**: 检查服务器日志，确认签名验证正常工作
4. **安全扫描**: 定期检查是否有未经授权的请求被拒绝

## 故障排除

### 常见错误

1. **"invalid signature"**: 检查 GitHub Webhook 配置中的 secret 是否与服务器配置一致
2. **"missing signature"**: 检查 GitHub Webhook 是否正确配置了 secret
3. **"invalid signature format"**: 确认 GitHub 发送的是 `sha256=...` 格式的签名

### 调试建议

1. 启用详细日志：`export LOG_LEVEL=debug`
2. 检查请求头：确认 `X-Hub-Signature-256` 头存在
3. 验证负载：确认请求体与签名计算使用的数据一致

## 安全注意事项

- 永远不要在日志中输出 webhook secret
- 使用 HTTPS 保护 webhook 端点
- 定期轮换 webhook secret
- 监控异常的签名验证失败，可能表明有攻击行为