package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	githubcontext "github.com/qiniu/codeagent/internal/context"
	"github.com/qiniu/x/log"
)

// ContextAwareDirectoryProcessor handles the complete lifecycle of .codeagent directory processing
// including merging, GitHub context injection, template rendering, and workspace integration
type ContextAwareDirectoryProcessor struct {
	globalConfigPath string
	repositoryPath   string
	repoName         string
	githubEvent      *githubcontext.GitHubEvent
	contextInjector  *githubcontext.GitHubContextInjector

	// Internal components
	directoryMerger *DirectoryMerger
	commandLoader   *CommandLoader

	// Processed paths
	mergedPath    string
	processedPath string
	timestamp     string
}

// NewContextAwareDirectoryProcessor creates a new context-aware directory processor
func NewContextAwareDirectoryProcessor(globalConfigPath, repositoryPath, repoName string) *ContextAwareDirectoryProcessor {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	return &ContextAwareDirectoryProcessor{
		globalConfigPath: globalConfigPath,
		repositoryPath:   repositoryPath,
		repoName:         repoName,
		timestamp:        timestamp,
		contextInjector:  githubcontext.NewGitHubContextInjector(),
	}
}

// ProcessDirectories performs the complete .codeagent directory processing pipeline
// Step 1: Merge global and repository .codeagent directories
// Step 2: Apply GitHub context injection and template rendering
// Step 3: Generate unique processed directory
// Step 4: Return path for workspace integration
func (p *ContextAwareDirectoryProcessor) ProcessDirectories(githubEvent *githubcontext.GitHubEvent) error {
	p.githubEvent = githubEvent

	// Step 1: Merge .codeagent directories
	if err := p.mergeDirectories(); err != nil {
		return fmt.Errorf("failed to merge directories: %w", err)
	}

	// Step 2: Apply GitHub context rendering to create final processed directory
	if err := p.renderWithContext(); err != nil {
		return fmt.Errorf("failed to render with GitHub context: %w", err)
	}

	log.Infof("Successfully processed .codeagent directories - final path: %s", p.processedPath)
	return nil
}

// GetProcessedPath returns the final processed .codeagent directory path for workspace integration
func (p *ContextAwareDirectoryProcessor) GetProcessedPath() string {
	return p.processedPath
}

// GetCommandLoader returns a command loader configured to use the processed directory
func (p *ContextAwareDirectoryProcessor) GetCommandLoader() *CommandLoader {
	if p.commandLoader == nil {
		// Use the processed path as the primary source, with global as fallback
		p.commandLoader = NewCommandLoader(p.globalConfigPath, p.processedPath)
	}
	return p.commandLoader
}

// LoadCommand loads a command definition using the processed directory structure
func (p *ContextAwareDirectoryProcessor) LoadCommand(name string) (*CommandDefinition, error) {
	if p.processedPath == "" {
		return nil, fmt.Errorf("directories not processed yet, call ProcessDirectories first")
	}

	loader := p.GetCommandLoader()
	return loader.LoadCommand(name)
}

// Cleanup removes all temporary directories created during processing
func (p *ContextAwareDirectoryProcessor) Cleanup() error {
	var errors []string

	// Cleanup merged directory
	if p.directoryMerger != nil {
		if err := p.directoryMerger.Cleanup(); err != nil {
			errors = append(errors, fmt.Sprintf("merged directory cleanup failed: %v", err))
		}
	}

	// Cleanup processed directory
	if p.processedPath != "" {
		if err := os.RemoveAll(p.processedPath); err != nil {
			errors = append(errors, fmt.Sprintf("processed directory cleanup failed: %v", err))
		} else {
			log.Infof("Cleaned up processed directory: %s", p.processedPath)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// mergeDirectories handles the initial merge of global and repository .codeagent directories
func (p *ContextAwareDirectoryProcessor) mergeDirectories() error {
	// Create directory merger
	p.directoryMerger = NewDirectoryMerger(p.globalConfigPath, p.repositoryPath, p.repoName)

	// Perform merge
	if err := p.directoryMerger.MergeDirectories(); err != nil {
		return fmt.Errorf("directory merge failed: %w", err)
	}

	p.mergedPath = p.directoryMerger.GetMergedPath()
	log.Infof("Merged .codeagent directories to: %s", p.mergedPath)

	return nil
}

// renderWithContext applies GitHub context injection and template rendering to create the final directory
func (p *ContextAwareDirectoryProcessor) renderWithContext() error {
	if p.mergedPath == "" {
		return fmt.Errorf("merged directory not available")
	}

	if p.githubEvent == nil {
		return fmt.Errorf("GitHub event not provided")
	}

	// Create unique processed directory
	p.processedPath = filepath.Join(os.TempDir(), fmt.Sprintf("codeagent-processed-%s-%s", p.repoName, p.timestamp))

	if err := os.MkdirAll(p.processedPath, 0755); err != nil {
		return fmt.Errorf("failed to create processed directory %s: %w", p.processedPath, err)
	}

	// Copy merged directory to processed directory, applying context rendering to each file
	if err := p.renderDirectoryWithContext(p.mergedPath, p.processedPath); err != nil {
		return fmt.Errorf("failed to render directory with context: %w", err)
	}

	log.Infof("Applied GitHub context rendering to create processed directory: %s", p.processedPath)
	return nil
}

// renderDirectoryWithContext recursively copies and renders files with GitHub context injection
func (p *ContextAwareDirectoryProcessor) renderDirectoryWithContext(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(srcPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path and destination path
		relPath, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}

		// Skip root directory
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dstDir, relPath)

		if info.IsDir() {
			// Create directory
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
		} else {
			// Process and copy file with context injection
			if err := p.renderFileWithContext(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to render file %s: %w", srcPath, err)
			}
		}

		return nil
	})
}

// renderFileWithContext processes a single file, applying GitHub context injection to its content
func (p *ContextAwareDirectoryProcessor) renderFileWithContext(srcPath, dstPath string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Read source file
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Apply GitHub context injection to content
	renderedContent := p.contextInjector.InjectContext(string(content), p.githubEvent)

	// Write rendered content to destination
	if err := os.WriteFile(dstPath, []byte(renderedContent), 0644); err != nil {
		return fmt.Errorf("failed to write rendered file: %w", err)
	}

	log.Debugf("Rendered file with GitHub context: %s -> %s", srcPath, dstPath)
	return nil
}

// GetProcessingInfo returns detailed information about the processing pipeline
type ProcessingInfo struct {
	GlobalPath      string
	RepositoryPath  string
	MergedPath      string
	ProcessedPath   string
	RepoName        string
	Timestamp       string
	GitHubEventType string
	FilesProcessed  int
}

// GetProcessingInfo returns detailed information about the processing pipeline
func (p *ContextAwareDirectoryProcessor) GetProcessingInfo() *ProcessingInfo {
	info := &ProcessingInfo{
		GlobalPath:     p.globalConfigPath,
		RepositoryPath: p.repositoryPath,
		MergedPath:     p.mergedPath,
		ProcessedPath:  p.processedPath,
		RepoName:       p.repoName,
		Timestamp:      p.timestamp,
	}

	if p.githubEvent != nil {
		info.GitHubEventType = p.githubEvent.Type
	}

	// Count processed files
	if p.processedPath != "" {
		if count, err := p.countFiles(p.processedPath); err == nil {
			info.FilesProcessed = count
		}
	}

	return info
}

// countFiles recursively counts files in a directory
func (p *ContextAwareDirectoryProcessor) countFiles(dirPath string) (int, error) {
	count := 0
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}
