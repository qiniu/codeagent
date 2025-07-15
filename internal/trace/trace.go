package trace

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/qiniu/x/xlog"
)

// TraceID 表示追踪 ID
type TraceID string

// 为不同的事件类型定义追踪前缀
const (
	TracePrefix        = "codeagent"
	IssueCommentPrefix = "issue_comment"
	PRCommentPrefix    = "pr_comment"
	PRReviewPrefix     = "pr_review"
	PullRequestPrefix  = "pull_request"
	PushPrefix         = "push"
)

// generateTraceID 生成唯一的追踪 ID
func generateTraceID() TraceID {
	// 生成16字节的随机数据
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// 如果随机数生成失败，使用时间戳作为备用
		timestamp := time.Now().UnixNano()
		return TraceID(fmt.Sprintf("%s_%d", TracePrefix, timestamp))
	}
	
	// 转换为十六进制字符串
	return TraceID(fmt.Sprintf("%s_%x", TracePrefix, bytes))
}

// NewTraceID 创建新的追踪 ID
func NewTraceID(eventType string) TraceID {
	baseID := generateTraceID()
	return TraceID(fmt.Sprintf("%s_%s", eventType, baseID))
}

// 使用 context key 来存储追踪日志器
type contextKey string

const traceLoggerKey contextKey = "trace_logger"

// NewContext 创建带有追踪 ID 的上下文
func NewContext(ctx context.Context, traceID TraceID) context.Context {
	logger := xlog.New(string(traceID))
	return context.WithValue(ctx, traceLoggerKey, logger)
}

// FromContext 从上下文中获取追踪日志器
func FromContext(ctx context.Context) *xlog.Logger {
	if logger, ok := ctx.Value(traceLoggerKey).(*xlog.Logger); ok {
		return logger
	}
	return nil
}

// WithTraceID 为已有的上下文添加追踪 ID
func WithTraceID(ctx context.Context, traceID TraceID) context.Context {
	logger := FromContext(ctx)
	if logger == nil {
		logger = xlog.New(string(traceID))
	}
	return context.WithValue(ctx, traceLoggerKey, logger)
}

// GetTraceID 从上下文中获取追踪 ID
func GetTraceID(ctx context.Context) TraceID {
	logger := FromContext(ctx)
	if logger == nil {
		return ""
	}
	// 从 logger 中提取追踪 ID
	return TraceID(logger.ReqId)
}

// Info 记录信息级别的追踪日志
func Info(ctx context.Context, format string, args ...interface{}) {
	logger := FromContext(ctx)
	if logger != nil {
		logger.Infof(format, args...)
	}
}

// Error 记录错误级别的追踪日志
func Error(ctx context.Context, format string, args ...interface{}) {
	logger := FromContext(ctx)
	if logger != nil {
		logger.Errorf(format, args...)
	}
}

// Warn 记录警告级别的追踪日志
func Warn(ctx context.Context, format string, args ...interface{}) {
	logger := FromContext(ctx)
	if logger != nil {
		logger.Warnf(format, args...)
	}
}

// Debug 记录调试级别的追踪日志
func Debug(ctx context.Context, format string, args ...interface{}) {
	logger := FromContext(ctx)
	if logger != nil {
		logger.Debugf(format, args...)
	}
}