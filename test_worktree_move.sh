#!/bin/bash

# 测试新的 worktree 目录结构
# worktree 目录与仓库目录同级，避免污染

set -e

echo "=== 测试新的 worktree 目录结构 ==="

# 创建测试目录
TEST_DIR="/tmp/codeagent_test_worktree"
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "1. 创建测试组织结构"
mkdir -p qbox
cd qbox

echo "2. 克隆测试仓库"
git clone https://github.com/qbox/codeagent.git
cd codeagent

echo "3. 创建 worktree（与仓库同级）"
# 创建 Issue worktree（创建新分支）
git worktree add -b codeagent/issue-123 ../codeagent-issue-123-1703123456789 main
# 创建 PR worktree（创建新分支）
git worktree add -b codeagent/pr-456 ../codeagent-pr-456-1703123456790 main

echo "4. 查看目录结构"
echo "当前目录结构："
ls -la ../
echo ""
echo "仓库内的 .git 目录："
ls -la .git/
echo ""
echo "worktree 的 .git 文件："
cat ../codeagent-issue-123-1703123456789/.git
echo ""
cat ../codeagent-pr-456-1703123456790/.git

echo "5. 验证 worktree 列表"
git worktree list

echo "6. 测试映射文件"
# 创建映射文件
mkdir -p .git/info
cat > .git/info/exclude << EOF
# 原有的 exclude 规则
*.log
*.tmp

# CodeAgent Mappings
# Format: issue-{number}-{timestamp} -> PR-{number}
issue-123-1703123456789 -> PR-91
issue-124-1703123456790 -> PR-92
EOF

echo "映射文件内容："
cat .git/info/exclude

echo "7. 清理测试环境"
cd /
rm -rf "$TEST_DIR"

echo "=== 测试完成 ===" 