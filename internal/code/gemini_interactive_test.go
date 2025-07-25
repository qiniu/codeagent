package code

import (
	"bytes"
	"testing"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
)

func TestNewGeminiInteractive(t *testing.T) {
	// 跳过实际的Docker测试，因为可能没有Docker环境
	t.Skip("Skipping Docker-based test in CI environment")

	// 创建测试工作空间
	workspace := &models.Workspace{
		Repository:  "https://github.com/test/repo.git",
		Org:         "testorg",
		PRNumber:    123,
		Path:        "/tmp/test-workspace",
		SessionPath: "/tmp/test-session",
	}

	// 创建测试配置
	cfg := &config.Config{
		Gemini: config.GeminiConfig{
			APIKey:             "test-api-key",
			ContainerImage:     "test-gemini-image",
			Timeout:            5 * time.Minute,
			GoogleCloudProject: "test-project",
			Interactive:        true,
		},
		UseDocker: true,
	}

	// 测试创建交互式Gemini实例（在没有Docker的环境下会失败，这是预期的）
	_, err := NewGeminiInteractive(workspace, cfg)
	if err != nil {
		t.Logf("Expected error in test environment without Docker: %v", err)
		return
	}

	// 如果成功创建（在有Docker的环境下），这里可以添加更多测试
	t.Log("GeminiInteractive created successfully")
}

func TestGeminiInteractiveResponseReader(t *testing.T) {
	// 创建足够长的缓冲区数据以通过长度检查
	longBufferData := "这是一个很长的响应数据，用于测试响应完成检测功能。" + 
		"我们需要确保缓冲区足够长，至少500个字符，这样才能正确测试响应完成的检测逻辑。" +
		"这个测试是为了验证Gemini交互式响应读取器的功能是否正常工作。" +
		"让我们继续添加更多的文本来达到所需的长度限制。" +
		"这样我们就可以确保isGeminiResponseComplete函数能够正确地检测响应是否完成。" +
		"当然，实际的使用场景中，响应内容会更长，包含真实的AI生成内容。" +
		"这个测试主要是为了确保代码质量和功能的正确性。"

	// 创建模拟的响应读取器
	reader := &GeminiInteractiveResponseReader{
		buffer: *bytes.NewBufferString(longBufferData),
	}

	// 测试响应完成检测 - 使用包含完成标记的数据
	testData := []byte("## 改动摘要\ntest\n## 具体改动\ntest")
	if !reader.isGeminiResponseComplete(testData) {
		t.Error("Should detect completion with standard completion markers")
	}

	// 测试Gemini CLI提示符检测
	testData2 := []byte("gemini> ")
	if !reader.isGeminiResponseComplete(testData2) {
		t.Error("Should detect completion with gemini prompt")
	}

	// 测试长度不足的情况
	shortReader := &GeminiInteractiveResponseReader{
		buffer: *bytes.NewBufferString("short data"),
	}
	if shortReader.isGeminiResponseComplete([]byte("gemini> ")) {
		t.Error("Should not detect completion with insufficient buffer length")
	}
}