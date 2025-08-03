# CodeAgent 部署指导教程

本教程将指导您在 Ubuntu 系统上部署 CodeAgent 程序。

## 系统要求

- Ubuntu 18.04 或更高版本
- 至少 2GB RAM
- 至少 10GB 可用磁盘空间
- 网络连接

## 1. 系统更新

首先更新系统包：

```bash
sudo apt update
sudo apt upgrade -y
```

## 2. 安装 Git

```bash
# 安装 Git
sudo apt install git -y

# 验证安装
git --version

# 配置 Git（可选）
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
```

## 3. 安装 Docker

### 3.1 卸载旧版本（如果存在）

```bash
sudo apt remove docker docker-engine docker.io containerd runc
```

### 3.2 安装依赖包

```bash
sudo apt install apt-transport-https ca-certificates curl gnupg lsb-release -y
```

### 3.3 添加 Docker 官方 GPG 密钥

```bash
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
```

### 3.4 设置 Docker 仓库

```bash
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
```

### 3.5 安装 Docker Engine

```bash
sudo apt update
sudo apt install docker-ce docker-ce-cli containerd.io -y
```

### 3.6 启动 Docker 服务

```bash
sudo systemctl start docker
sudo systemctl enable docker
```

### 3.7 将当前用户添加到 docker 组（避免每次使用 sudo）

```bash
sudo usermod -aG docker $USER
# 重新登录或执行以下命令使更改生效
newgrp docker
```

### 3.8 验证 Docker 安装

```bash
docker --version
docker run hello-world
```

## 4. 安装 Go

### 4.1 下载 Go

```bash
# 下载最新版本的 Go（请根据最新版本号调整）
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
```

### 4.2 解压并安装

```bash
# 删除旧版本（如果存在）
sudo rm -rf /usr/local/go

# 解压到 /usr/local
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
```

### 4.3 配置环境变量

```bash
# 添加到 ~/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 4.4 验证 Go 安装

```bash
go version
```

## 5. 克隆 CodeAgent 项目

```bash
# 克隆项目
git clone https://github.com/your-username/codeagent.git
cd codeagent

# 或者如果项目在私有仓库
git clone git@github.com:your-username/codeagent.git
cd codeagent
```

## 6. 构建 Docker 镜像

### 6.1 构建 Claude 镜像

```bash
docker build -f Dockerfile.claude -t codeagent-claude:latest .
```

### 6.2 构建 Gemini 镜像

```bash
docker build -f Dockerfile.gemini -t codeagent-gemini:latest .
```

## 7. 配置 CodeAgent

### 7.1 复制配置文件

```bash
cp config.example.yaml config.yaml
```

### 7.2 编辑配置文件

```bash
nano config.yaml
```

配置示例：

```yaml
# CodeAgent 配置文件
server:
  port: 8080
  host: "0.0.0.0"

# GitHub 配置
github:
  token: "your-github-token"
  webhook_secret: "your-webhook-secret"

# AI 服务配置
ai:
  # Claude 配置
  claude:
    enabled: true
    api_key: "your-claude-api-key"
    model: "claude-3-sonnet-20240229"

  # Gemini 配置
  gemini:
    enabled: true
    api_key: "your-gemini-api-key"
    model: "gemini-pro"

# 工作目录配置
workspace:
  # 重要：在macOS上避免使用 /tmp 目录，可能导致Docker挂载问题
  # 推荐路径：
  # - macOS: /private/tmp/codeagent-workspace
  # - Linux: /var/tmp/codeagent-workspace
  # - 跨平台: ~/tmp/codeagent-workspace
  base_path: "/private/tmp/codeagent-workspace" # 推荐用于macOS
  cleanup_after_hours: 24
```

### 7.3 设置环境变量（可选）

```bash
cp env.example .env
nano .env
```

## 8. 运行 CodeAgent

### 8.1 直接运行

```bash
# 编译项目
go build -o codeagent ./cmd/codeagent

# 运行程序
./codeagent
```

### 8.2 使用 Docker 运行

```bash
# 运行 Claude 版本
docker run -d \
  --name codeagent-claude \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/workspace:/app/workspace \
  codeagent-claude:latest

# 运行 Gemini 版本
docker run -d \
  --name codeagent-gemini \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/workspace:/app/workspace \
  codeagent-gemini:latest
```

## 9. 验证部署

### 9.1 检查服务状态

```bash
# 检查程序是否运行
ps aux | grep codeagent

# 检查端口是否监听
netstat -tlnp | grep 8080

# 如果使用 Docker
docker ps
```

### 9.2 测试 API

```bash
# 测试健康检查端点
curl http://localhost:8080/health

# 测试 GitHub webhook 端点
curl -X POST http://localhost:8080/webhook/github \
  -H "Content-Type: application/json" \
  -d '{"test": "data"}'
```

## 10. 设置系统服务（可选）

### 10.1 创建 systemd 服务文件

```bash
sudo nano /etc/systemd/system/codeagent.service
```

服务文件内容：

```ini
[Unit]
Description=CodeAgent Service
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/path/to/codeagent
ExecStart=/path/to/codeagent/codeagent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 10.2 启用并启动服务

```bash
sudo systemctl daemon-reload
sudo systemctl enable codeagent
sudo systemctl start codeagent
sudo systemctl status codeagent
```

## 11. 配置 Nginx 反向代理（可选）

### 11.1 安装 Nginx

```bash
sudo apt install nginx -y
```

### 11.2 配置 Nginx

```bash
sudo nano /etc/nginx/sites-available/codeagent
```

配置内容：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 11.3 启用站点

```bash
sudo ln -s /etc/nginx/sites-available/codeagent /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## 12. 配置防火墙

```bash
# 允许 SSH
sudo ufw allow ssh

# 允许 HTTP/HTTPS
sudo ufw allow 80
sudo ufw allow 443

# 如果直接暴露 CodeAgent 端口
sudo ufw allow 8080

# 启用防火墙
sudo ufw enable
```

## 13. 监控和日志

### 13.1 查看日志

```bash
# 如果使用 systemd
sudo journalctl -u codeagent -f

# 如果直接运行
tail -f /path/to/codeagent/logs/codeagent.log

# 如果使用 Docker
docker logs -f codeagent-claude
```

### 13.2 设置日志轮转

```bash
sudo nano /etc/logrotate.d/codeagent
```

配置内容：

```
/path/to/codeagent/logs/*.log {
    daily
    missingok
    rotate 7
    compress
    delaycompress
    notifempty
    create 644 your-username your-username
}
```

## 14. 故障排除

### 14.1 常见问题

1. **Docker 权限问题**

   ```bash
   sudo chmod 666 /var/run/docker.sock
   ```

2. **macOS /tmp 目录挂载问题**

   ```bash
   # 问题：容器内 /workspace 目录为空
   # 原因：macOS的 /tmp 是符号链接，Docker挂载可能有问题
   # 解决：使用 /private/tmp 或 ~/tmp 目录

   # 修改配置文件
   workspace:
     base_path: "/private/tmp/codeagent-workspace"  # 推荐
     # 或者
     base_path: "~/tmp/codeagent-workspace"         # 跨平台
   ```

3. **端口被占用**

   ```bash
   sudo netstat -tlnp | grep 8080
   sudo kill -9 <PID>
   ```

4. **GitHub Token 权限问题**

   - 确保 GitHub Token 有足够的权限
   - 检查 Token 是否过期

5. **AI API 连接问题**
   - 检查网络连接
   - 验证 API Key 是否正确
   - 检查 API 配额是否用完

### 14.2 调试模式

```bash
# 启用调试日志
export LOG_LEVEL=debug
./codeagent

# 或者使用 Docker
docker run -it --rm \
  -e LOG_LEVEL=debug \
  -v $(pwd)/config.yaml:/app/config.yaml \
  codeagent-claude:latest
```

## 15. 备份和恢复

### 15.1 备份配置

```bash
# 备份配置文件
cp config.yaml config.yaml.backup

# 备份工作目录
tar -czf workspace-backup-$(date +%Y%m%d).tar.gz workspace/
```

### 15.2 恢复配置

```bash
# 恢复配置文件
cp config.yaml.backup config.yaml

# 恢复工作目录
tar -xzf workspace-backup-20231201.tar.gz
```

## 16. 更新和维护

### 16.1 更新代码

```bash
cd codeagent
git pull origin main
go build -o codeagent ./cmd/codeagent
sudo systemctl restart codeagent
```

### 16.2 更新 Docker 镜像

```bash
docker build -f Dockerfile.claude -t codeagent-claude:latest .
docker stop codeagent-claude
docker rm codeagent-claude
docker run -d --name codeagent-claude -p 8080:8080 codeagent-claude:latest
```

## 完成！

现在您的 CodeAgent 已经成功部署并运行。您可以通过以下方式访问：

- 本地访问：http://localhost:8080
- 远程访问：http://your-server-ip:8080
- 域名访问：http://your-domain.com（如果配置了 Nginx）

记得定期检查日志和监控系统状态，确保服务正常运行。
