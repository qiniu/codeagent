package modes

import (
	"context"
	"fmt"

	"github.com/qiniu/codeagent/pkg/models"
)

// ExecutionMode 执行模式类型
type ExecutionMode string

const (
	// TagMode @codeagent 提及模式
	TagMode ExecutionMode = "tag"

	// AgentMode 自动化模式
	AgentMode ExecutionMode = "agent"

	// ReviewMode 自动审查模式
	ReviewMode ExecutionMode = "review"

	// CustomCommandMode 自定义命令模式
	CustomCommandMode ExecutionMode = "custom-commands"
)

// ModeHandler 模式处理器接口
type ModeHandler interface {
	// CanHandle 检查是否能处理给定的事件上下文
	CanHandle(ctx context.Context, event models.GitHubContext) bool

	// Execute 执行模式逻辑
	Execute(ctx context.Context, event models.GitHubContext) error

	// GetPriority 获取处理器优先级（数字越小优先级越高）
	GetPriority() int

	// GetMode 获取模式类型
	GetMode() ExecutionMode

	// GetDescription 获取模式描述
	GetDescription() string

	// GetHandlerName 获取处理器名称
	GetHandlerName() string
}

// BaseHandler 基础处理器，提供通用功能
type BaseHandler struct {
	mode        ExecutionMode
	priority    int
	description string
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(mode ExecutionMode, priority int, description string) *BaseHandler {
	return &BaseHandler{
		mode:        mode,
		priority:    priority,
		description: description,
	}
}

func (bh *BaseHandler) GetPriority() int {
	return bh.priority
}

func (bh *BaseHandler) GetMode() ExecutionMode {
	return bh.mode
}

func (bh *BaseHandler) GetDescription() string {
	return bh.description
}

func (bh *BaseHandler) GetHandlerName() string {
	return string(bh.mode) + "_handler"
}

// ModeManager 模式管理器
type ModeManager struct {
	handlers []ModeHandler
	enabled  map[ExecutionMode]bool
}

// NewModeManager 创建新的模式管理器
func NewModeManager() *ModeManager {
	return &ModeManager{
		handlers: make([]ModeHandler, 0),
		enabled: map[ExecutionMode]bool{
			TagMode:           true,  // 默认启用Tag模式
			AgentMode:         false, // 默认禁用Agent模式
			ReviewMode:        false, // 默认禁用Review模式
			CustomCommandMode: false, // 默认禁用自定义命令模式，需要手动启用
		},
	}
}

// RegisterHandler 注册模式处理器
func (mm *ModeManager) RegisterHandler(handler ModeHandler) {
	mm.handlers = append(mm.handlers, handler)

	// 按优先级排序（优先级数字越小越优先）
	for i := len(mm.handlers) - 1; i > 0; i-- {
		if mm.handlers[i].GetPriority() < mm.handlers[i-1].GetPriority() {
			mm.handlers[i], mm.handlers[i-1] = mm.handlers[i-1], mm.handlers[i]
		} else {
			break
		}
	}
}

// EnableMode 启用指定模式
func (mm *ModeManager) EnableMode(mode ExecutionMode) {
	mm.enabled[mode] = true
}

// DisableMode 禁用指定模式
func (mm *ModeManager) DisableMode(mode ExecutionMode) {
	mm.enabled[mode] = false
}

// IsEnabled 检查模式是否启用
func (mm *ModeManager) IsEnabled(mode ExecutionMode) bool {
	enabled, exists := mm.enabled[mode]
	return exists && enabled
}

// GetHandlerCount 获取注册的处理器数量
func (mm *ModeManager) GetHandlerCount() int {
	return len(mm.handlers)
}

// SelectHandler 选择合适的处理器（FindHandler的别名）
func (mm *ModeManager) SelectHandler(ctx context.Context, event models.GitHubContext) (ModeHandler, error) {
	return mm.FindHandler(ctx, event)
}

// FindHandler 根据事件上下文找到合适的处理器
func (mm *ModeManager) FindHandler(ctx context.Context, event models.GitHubContext) (ModeHandler, error) {
	for _, handler := range mm.handlers {
		// 检查模式是否启用
		if !mm.IsEnabled(handler.GetMode()) {
			continue
		}

		// 检查处理器是否能处理该事件
		if handler.CanHandle(ctx, event) {
			return handler, nil
		}
	}

	return nil, fmt.Errorf("no suitable handler found for event type: %s", event.GetEventType())
}

// Execute 执行事件处理
func (mm *ModeManager) Execute(ctx context.Context, event models.GitHubContext) error {
	handler, err := mm.FindHandler(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to find handler: %w", err)
	}

	return handler.Execute(ctx, event)
}

// GetRegisteredHandlers 获取所有注册的处理器
func (mm *ModeManager) GetRegisteredHandlers() []ModeHandler {
	return mm.handlers
}

// GetEnabledModes 获取所有启用的模式
func (mm *ModeManager) GetEnabledModes() []ExecutionMode {
	var enabled []ExecutionMode
	for mode, isEnabled := range mm.enabled {
		if isEnabled {
			enabled = append(enabled, mode)
		}
	}
	return enabled
}
