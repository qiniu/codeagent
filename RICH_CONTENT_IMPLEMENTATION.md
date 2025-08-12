# Rich Content Processing Implementation

This document describes the implementation of rich content processing for GitHub Issues and Pull Requests in the CodeAgent system.

## Problem Solved

**Issue #304**: Issue内容中的图片和富文本信息未能正确传递到AI模型的prompt中

Previously, the system only extracted plain text from GitHub Issues and comments using `event.Issue.GetBody()` and `event.Comment.GetBody()`, losing important rich content like:
- Images and attachments
- Code blocks with syntax highlighting
- Structured markdown content

## Solution Overview

Implemented a comprehensive rich content processing system that:
1. Extracts and processes images, documents, and code blocks from markdown
2. Formats rich content for optimal AI consumption
3. Maintains backward compatibility with existing functionality
4. Integrates seamlessly with the existing webhook → agent → AI workflow

## Key Components

### 1. Content Processor (`internal/content/processor.go`)

**Core Features:**
- **Attachment Detection**: Extracts GitHub user-attachments URLs for images and documents
- **Code Block Parsing**: Identifies and preserves code blocks with language information
- **Rich Formatting**: Formats content with attachment metadata for AI consumption
- **HTTP Client**: Downloads attachment metadata (content-type, size) for enhanced context

**Key Methods:**
- `ProcessContent()`: Processes raw markdown into structured rich content
- `FormatForAI()`: Formats rich content for AI model consumption
- `DownloadAttachment()`: Downloads attachment content when needed

### 2. Enhanced Agent Processing

**Updated Methods:**
- `ProcessIssueCommentWithAI()`: Already used rich content processing ✅
- `processPRWithArgsAndAI()`: Now processes PR comment rich content
- `ContinuePRFromReviewCommentWithAI()`: Enhanced with rich content processing  
- `FixPRFromReviewCommentWithAI()`: Enhanced with rich content processing
- `ProcessPRFromReviewWithTriggerUserAndAI()`: Enhanced for batch review processing

**Integration Pattern:**
```go
// Process rich content from comment
if richContent, err := a.contentProcessor.ProcessContent(ctx, comment.GetBody()); err == nil {
    formattedContent = a.contentProcessor.FormatForAI(richContent)
    log.Infof("Processed rich content: %d attachments, %d code blocks", 
        len(richContent.Attachments), len(richContent.CodeBlocks))
} else {
    log.Warnf("Failed to process rich content, using plain text: %v", err)
    formattedContent = comment.GetBody() // Fallback to plain text
}
```

### 3. Enhanced Context Collection

**Updated Components:**
- `DefaultContextCollector`: Now processes rich content in all context collection methods
- Added `CollectBasicContextWithProcessor()` with context support
- Added `CollectCommentContextWithProcessor()` for rich comment processing
- Safe fallback mechanism to plain text if processing fails

### 4. Webhook Handler Integration

**Enhanced Features:**
- Added `createEnhancedIssueCommentEvent()` for rich content pre-processing
- Content processor initialization in handler
- Rich content logging for debugging and monitoring

## Rich Content Format

### Input (Raw Markdown):
```markdown
Here's a bug report with screenshot:

![Error Screenshot](https://github.com/user-attachments/assets/abc123/error.png)

The code causing the issue:
```python
def broken_function():
    return None  # This breaks everything
```

Please see the [log file](https://github.com/user-attachments/files/456/debug.log) for details.
```

### Output (AI-Formatted):
```markdown
Here's a bug report with screenshot:

![Error Screenshot](https://github.com/user-attachments/assets/abc123/error.png)

The code causing the issue:
```python
def broken_function():
    return None  # This breaks everything
```

Please see the [log file](https://github.com/user-attachments/files/456/debug.log) for details.

## Attachments:
1. **error.png** (image)
   - URL: https://github.com/user-attachments/assets/abc123/error.png
   - Description: Error Screenshot

2. **debug.log** (document)  
   - URL: https://github.com/user-attachments/files/456/debug.log

## Code Blocks:
1. Language: python
   Content:
```python
def broken_function():
    return None  # This breaks everything
```
```

## Files Modified

### New Files:
- `internal/content/processor.go` - Core rich content processing logic
- `internal/content/processor_test.go` - Comprehensive test suite  
- `internal/webhook/enhanced_events.go` - Enhanced event structures
- `RICH_CONTENT_IMPLEMENTATION.md` - This documentation

### Modified Files:
- `internal/agent/agent.go` - Enhanced PR processing methods with rich content
- `internal/context/collector.go` - Enhanced context collection with rich content
- `internal/webhook/handler.go` - Added content processor integration

## Backward Compatibility

✅ **Full backward compatibility maintained:**
- All existing method signatures unchanged
- Graceful fallback to plain text if rich processing fails
- No breaking changes to existing functionality
- Command parsing still works with raw comment text

## Testing

Comprehensive test coverage includes:
- Plain text processing (baseline)
- Image attachment extraction
- Document attachment extraction  
- Code block parsing
- Mixed content scenarios
- Error handling and fallbacks

**Test Results:**
```
=== RUN   TestProcessContent
--- PASS: TestProcessContent (0.38s)
=== RUN   TestProcessorCreation  
--- PASS: TestProcessorCreation (0.00s)
PASS
```

## Performance Considerations

- **Lazy Processing**: Rich content is only processed when content contains potential attachments
- **HTTP Client Reuse**: Single HTTP client with timeout for all attachment metadata requests
- **Memory Efficient**: Processes content in streaming fashion without loading large attachments
- **Caching Ready**: Architecture supports adding caching layer for downloaded attachments

## Future Enhancements

Possible future improvements:
1. **Image Analysis**: Use AI vision capabilities to analyze attached images
2. **Document Processing**: Extract text content from PDFs and other documents  
3. **Attachment Caching**: Cache downloaded attachments for faster subsequent access
4. **Rich Response Generation**: Generate responses that can include images and formatted content

## Usage in AI Prompts

The rich content system enables AI models to:
- **See Attached Images**: Reference and analyze screenshots, diagrams, and error images
- **Process Code Blocks**: Understand code structure with proper syntax highlighting context
- **Access Documents**: Know about attached documentation and log files
- **Generate Contextual Responses**: Create responses that acknowledge and work with all provided content

This implementation significantly enhances the AI's ability to understand and respond to complex GitHub Issues and Pull Requests with rich multimedia content.