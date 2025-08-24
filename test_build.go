// Simple test to verify the code compiles
package main

import (
	"fmt"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/internal/config"
)

func main() {
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			BaseDir: "/tmp/test",
		},
	}
	
	manager := workspace.NewManager(cfg)
	fmt.Printf("Workspace manager created successfully: %v\n", manager != nil)
}