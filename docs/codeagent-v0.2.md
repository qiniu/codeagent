# 一、背景
CodeAgent 是一个辅助 Github 进行代码开发的工具。

# 二、串流程
我们可以理解 CodeAgent 就是一个真实的“人”，通过理解 Github Issue 中的描述/问题，自己提 Pull Request ，然后根据 Reviewer 提的建议修改代码，直到 Pull Request 被 Merge。

1. 准备阶段
    - 宿主机预装
        - claude-code (npm install -g @anthropic-ai/claude-code) 
        - gemini-cli (npm install -g @google/gemini-cli)
    - 完成 claude-code，gemini-cli 认证（为了拿到认证文件）

2. 启动阶段
    - 收到 Issue Assign 或者命令 /code
    - 准备代码：
        - 通过 webhook 中的 RepoInfo 拉取，如果拉去过，忽略
        - git worktree add ./issue-${id}-${timestamp} -b codeagent/issue-${id}-${timestamp} 
    - 启动镜像 
        - 挂载代码目录做为 workspace -v `issue-${id}-${timestamp}`:/workspace
        - 挂载认证信息：
            - gemini-cli 认证信息在：`~/.gemini:~/.gemini`
        - 挂载工具 -v /user/local/bin/gemini:/user/local/bin/gemini
      - 启动 gemini/claude （未来可能是 wrap 后的 codeagent）
    - 代码层面对外暴露 
        - Input / Output 接口
3. 对话阶段
    - 对话1
        - 输入：这是 Issue 内容 ${issue} ，根据 Issue 内容，整理出修改计划
        - 输出：将输出的内容评论到 pr 第一个 comment
    - 对话2: 
        - 输入：按 issue 内容修改代码
        - 输出完成后，把 output 通过 comment 的方式评论到 github，格式：<details><summary>Session Name</summary>$output</details>
        - 提交 commit & push
    - review comment：
        - 输入： issue comment ;        
        - 输出完成后，把 output 通过 comment 的方式评论到 github：通过  <details><summary>Session Name</summary>$output</details>
      - 提交 commit & push
4. 结束阶段
    - 触发动作：合并/关闭 pr
    - 清理会话（如果可以）
    - 关闭容器
    - git worktree del ./issue-${id}-${timestamp} 
    - 关闭 issue
  
# 三、规格定义

1. 通过一个启动命令，创建一个 code 对象
   
```golang
// 先创建 pr -> pr id
// 创建一个 code 对象
// 1. 启动镜像 use agent 工具，需要实现 gemini / claude
// code ,err := agent.Create(repo)

// 进入交互模式，通过 Prompt 发送消息
res, err := code.Prompt(message)
out, err := io.ReadAll(res.out)

code, err := agent.Resume(workspace)
// 收到 review comment, 通过 pr id 找到 code
res, err := code.Prompt(comment)

// 结束：
// 关闭 pr
// 调用 code.Close()
// 1. 清理会话（如果可以）
// 2. 关闭容器
// 3. 清理工作区：git worktree del ./issue-${id}-${timestamp}
// code.Close()
// 关闭 issue
```
