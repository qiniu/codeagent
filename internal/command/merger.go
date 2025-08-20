package command

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/qiniu/x/log"
)

// DirectoryMerger handles physical merging of .codeagent directories
type DirectoryMerger struct {
	globalPath     string
	repositoryPath string
	mergedPath     string
	timestamp      string
}

// NewDirectoryMerger creates a new directory merger
func NewDirectoryMerger(globalPath, repositoryPath, repoName, baseDir string) *DirectoryMerger {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	// Use workspace baseDir instead of temp dir to avoid macOS Docker mount issues
	mergedPath := filepath.Join(baseDir, fmt.Sprintf(".codeagent-merged-%s-%s", repoName, timestamp))

	return &DirectoryMerger{
		globalPath:     globalPath,
		repositoryPath: repositoryPath,
		mergedPath:     mergedPath,
		timestamp:      timestamp,
	}
}

// MergeDirectories performs physical merge of global and repository .codeagent directories
// Repository configs override global configs for same-named files
func (dm *DirectoryMerger) MergeDirectories() error {
	log.Infof("Merging directories: global=%s, repo=%s, merged=%s",
		dm.globalPath, dm.repositoryPath, dm.mergedPath)

	// Create temporary merged directory
	if err := os.MkdirAll(dm.mergedPath, 0755); err != nil {
		return fmt.Errorf("failed to create merged directory %s: %w", dm.mergedPath, err)
	}

	// Step 1: Copy global configuration first
	if dm.globalPath != "" && dm.pathExists(dm.globalPath) {
		if err := dm.copyDirectory(dm.globalPath, dm.mergedPath); err != nil {
			return fmt.Errorf("failed to copy global directory: %w", err)
		}
		log.Infof("Copied global configs from %s", dm.globalPath)
	} else {
		log.Warnf("Global path does not exist or is empty: %s", dm.globalPath)
	}

	// Step 2: Copy repository configuration (overrides global)
	if dm.repositoryPath != "" && dm.pathExists(dm.repositoryPath) {
		if err := dm.copyDirectory(dm.repositoryPath, dm.mergedPath); err != nil {
			return fmt.Errorf("failed to copy repository directory: %w", err)
		}
		log.Infof("Copied repository configs from %s (overriding global)", dm.repositoryPath)
	} else {
		log.Infof("Repository path does not exist: %s", dm.repositoryPath)
	}

	return nil
}

// GetMergedPath returns the path to the merged directory
func (dm *DirectoryMerger) GetMergedPath() string {
	return dm.mergedPath
}

// GetCommandsPath returns the commands subdirectory in merged path
func (dm *DirectoryMerger) GetCommandsPath() string {
	return filepath.Join(dm.mergedPath, "commands")
}

// GetAgentsPath returns the agents subdirectory in merged path
func (dm *DirectoryMerger) GetAgentsPath() string {
	return filepath.Join(dm.mergedPath, "agents")
}

// Cleanup removes the temporary merged directory
func (dm *DirectoryMerger) Cleanup() error {
	if dm.mergedPath == "" {
		return nil
	}

	if err := os.RemoveAll(dm.mergedPath); err != nil {
		log.Warnf("Failed to cleanup merged directory %s: %v", dm.mergedPath, err)
		return err
	}

	log.Infof("Cleaned up merged directory %s", dm.mergedPath)
	return nil
}

// pathExists checks if a path exists
func (dm *DirectoryMerger) pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// copyDirectory recursively copies a directory tree
func (dm *DirectoryMerger) copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from source
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip if it's the root directory
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// Create directory
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := dm.copyFile(path, dstPath); err != nil {
				return err
			}
		}

		return nil
	})
}

// copyFile copies a single file
func (dm *DirectoryMerger) copyFile(src, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}

// GetMergeInfo returns information about the merge process
type MergeInfo struct {
	GlobalPath     string
	RepositoryPath string
	MergedPath     string
	Timestamp      string
	CommandsCount  int
	AgentsCount    int
}

// GetMergeInfo returns detailed information about the merged directory
func (dm *DirectoryMerger) GetMergeInfo() (*MergeInfo, error) {
	info := &MergeInfo{
		GlobalPath:     dm.globalPath,
		RepositoryPath: dm.repositoryPath,
		MergedPath:     dm.mergedPath,
		Timestamp:      dm.timestamp,
	}

	// Count commands
	commandsPath := dm.GetCommandsPath()
	if entries, err := os.ReadDir(commandsPath); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
				info.CommandsCount++
			}
		}
	}

	// Count agents
	agentsPath := dm.GetAgentsPath()
	if entries, err := os.ReadDir(agentsPath); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
				info.AgentsCount++
			}
		}
	}

	return info, nil
}

// ValidateMergedDirectory validates that the merged directory has the expected structure
func (dm *DirectoryMerger) ValidateMergedDirectory() error {
	// Check if merged directory exists
	if !dm.pathExists(dm.mergedPath) {
		return fmt.Errorf("merged directory does not exist: %s", dm.mergedPath)
	}

	// Check required subdirectories
	requiredDirs := []string{"commands", "agents"}
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(dm.mergedPath, dir)
		if !dm.pathExists(dirPath) {
			return fmt.Errorf("required subdirectory does not exist: %s", dirPath)
		}
	}

	return nil
}
