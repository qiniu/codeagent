# 1. Background
CodeAgent is a tool that assists with code development on GitHub.

# 2. Process Flow
We can understand CodeAgent as a real "person" who, by understanding the descriptions/problems in GitHub Issues, creates Pull Requests, and then modifies code based on reviewers' suggestions until the Pull Request is merged.

1. Preparation Phase
    - Host machine prerequisites
        - claude-code (npm install -g @anthropic-ai/claude-code) 
        - gemini-cli (npm install -g @google/gemini-cli)
    - Complete claude-code and gemini-cli authentication (to obtain authentication files)

2. Startup Phase
    - Triggered by Issue Assignment or /code command
    - Prepare code:
        - Pull from RepoInfo in webhook, skip if already pulled
        - git worktree add ./issue-${id}-${timestamp} -b codeagent/issue-${id}-${timestamp} 
    - Start container 
        - Mount code directory as workspace -v `issue-${id}-${timestamp}`:/workspace
        - Mount authentication information:
            - gemini-cli auth info at: `~/.gemini:~/.gemini`
        - Mount tools -v /user/local/bin/gemini:/user/local/bin/gemini
      - Start gemini/claude (may be wrapped codeagent in the future)
    - Code layer exposes external interfaces
        - Input / Output interfaces
3. Interaction Phase
    - Interaction 1
        - Input: This is the Issue content ${issue}, organize a modification plan based on the Issue content
        - Output: Comment the output content as the first comment in the PR
    - Interaction 2: 
        - Input: Modify code according to issue content
        - After output completion, comment the output to GitHub in format: <details><summary>Session Name</summary>$output</details>
        - Submit commit & push
    - Review comment:
        - Input: issue comment        
        - After output completion, comment the output to GitHub: <details><summary>Session Name</summary>$output</details>
      - Submit commit & push
4. Completion Phase
    - Trigger action: merge/close PR
    - Clean up session (if possible)
    - Close container
    - git worktree del ./issue-${id}-${timestamp} 
    - Close issue
  
# 3. Specification Definition

1. Create a code object through a startup command
   
```golang
// Create PR first -> PR ID
// Create a code object
// 1. Start container using agent tools, need to implement gemini / claude
// code ,err := agent.Create(repo)

// Enter interactive mode, send messages through Prompt
res, err := code.Prompt(message)
out, err := io.ReadAll(res.out)

code, err := agent.Resume(workspace)
// Receive review comment, find code through PR ID
res, err := code.Prompt(comment)

// End:
// Close PR
// Call code.Close()
// 1. Clean up session (if possible)
// 2. Close container
// 3. Clean up workspace: git worktree del ./issue-${id}-${timestamp}
// code.Close()
// Close issue
```