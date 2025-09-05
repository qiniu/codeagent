package modes

import (
	"context"
	"testing"

	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockHandler 用于测试的Mock处理器
type MockHandler struct {
	*BaseHandler
	canHandleFunc func(ctx context.Context, event models.GitHubContext) bool
	executeFunc   func(ctx context.Context, event models.GitHubContext) error
}

func NewMockHandler(mode ExecutionMode, priority int, canHandle func(ctx context.Context, event models.GitHubContext) bool) *MockHandler {
	return &MockHandler{
		BaseHandler:   NewBaseHandler(mode, priority, "Mock handler for testing"),
		canHandleFunc: canHandle,
		executeFunc: func(ctx context.Context, event models.GitHubContext) error {
			return nil
		},
	}
}

func (mh *MockHandler) CanHandle(ctx context.Context, event models.GitHubContext) bool {
	if mh.canHandleFunc != nil {
		return mh.canHandleFunc(ctx, event)
	}
	return false
}

func (mh *MockHandler) Execute(ctx context.Context, event models.GitHubContext) error {
	if mh.executeFunc != nil {
		return mh.executeFunc(ctx, event)
	}
	return nil
}

func TestModeManager_RegisterHandler(t *testing.T) {
	manager := NewModeManager()

	// 创建不同优先级的处理器
	handler1 := NewMockHandler(TagMode, 20, nil)
	handler2 := NewMockHandler(AgentMode, 10, nil)
	handler3 := NewMockHandler(ReviewMode, 30, nil)

	// 注册处理器
	manager.RegisterHandler(handler1)
	manager.RegisterHandler(handler2)
	manager.RegisterHandler(handler3)

	// 验证处理器按优先级排序
	handlers := manager.GetRegisteredHandlers()
	require.Len(t, handlers, 3)

	// 应该按优先级排序：10, 20, 30
	assert.Equal(t, 10, handlers[0].GetPriority())
	assert.Equal(t, 20, handlers[1].GetPriority())
	assert.Equal(t, 30, handlers[2].GetPriority())
}

func TestModeManager_FindHandler(t *testing.T) {
	manager := NewModeManager()
	ctx := context.Background()

	// 创建测试事件
	event := &models.IssueCommentContext{
		BaseContext: models.BaseContext{
			Type: models.EventIssueComment,
		},
		Comment: &github.IssueComment{
			Body: github.String("/code implement this feature"),
		},
	}

	// 创建处理器
	tagHandler := NewMockHandler(TagMode, 10, func(ctx context.Context, event models.GitHubContext) bool {
		cmdInfo, hasCmd := models.HasCommandWithConfig(event, nil)
		return hasCmd && cmdInfo != nil
	})

	agentHandler := NewMockHandler(AgentMode, 20, func(ctx context.Context, event models.GitHubContext) bool {
		return false // 不处理这个事件
	})

	// 注册处理器
	manager.RegisterHandler(tagHandler)
	manager.RegisterHandler(agentHandler)

	// 查找处理器
	handler, err := manager.FindHandler(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, TagMode, handler.GetMode())
}

func TestModeManager_ModeEnableDisable(t *testing.T) {
	manager := NewModeManager()

	// 测试默认状态
	assert.True(t, manager.IsEnabled(TagMode))
	assert.False(t, manager.IsEnabled(AgentMode))
	assert.False(t, manager.IsEnabled(ReviewMode))

	// 启用Agent模式
	manager.EnableMode(AgentMode)
	assert.True(t, manager.IsEnabled(AgentMode))

	// 禁用Tag模式
	manager.DisableMode(TagMode)
	assert.False(t, manager.IsEnabled(TagMode))

	// 获取启用的模式
	enabledModes := manager.GetEnabledModes()
	assert.Contains(t, enabledModes, AgentMode)
	assert.NotContains(t, enabledModes, TagMode)
}

func TestModeManager_NoHandlerFound(t *testing.T) {
	manager := NewModeManager()
	ctx := context.Background()

	// 创建一个没有任何处理器能处理的事件
	event := &models.PushContext{
		BaseContext: models.BaseContext{
			Type: models.EventPush,
		},
	}

	// 创建一个不处理Push事件的处理器
	handler := NewMockHandler(TagMode, 10, func(ctx context.Context, event models.GitHubContext) bool {
		return event.GetEventType() == models.EventIssueComment
	})

	manager.RegisterHandler(handler)

	// 应该找不到处理器
	_, err := manager.FindHandler(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no suitable handler found")
}

func TestModeManager_DisabledModeSkipped(t *testing.T) {
	manager := NewModeManager()
	ctx := context.Background()

	// 创建测试事件
	event := &models.IssueCommentContext{
		BaseContext: models.BaseContext{
			Type: models.EventIssueComment,
		},
		Comment: &github.IssueComment{
			Body: github.String("/code implement this"),
		},
	}

	// 创建能处理该事件的处理器
	handler := NewMockHandler(TagMode, 10, func(ctx context.Context, event models.GitHubContext) bool {
		cmdInfo, hasCmd := models.HasCommandWithConfig(event, nil)
		return hasCmd && cmdInfo != nil
	})

	manager.RegisterHandler(handler)

	// 禁用Tag模式
	manager.DisableMode(TagMode)

	// 应该找不到处理器（因为Tag模式被禁用了）
	_, err := manager.FindHandler(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no suitable handler found")
}
