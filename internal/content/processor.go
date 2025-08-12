package content

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// AttachmentType represents the type of attachment
type AttachmentType string

const (
	AttachmentTypeImage    AttachmentType = "image"
	AttachmentTypeDocument AttachmentType = "document"
	AttachmentTypeUnknown  AttachmentType = "unknown"
)

// AttachmentInfo contains information about an attachment
type AttachmentInfo struct {
	Type        AttachmentType `json:"type"`
	URL         string         `json:"url"`
	Filename    string         `json:"filename"`
	AltText     string         `json:"alt_text,omitempty"`
	Size        int64          `json:"size,omitempty"`
	ContentType string         `json:"content_type,omitempty"`
}

// RichContent represents processed rich content from GitHub
type RichContent struct {
	PlainText   string           `json:"plain_text"`
	Attachments []AttachmentInfo `json:"attachments"`
	CodeBlocks  []CodeBlock      `json:"code_blocks"`
}

// CodeBlock represents a code block in markdown
type CodeBlock struct {
	Language string `json:"language"`
	Content  string `json:"content"`
	Line     int    `json:"line"`
}

// Processor handles rich content extraction and processing
type Processor struct {
	httpClient *http.Client
	// Image patterns for GitHub attachment URLs
	imagePattern    *regexp.Regexp
	documentPattern *regexp.Regexp
	codeBlockPattern *regexp.Regexp
}

// NewProcessor creates a new content processor
func NewProcessor() *Processor {
	return &Processor{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Match GitHub user-attachments image URLs
		imagePattern: regexp.MustCompile(`!\[([^\]]*)\]\((https://github\.com/user-attachments/assets/[^)]+\.(png|jpg|jpeg|gif|svg|webp))\)`),
		// Match GitHub user-attachments file URLs  
		documentPattern: regexp.MustCompile(`\[([^\]]+)\]\((https://github\.com/user-attachments/(?:assets|files)/[^)]+)\)`),
		// Match code blocks
		codeBlockPattern: regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```"),
	}
}

// ProcessContent extracts rich content from GitHub issue/comment body
func (p *Processor) ProcessContent(ctx context.Context, body string) (*RichContent, error) {
	content := &RichContent{
		PlainText:   body,
		Attachments: []AttachmentInfo{},
		CodeBlocks:  []CodeBlock{},
	}

	// Extract image attachments
	imageMatches := p.imagePattern.FindAllStringSubmatch(body, -1)
	for _, match := range imageMatches {
		if len(match) >= 3 {
			attachment := AttachmentInfo{
				Type:     AttachmentTypeImage,
				URL:      match[2],
				AltText:  match[1],
				Filename: p.extractFilenameFromURL(match[2]),
			}
			content.Attachments = append(content.Attachments, attachment)
		}
	}

	// Extract document attachments
	docMatches := p.documentPattern.FindAllStringSubmatch(body, -1)
	for _, match := range docMatches {
		if len(match) >= 3 {
			// Skip if already captured as image
			if p.isImageURL(match[2]) {
				continue
			}
			attachment := AttachmentInfo{
				Type:     AttachmentTypeDocument,
				URL:      match[2],
				Filename: match[1],
			}
			content.Attachments = append(content.Attachments, attachment)
		}
	}

	// Extract code blocks
	codeMatches := p.codeBlockPattern.FindAllStringSubmatch(body, -1)
	line := 1
	for _, match := range codeMatches {
		if len(match) >= 3 {
			codeBlock := CodeBlock{
				Language: match[1],
				Content:  strings.TrimSpace(match[2]),
				Line:     line,
			}
			content.CodeBlocks = append(content.CodeBlocks, codeBlock)
			line++
		}
	}

	// Enrich attachments with metadata
	if err := p.enrichAttachments(ctx, content.Attachments); err != nil {
		// Log error but don't fail - continue with partial information
		fmt.Printf("Warning: Failed to enrich attachments: %v\n", err)
	}

	return content, nil
}

// enrichAttachments fetches metadata for attachments
func (p *Processor) enrichAttachments(ctx context.Context, attachments []AttachmentInfo) error {
	for i := range attachments {
		attachment := &attachments[i]
		
		// Make HEAD request to get content type and size
		req, err := http.NewRequestWithContext(ctx, "HEAD", attachment.URL, nil)
		if err != nil {
			continue
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			attachment.ContentType = resp.Header.Get("Content-Type")
			if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
				// Parse content length if needed
			}
			
			// Determine type from content type if not already set
			if attachment.Type == AttachmentTypeUnknown {
				if strings.HasPrefix(attachment.ContentType, "image/") {
					attachment.Type = AttachmentTypeImage
				} else {
					attachment.Type = AttachmentTypeDocument
				}
			}
		}
	}
	return nil
}

// DownloadAttachment downloads the content of an attachment
func (p *Processor) DownloadAttachment(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}

// extractFilenameFromURL extracts filename from GitHub attachment URL
func (p *Processor) extractFilenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove query parameters if any
		if idx := strings.Index(filename, "?"); idx != -1 {
			filename = filename[:idx]
		}
		return filename
	}
	return ""
}

// isImageURL checks if URL points to an image
func (p *Processor) isImageURL(url string) bool {
	imageExtensions := []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp"}
	urlLower := strings.ToLower(url)
	for _, ext := range imageExtensions {
		if strings.Contains(urlLower, ext) {
			return true
		}
	}
	return false
}

// FormatForAI formats rich content for AI consumption
func (p *Processor) FormatForAI(content *RichContent) string {
	result := content.PlainText

	// Add attachment information
	if len(content.Attachments) > 0 {
		result += "\n\n## Attachments:\n"
		for i, attachment := range content.Attachments {
			result += fmt.Sprintf("%d. **%s** (%s)\n", i+1, attachment.Filename, attachment.Type)
			result += fmt.Sprintf("   - URL: %s\n", attachment.URL)
			if attachment.AltText != "" {
				result += fmt.Sprintf("   - Description: %s\n", attachment.AltText)
			}
			if attachment.ContentType != "" {
				result += fmt.Sprintf("   - Type: %s\n", attachment.ContentType)
			}
		}
	}

	// Add code block summary
	if len(content.CodeBlocks) > 0 {
		result += "\n\n## Code Blocks:\n"
		for i, block := range content.CodeBlocks {
			result += fmt.Sprintf("%d. Language: %s\n", i+1, block.Language)
			if len(block.Content) > 200 {
				result += fmt.Sprintf("   Preview: %s...\n", block.Content[:200])
			} else {
				result += fmt.Sprintf("   Content:\n```%s\n%s\n```\n", block.Language, block.Content)
			}
		}
	}

	return result
}