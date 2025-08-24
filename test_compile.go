package main

import (
	"fmt"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/internal/config"
)

func main() {
	cfg := &config.Config{}
	manager := workspace.NewManager(cfg)
	fmt.Printf("Manager created successfully: %v\n", manager != nil)
}