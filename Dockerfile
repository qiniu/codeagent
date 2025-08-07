FROM node:24-bookworm

# Add user
RUN groupadd -g 24368 codeagent && useradd -u 24368 -g 24368 -m codeagent

# Install Claude Code and Gemini CLI
RUN npm install -g @anthropic-ai/claude-code @google/gemini-cli

# Install toolkit
RUN apt-get update && apt-get install -y tree jq fd-find ripgrep git-lfs
RUN npm install -g @ast-grep/cli

# Install Go
RUN curl -fsSL https://go.dev/dl/go1.24.5.linux-amd64.tar.gz | tar -xz -C /usr/local
ENV PATH=/usr/local/go/bin:$PATH

# Install XGo
RUN echo "deb [trusted=yes] https://pkgs.xgo.dev/apt/ /" | tee /etc/apt/sources.list.d/goplus.list
RUN apt-get update && apt-get install -y xgo

# Install LLGo
RUN echo "deb http://apt.llvm.org/bookworm/ llvm-toolchain-bookworm-19 main" | tee /etc/apt/sources.list.d/llvm.list
RUN wget -O - https://apt.llvm.org/llvm-snapshot.gpg.key | apt-key add -
RUN apt-get update && apt-get install -y llvm-19-dev clang-19 libclang-19-dev lld-19 libunwind-19-dev libc++-19-dev pkg-config libgc-dev libssl-dev zlib1g-dev libcjson-dev libsqlite3-dev libuv1-dev python3.11-dev
RUN git clone https://github.com/goplus/llgo.git /tmp/llgo && cd /tmp/llgo && git checkout v0.11.5 && GOBIN=/usr/local/bin go install ./cmd/llgo && rm -rf /tmp/llgo

# Clean cache
RUN apt-get clean && rm -rf /var/lib/apt/lists/*

# Switch user
USER codeagent

# Set working directory
WORKDIR /workspace

# Default command
CMD ["tail", "-f", "/dev/null"]
