package mcp

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/qiniu/codeagent/pkg/models"
)

// toolValidator 工具验证器实现
type toolValidator struct{}

// NewToolValidator 创建工具验证器
func NewToolValidator() ToolValidator {
	return &toolValidator{}
}

// ValidateCall 验证工具调用
func (v *toolValidator) ValidateCall(call *models.ToolCall, tool *models.Tool) error {
	if call == nil {
		return fmt.Errorf("tool call is nil")
	}

	if tool == nil {
		return fmt.Errorf("tool definition is nil")
	}

	// 验证参数schema
	if tool.InputSchema != nil {
		if err := v.ValidateArguments(call.Function.Arguments, tool.InputSchema); err != nil {
			return fmt.Errorf("argument validation failed: %w", err)
		}
	}

	return nil
}

// ValidatePermissions 验证权限
func (v *toolValidator) ValidatePermissions(call *models.ToolCall, mcpCtx *models.MCPContext) error {
	if mcpCtx == nil {
		return nil // 无上下文时跳过权限检查
	}

	// 检查权限约束
	for _, constraint := range mcpCtx.Constraints {
		if v.violatesConstraint(call, constraint) {
			return fmt.Errorf("tool call violates constraint: %s", constraint)
		}
	}

	// 检查必需权限
	requiredPermission := v.getRequiredPermission(call.Function.Name)
	if requiredPermission != "" && !slices.Contains(mcpCtx.Permissions, requiredPermission) {
		return fmt.Errorf("insufficient permissions: requires %s", requiredPermission)
	}

	return nil
}

// ValidateArguments 验证参数
func (v *toolValidator) ValidateArguments(args map[string]interface{}, schema *models.JSONSchema) error {
	if schema == nil {
		return nil
	}

	// 验证必需字段
	for _, required := range schema.Required {
		if _, exists := args[required]; !exists {
			return fmt.Errorf("missing required argument: %s", required)
		}
	}

	// 验证字段类型和值
	for key, value := range args {
		if schema.Properties != nil {
			if fieldSchema, exists := schema.Properties[key]; exists {
				if err := v.validateValue(value, fieldSchema, key); err != nil {
					return err
				}
			} else if !schema.AdditionalProperties {
				return fmt.Errorf("unexpected argument: %s", key)
			}
		}
	}

	return nil
}

// validateValue 验证单个值
func (v *toolValidator) validateValue(value interface{}, schema *models.JSONSchema, fieldName string) error {
	if value == nil {
		return nil
	}

	// 类型检查
	switch schema.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("argument %s must be a string, got %T", fieldName, value)
		}

		// 枚举检查
		if len(schema.Enum) > 0 {
			if !slices.Contains(schema.Enum, value) {
				return fmt.Errorf("argument %s must be one of %v, got %v", fieldName, schema.Enum, value)
			}
		}

	case "number":
		switch value.(type) {
		case float64, float32, int, int32, int64:
			// 数字类型正确
		default:
			return fmt.Errorf("argument %s must be a number, got %T", fieldName, value)
		}

	case "integer":
		switch value.(type) {
		case int, int32, int64:
			// 整数类型正确
		case float64:
			// 检查是否为整数值
			if f, ok := value.(float64); ok && f != float64(int64(f)) {
				return fmt.Errorf("argument %s must be an integer, got float %v", fieldName, f)
			}
		default:
			return fmt.Errorf("argument %s must be an integer, got %T", fieldName, value)
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("argument %s must be a boolean, got %T", fieldName, value)
		}

	case "array":
		slice := reflect.ValueOf(value)
		if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
			return fmt.Errorf("argument %s must be an array, got %T", fieldName, value)
		}

		// 验证数组元素
		if schema.Items != nil {
			for i := 0; i < slice.Len(); i++ {
				item := slice.Index(i).Interface()
				if err := v.validateValue(item, schema.Items, fmt.Sprintf("%s[%d]", fieldName, i)); err != nil {
					return err
				}
			}
		}

	case "object":
		objMap, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("argument %s must be an object, got %T", fieldName, value)
		}

		// 递归验证对象属性
		if schema.Properties != nil {
			for key, val := range objMap {
				if propSchema, exists := schema.Properties[key]; exists {
					if err := v.validateValue(val, propSchema, fmt.Sprintf("%s.%s", fieldName, key)); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// violatesConstraint 检查是否违反约束
func (v *toolValidator) violatesConstraint(call *models.ToolCall, constraint string) bool {
	switch constraint {
	case "read-only":
		// 只读模式：禁止写操作
		return v.isWriteOperation(call.Function.Name)
	case "no-file-operations":
		// 禁止文件操作
		return v.isFileOperation(call.Function.Name)
	case "no-external-access":
		// 禁止外部访问
		return v.isExternalOperation(call.Function.Name)
	default:
		return false
	}
}

// isWriteOperation 检查是否为写操作
func (v *toolValidator) isWriteOperation(toolName string) bool {
	writeKeywords := []string{"write", "create", "update", "delete", "modify", "commit", "push"}
	lowerName := strings.ToLower(toolName)

	for _, keyword := range writeKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}
	return false
}

// isFileOperation 检查是否为文件操作
func (v *toolValidator) isFileOperation(toolName string) bool {
	fileKeywords := []string{"file", "read", "write", "create", "delete", "list", "search"}
	lowerName := strings.ToLower(toolName)

	for _, keyword := range fileKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}
	return false
}

// isExternalOperation 检查是否为外部操作
func (v *toolValidator) isExternalOperation(toolName string) bool {
	externalKeywords := []string{"http", "fetch", "api", "webhook", "notification"}
	lowerName := strings.ToLower(toolName)

	for _, keyword := range externalKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}
	return false
}

// getRequiredPermission 获取工具所需权限
func (v *toolValidator) getRequiredPermission(toolName string) string {
	lowerName := strings.ToLower(toolName)

	if strings.Contains(lowerName, "github") {
		if v.isWriteOperation(toolName) {
			return "github:write"
		}
		return "github:read"
	}

	if strings.Contains(lowerName, "file") {
		if v.isWriteOperation(toolName) {
			return "filesystem:write"
		}
		return "filesystem:read"
	}

	if v.isExternalOperation(toolName) {
		return "network:access"
	}

	return "" // 无特殊权限要求
}
