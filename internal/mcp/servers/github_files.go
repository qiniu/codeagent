package servers

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/pkg/models"

	githubapi "github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// GitHubFilesServer GitHub文件操作MCP服务器
// 对应claude-code-action中的GitHubFileOperationsServer
type GitHubFilesServer struct {
	client *github.Client
	info   *models.MCPServerInfo
}

// NewGitHubFilesServer 创建GitHub文件操作服务器
func NewGitHubFilesServer(client *github.Client) *GitHubFilesServer {
	return &GitHubFilesServer{
		client: client,
		info: &models.MCPServerInfo{
			Name:        "github-files",
			Version:     "1.0.0",
			Description: "GitHub repository file operations via API",
			Capabilities: models.MCPServerCapabilities{
				Tools: []models.Tool{
					{
						Name:        "read_file",
						Description: "Read a file from the GitHub repository",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"path": {
									Type:        "string",
									Description: "File path in the repository",
								},
								"ref": {
									Type:        "string",
									Description: "Git reference (branch/commit), defaults to HEAD",
								},
							},
							Required: []string{"path"},
						},
					},
					{
						Name:        "write_file",
						Description: "Write content to a file in the repository",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"path": {
									Type:        "string",
									Description: "File path in the repository",
								},
								"content": {
									Type:        "string",
									Description: "File content (UTF-8 text)",
								},
								"message": {
									Type:        "string",
									Description: "Commit message",
								},
								"branch": {
									Type:        "string",
									Description: "Target branch, defaults to default branch",
								},
								"encoding": {
									Type:        "string",
									Description: "Content encoding (text/base64), defaults to text",
									Enum:        []interface{}{"text", "base64"},
								},
							},
							Required: []string{"path", "content", "message"},
						},
					},
					{
						Name:        "list_files",
						Description: "List files in a directory",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"path": {
									Type:        "string",
									Description: "Directory path, defaults to root",
								},
								"ref": {
									Type:        "string",
									Description: "Git reference (branch/commit), defaults to HEAD",
								},
								"recursive": {
									Type:        "boolean",
									Description: "List files recursively, defaults to false",
								},
							},
						},
					},
					{
						Name:        "search_files",
						Description: "Search for files by name pattern",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"query": {
									Type:        "string",
									Description: "Search query (filename patterns)",
								},
								"path": {
									Type:        "string",
									Description: "Search within specific path",
								},
								"extension": {
									Type:        "string",
									Description: "Filter by file extension",
								},
							},
							Required: []string{"query"},
						},
					},
				},
			},
			CreatedAt: time.Now(),
		},
	}
}

// GetInfo 获取服务器信息
func (s *GitHubFilesServer) GetInfo() *models.MCPServerInfo {
	return s.info
}

// GetTools 获取服务器提供的工具列表
func (s *GitHubFilesServer) GetTools() []models.Tool {
	return s.info.Capabilities.Tools
}

// IsAvailable 检查服务器是否在当前上下文中可用
func (s *GitHubFilesServer) IsAvailable(ctx context.Context, mcpCtx *models.MCPContext) bool {
	if mcpCtx == nil || mcpCtx.Repository == nil {
		return false
	}

	// 检查是否有GitHub访问权限
	if mcpCtx.Permissions != nil {
		hasReadPerm := false
		for _, perm := range mcpCtx.Permissions {
			if perm == "github:read" || perm == "github:write" || perm == "github:admin" {
				hasReadPerm = true
				break
			}
		}
		if !hasReadPerm {
			return false
		}
	}

	return true
}

// HandleToolCall 处理工具调用
func (s *GitHubFilesServer) HandleToolCall(ctx context.Context, call *models.ToolCall, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	xl := xlog.NewWith(ctx)

	if mcpCtx.Repository == nil {
		return nil, fmt.Errorf("no repository context available")
	}

	owner := mcpCtx.Repository.GetRepository().Owner.GetLogin()
	repo := mcpCtx.Repository.GetRepository().GetName()

	xl.Infof("Executing GitHub files tool: %s on %s/%s", call.Function.Name, owner, repo)

	switch call.Function.Name {
	case "read_file":
		return s.readFile(ctx, call, owner, repo)
	case "write_file":
		return s.writeFile(ctx, call, owner, repo, mcpCtx)
	case "list_files":
		return s.listFiles(ctx, call, owner, repo)
	case "search_files":
		return s.searchFiles(ctx, call, owner, repo)
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Function.Name)
	}
}

// Initialize 初始化服务器
func (s *GitHubFilesServer) Initialize(ctx context.Context) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Initializing GitHub Files MCP server")
	return nil
}

// Shutdown 关闭服务器
func (s *GitHubFilesServer) Shutdown(ctx context.Context) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Shutting down GitHub Files MCP server")
	return nil
}

// readFile 读取文件
func (s *GitHubFilesServer) readFile(ctx context.Context, call *models.ToolCall, owner, repo string) (*models.ToolResult, error) {
	path := call.Function.Arguments["path"].(string)
	ref := "HEAD"
	if r, ok := call.Function.Arguments["ref"].(string); ok && r != "" {
		ref = r
	}

	// 使用GitHub API获取文件内容
	opts := &githubapi.RepositoryContentGetOptions{Ref: ref}
	fileContent, _, _, err := s.client.GetClient().Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to read file: %v", err),
			Type:    "error",
		}, nil
	}

	if fileContent == nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   "file not found or is a directory",
			Type:    "error",
		}, nil
	}

	// 解码文件内容
	content, err := fileContent.GetContent()
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to decode file content: %v", err),
			Type:    "error",
		}, nil
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"path":     path,
			"content":  content,
			"encoding": fileContent.GetEncoding(),
			"size":     fileContent.GetSize(),
			"sha":      fileContent.GetSHA(),
		},
		Type: "json",
	}, nil
}

// writeFile 写入文件
func (s *GitHubFilesServer) writeFile(ctx context.Context, call *models.ToolCall, owner, repo string, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	path := call.Function.Arguments["path"].(string)
	content := call.Function.Arguments["content"].(string)
	message := call.Function.Arguments["message"].(string)

	branch := ""
	if b, ok := call.Function.Arguments["branch"].(string); ok {
		branch = b
	}

	encoding := "text"
	if e, ok := call.Function.Arguments["encoding"].(string); ok {
		encoding = e
	}

	// 检查写权限
	hasWritePerm := false
	if mcpCtx.Permissions != nil {
		for _, perm := range mcpCtx.Permissions {
			if perm == "github:write" || perm == "github:admin" {
				hasWritePerm = true
				break
			}
		}
	}

	if !hasWritePerm {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   "insufficient permissions for write operation",
			Type:    "error",
		}, nil
	}

	// 准备文件内容
	var encodedContent string
	if encoding == "base64" {
		encodedContent = content
	} else {
		encodedContent = base64.StdEncoding.EncodeToString([]byte(content))
	}

	// 尝试获取现有文件的SHA（用于更新）
	var existingSHA *string
	if fileContent, _, _, err := s.client.GetClient().Repositories.GetContents(ctx, owner, repo, path, nil); err == nil && fileContent != nil {
		existingSHA = fileContent.SHA
	}

	// 创建或更新文件
	opts := &githubapi.RepositoryContentFileOptions{
		Message: &message,
		Content: []byte(encodedContent),
		SHA:     existingSHA,
	}

	if branch != "" {
		opts.Branch = &branch
	}

	result, _, err := s.client.GetClient().Repositories.CreateFile(ctx, owner, repo, path, opts)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to write file: %v", err),
			Type:    "error",
		}, nil
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"path":   path,
			"sha":    result.Content.GetSHA(),
			"commit": result.Commit.GetSHA(),
			"url":    result.Content.GetHTMLURL(),
		},
		Type: "json",
	}, nil
}

// listFiles 列出文件
func (s *GitHubFilesServer) listFiles(ctx context.Context, call *models.ToolCall, owner, repo string) (*models.ToolResult, error) {
	path := ""
	if p, ok := call.Function.Arguments["path"].(string); ok {
		path = p
	}

	ref := "HEAD"
	if r, ok := call.Function.Arguments["ref"].(string); ok && r != "" {
		ref = r
	}

	recursive := false
	if r, ok := call.Function.Arguments["recursive"].(bool); ok {
		recursive = r
	}

	opts := &githubapi.RepositoryContentGetOptions{Ref: ref}
	_, directoryContent, _, err := s.client.GetClient().Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to list files: %v", err),
			Type:    "error",
		}, nil
	}

	var files []map[string]interface{}

	for _, item := range directoryContent {
		fileInfo := map[string]interface{}{
			"name":     item.GetName(),
			"path":     item.GetPath(),
			"type":     item.GetType(),
			"size":     item.GetSize(),
			"sha":      item.GetSHA(),
			"url":      item.GetHTMLURL(),
			"download": item.GetDownloadURL(),
		}

		files = append(files, fileInfo)

		// 递归列出子目录（如果启用）
		if recursive && item.GetType() == "dir" {
			subCall := &models.ToolCall{
				ID: call.ID,
				Function: models.ToolFunction{
					Name: "list_files",
					Arguments: map[string]interface{}{
						"path":      item.GetPath(),
						"ref":       ref,
						"recursive": true,
					},
				},
			}

			subResult, err := s.listFiles(ctx, subCall, owner, repo)
			if err == nil && subResult.Success {
				if subFiles, ok := subResult.Content.(map[string]interface{})["files"].([]map[string]interface{}); ok {
					files = append(files, subFiles...)
				}
			}
		}
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"path":  path,
			"files": files,
			"count": len(files),
		},
		Type: "json",
	}, nil
}

// searchFiles 搜索文件
func (s *GitHubFilesServer) searchFiles(ctx context.Context, call *models.ToolCall, owner, repo string) (*models.ToolResult, error) {
	query := call.Function.Arguments["query"].(string)

	searchPath := ""
	if p, ok := call.Function.Arguments["path"].(string); ok {
		searchPath = p
	}

	extension := ""
	if e, ok := call.Function.Arguments["extension"].(string); ok {
		extension = e
	}

	// 构建GitHub搜索查询
	searchQuery := fmt.Sprintf("%s repo:%s/%s", query, owner, repo)
	if searchPath != "" {
		searchQuery += fmt.Sprintf(" path:%s", searchPath)
	}
	if extension != "" {
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		searchQuery += fmt.Sprintf(" extension:%s", extension[1:])
	}

	// 执行搜索
	opts := &githubapi.SearchOptions{
		ListOptions: githubapi.ListOptions{PerPage: 100},
	}

	result, _, err := s.client.GetClient().Search.Code(ctx, searchQuery, opts)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to search files: %v", err),
			Type:    "error",
		}, nil
	}

	var files []map[string]interface{}
	for _, item := range result.CodeResults {
		fileInfo := map[string]interface{}{
			"name":       item.GetName(),
			"path":       item.GetPath(),
			"sha":        item.GetSHA(),
			"url":        item.GetHTMLURL(),
			"repository": item.Repository.GetFullName(),
		}
		files = append(files, fileInfo)
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"query":       query,
			"total_count": result.GetTotal(),
			"files":       files,
		},
		Type: "json",
	}, nil
}
