package modes

import (
	"context"
	"fmt"
	"sort"

	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/xlog"
)

// Manager 模式管理器
type Manager struct {
	handlers []ModeHandler
}

// NewManager 创建新的模式管理器
func NewManager() *Manager {
	return &Manager{
		handlers: make([]ModeHandler, 0),
	}
}

// RegisterHandler 注册模式处理器
func (m *Manager) RegisterHandler(handler ModeHandler) {
	m.handlers = append(m.handlers, handler)
	
	// 按优先级排序（高优先级在前）
	sort.Slice(m.handlers, func(i, j int) bool {
		return m.handlers[i].GetPriority() > m.handlers[j].GetPriority()
	})
}

// GetHandlerCount 获取处理器数量
func (m *Manager) GetHandlerCount() int {
	return len(m.handlers)
}

// SelectHandler 选择合适的处理器处理事件
func (m *Manager) SelectHandler(ctx context.Context, event models.GitHubContext) (ModeHandler, error) {
	xl := xlog.NewWith(ctx)
	
	// 按优先级顺序查找能处理的处理器
	for _, handler := range m.handlers {
		if handler.CanHandle(ctx, event) {
			xl.Infof("Selected handler: %s for event type: %s", 
				handler.GetHandlerName(), event.GetEventType())
			return handler, nil
		}
	}
	
	return nil, fmt.Errorf("no handler found for event type: %s", event.GetEventType())
}

// ProcessEvent 处理事件（选择合适的处理器并执行）
func (m *Manager) ProcessEvent(ctx context.Context, event models.GitHubContext) error {
	handler, err := m.SelectHandler(ctx, event)
	if err != nil {
		return err
	}
	
	return handler.Execute(ctx, event)
}

// GetHandlers 获取所有注册的处理器
func (m *Manager) GetHandlers() []ModeHandler {
	return m.handlers
}

// GetHandlerByMode 根据模式类型获取处理器
func (m *Manager) GetHandlerByMode(mode ExecutionMode) ModeHandler {
	for _, handler := range m.handlers {
		if handler.GetMode() == mode {
			return handler
		}
	}
	return nil
}