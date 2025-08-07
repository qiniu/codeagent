# I. Background
CodeAgent is a tool that assists with code development on GitHub.

# II. Process Flow
We can understand CodeAgent as a real "person" who, by understanding the descriptions/problems in GitHub Issues, creates Pull Requests themselves, then modifies code based on Reviewer suggestions until the Pull Request is merged.

1. Preparation Phase
    - Host machine pre-installation
        - claude-code (npm install -g @anthropic-ai/claude-code) 
        - gemini-cli (npm install -g @google/gemini-cli)
    - Complete claude-code, gemini-cli authentication (to obtain authentication files)

2. Startup Phase
    - Receive Issue Assign or command /code
    - Prepare code:
        - Pull via RepoInfo in webhook, ignore if already pulled
        - git worktree add ./issue-${id}-${timestamp} -b codeagent/issue-${id}-${timestamp} 
    - Start image 
        - Mount code directory as workspace -v `issue-${id}-${timestamp}`:/workspace
        - Mount authentication information:
            - gemini-cli authentication information is at: `~/.gemini:~/.gemini`
        - Mount tools -v /user/local/bin/gemini:/user/local/bin/gemini
      - Start gemini/claude (future may be wrapped codeagent)
    - Code layer external exposure 
        - Input / Output interfaces
3. Dialogue Phase
    - Dialogue 1
        - Input: This is Issue content ${issue}, organize a modification plan based on the Issue content
        - Output: Comment the output content to the first comment of the PR
    - Dialogue 2: 
        - Input: Modify code according to issue content
        - After output completion, comment the output to github via comment format: <details><summary>Session Name</summary>$output</details>
        - Submit commit & push
    - review comment:
        - Input: issue comment ;        
        - After output completion, comment the output to github via comment format: <details><summary>Session Name</summary>$output</details>
      - Submit commit & push
4. End Phase
    - Trigger action: merge/close PR
    - Clean up session (if possible)
    - Close container
    - git worktree del ./issue-${id}-${timestamp} 
    - Close issue
  
# III. Specification Definition

1. Create a code object through a startup command
   
```golang
// First create pr -> pr id
// Create a code object
// 1. Start image use agent tool, need to implement gemini / claude
// code ,err := agent.Create(repo)

// Enter interactive mode, send messages through Prompt
res, err := code.Prompt(message)
out, err := io.ReadAll(res.out)

code, err := agent.Resume(workspace)
// Receive review comment, find code through pr id
res, err := code.Prompt(comment)

// End:
// Close pr
// Call code.Close()
// 1. Clean up session (if possible)
// 2. Close container
// 3. Clean up workspace: git worktree del ./issue-${id}-${timestamp}
// code.Close()
// Close issue
```
