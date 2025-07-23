package prompt

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/qbox/codeagent/internal/code"
)

// OutputValidator 输出验证器
type OutputValidator struct {
	codeClient code.Code
}

// ValidationResult 验证结果
type ValidationResult struct {
	IsValid      bool                   `json:"is_valid"`
	FixedContent string                 `json:"fixed_content"`
	Issues       []string               `json:"issues"`
	Suggestions  []string               `json:"suggestions"`
	QualityScore float64                `json:"quality_score"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// NewOutputValidator 创建新的输出验证器
func NewOutputValidator(codeClient code.Code) *OutputValidator {
	return &OutputValidator{
		codeClient: codeClient,
	}
}

// ValidateAndFixCode 验证并修复代码
func (v *OutputValidator) ValidateAndFixCode(ctx context.Context, codeContent string, language string) (*ValidationResult, error) {
	// 使用 AI 进行代码质量验证和修复
	prompt := "你是一位资深的代码审查专家，请对以下" + language + "代码进行全面的质量检查：\n\n" +
		codeContent + "\n\n" +
		"请按以下格式回复：\n" +
		"## 代码质量评估\n" +
		"- 语法正确性: [通过/失败] - [说明]\n" +
		"- 代码风格: [通过/失败] - [说明]\n" +
		"- 性能优化: [通过/失败] - [说明]\n" +
		"- 安全性: [通过/失败] - [说明]\n" +
		"- 可维护性: [通过/失败] - [说明]\n\n" +
		"## 修复后的代码\n" +
		"```" + language + "\n" +
		"// 修复后的代码\n" +
		"```\n\n" +
		"## 发现的问题\n" +
		"- [问题 1]\n" +
		"- [问题 2]\n\n" +
		"## 优化建议\n" +
		"- [建议 1]\n" +
		"- [建议 2]\n\n" +
		"## 最佳实践指导\n" +
		"- [指导 1]\n" +
		"- [指导 2]"

	resp, err := v.codeClient.Prompt(prompt)
	if err != nil {
		return nil, fmt.Errorf("AI代码验证失败: %w", err)
	}

	// 读取响应内容
	content, err := io.ReadAll(resp.Out)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析AI验证结果
	return v.parseAICodeValidationResult(string(content))
}

// parseAICodeValidationResult 解析AI代码验证结果
func (v *OutputValidator) parseAICodeValidationResult(content string) (*ValidationResult, error) {
	result := &ValidationResult{
		IsValid:      true,
		Issues:       []string{},
		Suggestions:  []string{},
		QualityScore: 0.0,
		Metadata:     make(map[string]interface{}),
	}

	lines := strings.Split(content, "\n")
	var currentSection string
	var codeBlock bool
	var fixedCode strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 检测章节
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimPrefix(line, "## ")
			continue
		}

		// 检测代码块
		if strings.Contains(line, "```") {
			codeBlock = !codeBlock
			if codeBlock {
				// 开始代码块
				continue
			} else {
				// 结束代码块
				continue
			}
		}

		// 在代码块中收集修复后的代码
		if codeBlock && currentSection == "修复后的代码" {
			fixedCode.WriteString(line + "\n")
		}

		// 解析问题
		if currentSection == "发现的问题" && strings.HasPrefix(line, "- ") {
			issue := strings.TrimPrefix(line, "- ")
			if issue != "" {
				result.Issues = append(result.Issues, issue)
			}
		}

		// 解析建议
		if currentSection == "优化建议" && strings.HasPrefix(line, "- ") {
			suggestion := strings.TrimPrefix(line, "- ")
			if suggestion != "" {
				result.Suggestions = append(result.Suggestions, suggestion)
			}
		}

		// 解析质量评估
		if currentSection == "代码质量评估" {
			if strings.Contains(line, "失败") {
				result.IsValid = false
			}
		}
	}

	result.FixedContent = strings.TrimSpace(fixedCode.String())

	// 计算质量分数
	result.QualityScore = v.calculateQualityScore(result)

	return result, nil
}

// calculateQualityScore 计算质量分数
func (v *OutputValidator) calculateQualityScore(result *ValidationResult) float64 {
	score := 1.0

	// 根据问题数量扣分
	if len(result.Issues) > 0 {
		score -= float64(len(result.Issues)) * 0.1
	}

	// 根据建议数量加分
	if len(result.Suggestions) > 0 {
		score += float64(len(result.Suggestions)) * 0.05
	}

	// 确保分数在 0-1 之间
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// ValidatePromptOutput 验证 Prompt 输出
func (v *OutputValidator) ValidatePromptOutput(ctx context.Context, output string) (*ValidationResult, error) {
	// 使用 AI 验证 Prompt 输出格式和内容
	prompt := "请验证以下 AI 输出是否符合要求：\n\n" +
		output + "\n\n" +
		"请按以下格式回复：\n" +
		"## 验证结果\n" +
		"- 格式正确性: [通过/失败] - [说明]\n" +
		"- 内容完整性: [通过/失败] - [说明]\n" +
		"- 逻辑一致性: [通过/失败] - [说明]\n\n" +
		"## 修复后的输出\n" +
		"[修复后的输出内容]\n\n" +
		"## 发现的问题\n" +
		"- [问题 1]\n" +
		"- [问题 2]\n\n" +
		"## 改进建议\n" +
		"- [建议 1]\n" +
		"- [建议 2]"

	resp, err := v.codeClient.Prompt(prompt)
	if err != nil {
		return nil, fmt.Errorf("AI输出验证失败: %w", err)
	}

	// 读取响应内容
	content, err := io.ReadAll(resp.Out)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析验证结果
	return v.parseAIOutputValidationResult(string(content))
}

// parseAIOutputValidationResult 解析AI输出验证结果
func (v *OutputValidator) parseAIOutputValidationResult(content string) (*ValidationResult, error) {
	result := &ValidationResult{
		IsValid:      true,
		Issues:       []string{},
		Suggestions:  []string{},
		QualityScore: 0.0,
		Metadata:     make(map[string]interface{}),
	}

	lines := strings.Split(content, "\n")
	var currentSection string
	var fixedOutput strings.Builder
	var inFixedOutput bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 检测章节
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimPrefix(line, "## ")
			if currentSection == "修复后的输出" {
				inFixedOutput = true
			} else {
				inFixedOutput = false
			}
			continue
		}

		// 收集修复后的输出
		if inFixedOutput && line != "" {
			fixedOutput.WriteString(line + "\n")
		}

		// 解析问题
		if currentSection == "发现的问题" && strings.HasPrefix(line, "- ") {
			issue := strings.TrimPrefix(line, "- ")
			if issue != "" {
				result.Issues = append(result.Issues, issue)
			}
		}

		// 解析建议
		if currentSection == "改进建议" && strings.HasPrefix(line, "- ") {
			suggestion := strings.TrimPrefix(line, "- ")
			if suggestion != "" {
				result.Suggestions = append(result.Suggestions, suggestion)
			}
		}

		// 解析验证结果
		if currentSection == "验证结果" {
			if strings.Contains(line, "失败") {
				result.IsValid = false
			}
		}
	}

	result.FixedContent = strings.TrimSpace(fixedOutput.String())

	// 计算质量分数
	result.QualityScore = v.calculateQualityScore(result)

	return result, nil
}
