package models

import (
	"testing"

	"github.com/google/go-github/v58/github"
)

func TestParseMention(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
		args     string // 现在应该是完整内容
		aiModel  string
	}{
		// 正确的匹配案例
		{
			name:     "独立的@qiniu-ci",
			content:  "@qiniu-ci",
			expected: true,
			args:     "@qiniu-ci", // 现在传递完整内容
			aiModel:  "",
		},
		{
			name:     "@qiniu-ci后跟标点",
			content:  "@qiniu-ci, 请分析代码",
			expected: true,
			args:     "@qiniu-ci, 请分析代码", // 完整内容
			aiModel:  "",
		},
		{
			name:     "@qiniu-ci在句子开头",
			content:  "@qiniu-ci 这个函数有性能问题吗？",
			expected: true,
			args:     "@qiniu-ci 这个函数有性能问题吗？", // 完整内容
			aiModel:  "",
		},
		{
			name:     "前后有空格的@qiniu-ci",
			content:  "Hello @qiniu-ci please help me",
			expected: true,
			args:     "Hello @qiniu-ci please help me", // 完整内容
			aiModel:  "",
		},
		{
			name:     "@qiniu-ci后跟感叹号",
			content:  "@qiniu-ci! 快来帮忙",
			expected: true,
			args:     "@qiniu-ci! 快来帮忙", // 完整内容
			aiModel:  "",
		},
		{
			name:     "@qiniu-ci指定Claude模型",
			content:  "@qiniu-ci -claude 检查算法复杂度",
			expected: true,
			args:     "@qiniu-ci -claude 检查算法复杂度", // 完整内容
			aiModel:  AIModelClaude,
		},
		{
			name:     "@qiniu-ci指定Gemini模型",
			content:  "@qiniu-ci -gemini 这个模块如何重构",
			expected: true,
			args:     "@qiniu-ci -gemini 这个模块如何重构", // 完整内容
			aiModel:  AIModelGemini,
		},
		// 不匹配的案例
		{
			name:     "作为其他词的一部分",
			content:  "@qiniu-ci-test",
			expected: false,
		},
		{
			name:     "在邮箱地址中",
			content:  "email@qiniu-ci.com",
			expected: false,
		},
		{
			name:     "在URL中",
			content:  "https://github.com/@qiniu-ci/repo",
			expected: false,
		},
		{
			name:     "不包含@qiniu-ci",
			content:  "这是一个普通的评论",
			expected: false,
		},
		{
			name:     "空字符串",
			content:  "",
			expected: false,
		},
		{
			name:     "只包含@qiniu但不完整",
			content:  "@qiniu 请帮忙",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdInfo, found := parseMention(tt.content)

			if found != tt.expected {
				t.Errorf("parseMention(%q) found = %v, want %v", tt.content, found, tt.expected)
				return
			}

			if !tt.expected {
				return // 如果期望不匹配，不需要检查其他字段
			}

			if cmdInfo == nil {
				t.Errorf("parseMention(%q) returned nil cmdInfo when found = true", tt.content)
				return
			}

			if cmdInfo.Command != CommandMention {
				t.Errorf("parseMention(%q) command = %v, want %v", tt.content, cmdInfo.Command, CommandMention)
			}

			if cmdInfo.Args != tt.args {
				t.Errorf("parseMention(%q) args = %q, want %q", tt.content, cmdInfo.Args, tt.args)
			}

			if cmdInfo.AIModel != tt.aiModel {
				t.Errorf("parseMention(%q) aiModel = %q, want %q", tt.content, cmdInfo.AIModel, tt.aiModel)
			}

			if cmdInfo.RawText != tt.content {
				t.Errorf("parseMention(%q) rawText = %q, want %q", tt.content, cmdInfo.RawText, tt.content)
			}
		})
	}
}

func TestHasCommand(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		expected bool
		command  string
	}{
		{
			name:     "Issue评论中的@qiniu-ci",
			context:  "请帮我 @qiniu-ci 分析这个函数",
			expected: true,
			command:  CommandMention,
		},
		{
			name:     "斜杠命令优先级更高",
			context:  "/code 实现登录功能 @qiniu-ci",
			expected: true,
			command:  CommandCode,
		},
		{
			name:     "无匹配命令",
			context:  "这是一个普通的评论，没有特殊指令",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建一个模拟的IssueCommentContext
			body := tt.context
			mockComment := &IssueCommentContext{
				BaseContext: BaseContext{
					Type: EventIssueComment,
				},
				Comment: &github.IssueComment{
					Body: &body,
				},
			}

			cmdInfo, found := HasCommand(mockComment)

			if found != tt.expected {
				t.Errorf("HasCommand() found = %v, want %v", found, tt.expected)
				return
			}

			if tt.expected && cmdInfo.Command != tt.command {
				t.Errorf("HasCommand() command = %v, want %v", cmdInfo.Command, tt.command)
			}
		})
	}
}

func TestParseMentionWithConfig(t *testing.T) {
	// 创建自定义mention配置
	mentionConfig := &ConfigMentionAdapter{
		Triggers:       []string{"@ai", "@code-assistant", "@qiniu-ci"},
		DefaultTrigger: "@ai",
	}

	tests := []struct {
		name     string
		content  string
		expected bool
		args     string
		aiModel  string
	}{
		{
			name:     "使用@ai触发",
			content:  "@ai 请帮我优化这个函数",
			expected: true,
			args:     "@ai 请帮我优化这个函数",
			aiModel:  "",
		},
		{
			name:     "使用@code-assistant触发",
			content:  "Hello @code-assistant, can you review this?",
			expected: true,
			args:     "Hello @code-assistant, can you review this?",
			aiModel:  "",
		},
		{
			name:     "使用原有@qiniu-ci触发",
			content:  "@qiniu-ci -claude 分析性能",
			expected: true,
			args:     "@qiniu-ci -claude 分析性能",
			aiModel:  AIModelClaude,
		},
		{
			name:     "不匹配的mention",
			content:  "@unknown 请帮忙",
			expected: false,
		},
		{
			name:     "空配置时不匹配",
			content:  "@ai 请帮忙",
			expected: false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmdInfo *CommandInfo
			var found bool

			// 最后一个测试用例使用空配置
			if i == len(tests)-1 {
				emptyConfig := &ConfigMentionAdapter{
					Triggers:       []string{},
					DefaultTrigger: "",
				}
				cmdInfo, found = parseMentionWithConfig(tt.content, emptyConfig)
			} else {
				cmdInfo, found = parseMentionWithConfig(tt.content, mentionConfig)
			}

			if found != tt.expected {
				t.Errorf("parseMentionWithConfig(%q) found = %v, want %v", tt.content, found, tt.expected)
				return
			}

			if !tt.expected {
				return
			}

			if cmdInfo == nil {
				t.Errorf("parseMentionWithConfig(%q) returned nil cmdInfo when found = true", tt.content)
				return
			}

			if cmdInfo.Args != tt.args {
				t.Errorf("parseMentionWithConfig(%q) args = %q, want %q", tt.content, cmdInfo.Args, tt.args)
			}

			if cmdInfo.AIModel != tt.aiModel {
				t.Errorf("parseMentionWithConfig(%q) aiModel = %q, want %q", tt.content, cmdInfo.AIModel, tt.aiModel)
			}
		})
	}
}
