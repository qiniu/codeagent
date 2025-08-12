package content

import (
	"context"
	"testing"
)

func TestProcessContent(t *testing.T) {
	processor := NewProcessor()
	ctx := context.Background()

	tests := []struct {
		name            string
		input           string
		expectedImages  int
		expectedDocs    int
		expectedCode    int
		shouldContain   []string
	}{
		{
			name:          "plain text",
			input:         "This is just plain text",
			expectedImages: 0,
			expectedDocs:  0,
			expectedCode:  0,
			shouldContain: []string{"This is just plain text"},
		},
		{
			name:          "text with image",
			input:         "Check this ![screenshot](https://github.com/user-attachments/assets/abc123/screenshot.png)",
			expectedImages: 1,
			expectedDocs:  0,
			expectedCode:  0,
			shouldContain: []string{"Check this", "## Attachments:", "screenshot.png"},
		},
		{
			name:          "text with code block",
			input:         "Here's the fix:\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
			expectedImages: 0,
			expectedDocs:  0,
			expectedCode:  1,
			shouldContain: []string{"Here's the fix:", "## Code Blocks:", "Language: go"},
		},
		{
			name:          "text with document link",
			input:         "Please see [requirements.pdf](https://github.com/user-attachments/files/123/requirements.pdf)",
			expectedImages: 0,
			expectedDocs:  1,
			expectedCode:  0,
			shouldContain: []string{"Please see", "## Attachments:", "requirements.pdf"},
		},
		{
			name:          "mixed content",
			input:         "Bug report with ![error](https://github.com/user-attachments/assets/abc/error.png) and code:\n```python\nprint('error')\n```\nSee also [logs.txt](https://github.com/user-attachments/files/456/logs.txt)",
			expectedImages: 1,
			expectedDocs:  1,
			expectedCode:  1,
			shouldContain: []string{"Bug report", "## Attachments:", "error.png", "logs.txt", "## Code Blocks:", "Language: python"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ProcessContent(ctx, tt.input)
			if err != nil {
				t.Errorf("ProcessContent() error = %v", err)
				return
			}

			if len(result.Attachments) != tt.expectedImages+tt.expectedDocs {
				t.Errorf("Expected %d attachments, got %d", tt.expectedImages+tt.expectedDocs, len(result.Attachments))
			}

			imageCount := 0
			docCount := 0
			for _, att := range result.Attachments {
				if att.Type == AttachmentTypeImage {
					imageCount++
				} else if att.Type == AttachmentTypeDocument {
					docCount++
				}
			}

			if imageCount != tt.expectedImages {
				t.Errorf("Expected %d images, got %d", tt.expectedImages, imageCount)
			}

			if docCount != tt.expectedDocs {
				t.Errorf("Expected %d documents, got %d", tt.expectedDocs, docCount)
			}

			if len(result.CodeBlocks) != tt.expectedCode {
				t.Errorf("Expected %d code blocks, got %d", tt.expectedCode, len(result.CodeBlocks))
			}

			formatted := processor.FormatForAI(result)
			for _, expected := range tt.shouldContain {
				if !containsString(formatted, expected) {
					t.Errorf("Formatted output should contain '%s', but doesn't. Output: %s", expected, formatted)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[0:len(substr)] == substr || s[len(s)-len(substr):] == substr || findInString(s, substr))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestProcessorCreation(t *testing.T) {
	processor := NewProcessor()
	if processor == nil {
		t.Error("NewProcessor() returned nil")
	}
	if processor.httpClient == nil {
		t.Error("processor.httpClient is nil")
	}
}