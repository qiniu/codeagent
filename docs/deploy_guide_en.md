# CodeAgent Deployment Guide

This tutorial will guide you through deploying the CodeAgent program on Ubuntu systems.

## System Requirements

- Ubuntu 18.04 or higher
- At least 2GB RAM
- At least 10GB available disk space
- Network connection

## 1. System Update

First, update system packages:

```bash
sudo apt update
sudo apt upgrade -y
```

## 2. Install Git

```bash
# Install Git
sudo apt install git -y

# Verify installation
git --version

# Configure Git (optional)
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
```

## 3. Install Docker

### 3.1 Remove old versions (if present)

```bash
sudo apt remove docker docker-engine docker.io containerd runc
```

### 3.2 Install dependency packages

```bash
sudo apt install apt-transport-https ca-certificates curl gnupg lsb-release -y
```

### 3.3 Add Docker's official GPG key

```bash
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
```

### 3.4 Set up Docker repository

```bash
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
```

### 3.5 Install Docker Engine

```bash
sudo apt update
sudo apt install docker-ce docker-ce-cli containerd.io -y
```

### 3.6 Start Docker service

```bash
sudo systemctl start docker
sudo systemctl enable docker
```

### 3.7 Add current user to docker group (avoid using sudo each time)

```bash
sudo usermod -aG docker $USER
# Re-login or execute the following command to make changes effective
newgrp docker
```

### 3.8 Verify Docker installation

```bash
docker --version
docker run hello-world
```

## 4. Install Go

### 4.1 Download Go

```bash
# Download latest version of Go (adjust according to latest version number)
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
```

### 4.2 Extract and install

```bash
# Remove old version (if exists)
sudo rm -rf /usr/local/go

# Extract to /usr/local
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
```

### 4.3 Configure environment variables

```bash
# Add to ~/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 4.4 Verify Go installation

```bash
go version
```

## 5. Clone CodeAgent Project

```bash
# Clone project
git clone https://github.com/your-username/codeagent.git
cd codeagent

# Or if project is in private repository
git clone git@github.com:your-username/codeagent.git
cd codeagent
```

## 6. Build Docker Images

### 6.1 Build Claude image

```bash
docker build -f Dockerfile.claude -t codeagent-claude:latest .
```

### 6.2 Build Gemini image

```bash
docker build -f Dockerfile.gemini -t codeagent-gemini:latest .
```

## 7. Configure CodeAgent

### 7.1 Copy configuration file

```bash
cp config.example.yaml config.yaml
```

### 7.2 Edit configuration file

```bash
nano config.yaml
```

Configuration example:

```yaml
# CodeAgent configuration file
server:
  port: 8080
  host: "0.0.0.0"

# GitHub configuration
github:
  token: "your-github-token"
  webhook_secret: "your-webhook-secret"

# AI service configuration
ai:
  # Claude configuration
  claude:
    enabled: true
    api_key: "your-claude-api-key"
    model: "claude-3-sonnet-20240229"

  # Gemini configuration
  gemini:
    enabled: true
    api_key: "your-gemini-api-key"
    model: "gemini-pro"

# Workspace directory configuration
workspace:
  # Important: Avoid using /tmp directory on macOS, may cause Docker mount issues
  # Recommended paths:
  # - macOS: /private/tmp/codeagent-workspace
  # - Linux: /var/tmp/codeagent-workspace
  # - Cross-platform: ~/tmp/codeagent-workspace
  base_path: "/private/tmp/codeagent-workspace" # Recommended for macOS
  cleanup_after_hours: 24
```

### 7.3 Set environment variables (optional)

```bash
cp env.example .env
nano .env
```

## 8. Run CodeAgent

### 8.1 Run directly

```bash
# Compile project
go build -o codeagent ./cmd/codeagent

# Run program
./codeagent
```

### 8.2 Run with Docker

```bash
# Run Claude version
docker run -d \
  --name codeagent-claude \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/workspace:/app/workspace \
  codeagent-claude:latest

# Run Gemini version
docker run -d \
  --name codeagent-gemini \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/workspace:/app/workspace \
  codeagent-gemini:latest
```

## 9. Verify Deployment

### 9.1 Check service status

```bash
# Check if program is running
ps aux | grep codeagent

# Check if port is listening
netstat -tlnp | grep 8080

# If using Docker
docker ps
```

### 9.2 Test API

```bash
# Test health check endpoint
curl http://localhost:8080/health

# Test GitHub webhook endpoint
curl -X POST http://localhost:8080/webhook/github \
  -H "Content-Type: application/json" \
  -d '{"test": "data"}'
```

## 10. Set up System Service (Optional)

### 10.1 Create systemd service file

```bash
sudo nano /etc/systemd/system/codeagent.service
```

Service file content:

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

### 10.2 Enable and start service

```bash
sudo systemctl daemon-reload
sudo systemctl enable codeagent
sudo systemctl start codeagent
sudo systemctl status codeagent
```

## 11. Configure Nginx Reverse Proxy (Optional)

### 11.1 Install Nginx

```bash
sudo apt install nginx -y
```

### 11.2 Configure Nginx

```bash
sudo nano /etc/nginx/sites-available/codeagent
```

Configuration content:

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

### 11.3 Enable site

```bash
sudo ln -s /etc/nginx/sites-available/codeagent /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## 12. Configure Firewall

```bash
# Allow SSH
sudo ufw allow ssh

# Allow HTTP/HTTPS
sudo ufw allow 80
sudo ufw allow 443

# If directly exposing CodeAgent port
sudo ufw allow 8080

# Enable firewall
sudo ufw enable
```

## 13. Monitoring and Logs

### 13.1 View logs

```bash
# If using systemd
sudo journalctl -u codeagent -f

# If running directly
tail -f /path/to/codeagent/logs/codeagent.log

# If using Docker
docker logs -f codeagent-claude
```

### 13.2 Set up log rotation

```bash
sudo nano /etc/logrotate.d/codeagent
```

Configuration content:

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

## 14. Troubleshooting

### 14.1 Common Issues

1. **Docker permission issues**

   ```bash
   sudo chmod 666 /var/run/docker.sock
   ```

2. **macOS /tmp directory mount issues**

   ```bash
   # Issue: /workspace directory is empty in container
   # Cause: macOS's /tmp is a symlink, Docker mount may have issues
   # Solution: Use /private/tmp or ~/tmp directory

   # Modify configuration file
   workspace:
     base_path: "/private/tmp/codeagent-workspace"  # Recommended
     # or
     base_path: "~/tmp/codeagent-workspace"         # Cross-platform
   ```

3. **Port occupied**

   ```bash
   sudo netstat -tlnp | grep 8080
   sudo kill -9 <PID>
   ```

4. **GitHub Token permission issues**

   - Ensure GitHub Token has sufficient permissions
   - Check if Token is expired

5. **AI API connection issues**
   - Check network connection
   - Verify API Key is correct
   - Check if API quota is exhausted

### 14.2 Debug mode

```bash
# Enable debug logging
export LOG_LEVEL=debug
./codeagent

# Or use Docker
docker run -it --rm \
  -e LOG_LEVEL=debug \
  -v $(pwd)/config.yaml:/app/config.yaml \
  codeagent-claude:latest
```

## 15. Backup and Recovery

### 15.1 Backup configuration

```bash
# Backup configuration files
cp config.yaml config.yaml.backup

# Backup workspace directory
tar -czf workspace-backup-$(date +%Y%m%d).tar.gz workspace/
```

### 15.2 Restore configuration

```bash
# Restore configuration files
cp config.yaml.backup config.yaml

# Restore workspace directory
tar -xzf workspace-backup-20231201.tar.gz
```

## 16. Updates and Maintenance

### 16.1 Update code

```bash
cd codeagent
git pull origin main
go build -o codeagent ./cmd/codeagent
sudo systemctl restart codeagent
```

### 16.2 Update Docker images

```bash
docker build -f Dockerfile.claude -t codeagent-claude:latest .
docker stop codeagent-claude
docker rm codeagent-claude
docker run -d --name codeagent-claude -p 8080:8080 codeagent-claude:latest
```

## Complete!

Your CodeAgent is now successfully deployed and running. You can access it through:

- Local access: http://localhost:8080
- Remote access: http://your-server-ip:8080
- Domain access: http://your-domain.com (if Nginx is configured)

Remember to regularly check logs and monitor system status to ensure the service is running properly.