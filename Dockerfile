# Build stage for codeagent-github-mcp-server
FROM golang:1.24-bookworm AS mcp-builder

# 设置工作目录
WORKDIR /src

# 复制 Go 模块文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建 codeagent-github-mcp-server 二进制文件
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o codeagent-github-mcp-server ./cmd/codeagent-github-mcp-server

# Main stage
FROM node:24-bookworm

# 添加用户
RUN groupadd -g 24368 codeagent && useradd -u 24368 -g 24368 -m codeagent

# 安装 Claude Code 和 Gemini CLI
# NOTE(CarlJi): 固定Claude code 版本，防止上游更新导致不兼容
RUN npm install -g @anthropic-ai/claude-code@1.0.85 @google/gemini-cli

# 安装 toolkit
RUN apt-get update && apt-get install -y tree jq fd-find ripgrep git-lfs
RUN npm install -g @ast-grep/cli

# 安装 Go
RUN curl -fsSL https://go.dev/dl/go1.24.5.linux-amd64.tar.gz | tar -xz -C /usr/local
ENV PATH=/usr/local/go/bin:$PATH

# 安装 XGo
RUN echo "deb [trusted=yes] https://pkgs.xgo.dev/apt/ /" | tee /etc/apt/sources.list.d/goplus.list
RUN apt-get update && apt-get install -y xgo

# 安装 LLGo
RUN echo "deb http://apt.llvm.org/bookworm/ llvm-toolchain-bookworm-19 main" | tee /etc/apt/sources.list.d/llvm.list
RUN wget -O - https://apt.llvm.org/llvm-snapshot.gpg.key | apt-key add -
RUN apt-get update && apt-get install -y llvm-19-dev clang-19 libclang-19-dev lld-19 libunwind-19-dev libc++-19-dev pkg-config libgc-dev libssl-dev zlib1g-dev libcjson-dev libsqlite3-dev libuv1-dev python3.11-dev
RUN git clone https://github.com/goplus/llgo.git /tmp/llgo && cd /tmp/llgo && git checkout v0.11.5 && GOBIN=/usr/local/bin go install ./cmd/llgo && rm -rf /tmp/llgo

# 安装 GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh

# 清理缓存
RUN apt-get clean && rm -rf /var/lib/apt/lists/*


# 从构建阶段复制 codeagent-github-mcp-server 二进制文件
COPY --from=mcp-builder /src/codeagent-github-mcp-server /usr/local/bin/codeagent-github-mcp-server
RUN chmod +x /usr/local/bin/codeagent-github-mcp-server


# 切换用户
USER codeagent

# 设置工作目录
WORKDIR /workspace

# 配置Git safe.directory以解决挂载目录权限问题
RUN git config --global --add safe.directory /workspace

# 默认命令
CMD ["tail", "-f", "/dev/null"]
