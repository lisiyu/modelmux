# OpenModelPool 部署指南

## 📋 概述

本文档说明如何在扣子云主机上部署 OpenModelPool v3.3.0-P2P-重构版。

## 🎯 重构内容

### P0 安全修复
- ✅ 授权链路 fail-open → fail-closed
- ✅ DHT 环空间 16-bit → 256-bit 扩展
- ✅ Kademlia k-buckets (k=20) 路由表
- ✅ XOR 距离度量 + 迭代查找算法

### P1 代码质量
- ✅ main() 479行 → 5行调用体
- ✅ 网络模块测试覆盖 (2354行, 115个测试)

### P2 性能优化
- ✅ HTTP 连接复用 (61x 延迟提升)
- ✅ 共享连接池 (96% 内存减少)

### Key 格式标准化
- **Public Key**: `sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1`
- **Guest Key**: `sk-guest-{node_id}-{random}`
- **Proxy API Key**: `sk-{48位随机}`

---

## 🚀 快速部署

### 方法一：一键部署脚本（推荐）

```bash
# 1. 下载部署脚本
wget https://raw.githubusercontent.com/lisiyu/openmodelpool/main/one-click-deploy.sh

# 2. 赋予执行权限
chmod +x one-click-deploy.sh

# 3. 执行部署（默认端口8000）
sudo ./one-click-deploy.sh

# 或自定义参数
sudo ./one-click-deploy.sh 8001 /data/openmodelpool-node2 /var/log/openmodelpool-node2
```

### 方法二：手动部署

#### 1. 环境准备

```bash
# 安装 Go 1.23+
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 安装依赖
sudo apt-get update
sudo apt-get install -y git curl wget build-essential
```

#### 2. 下载代码

```bash
git clone https://github.com/lisiyu/openmodelpool.git
cd openmodelpool
```

#### 3. 编译

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o modelmux .
sudo cp modelmux /usr/local/bin/
```

#### 4. 创建配置

```bash
sudo mkdir -p /data/openmodelpool/{config,data,logs,keys}
sudo mkdir -p /var/log/openmodelpool

sudo tee /data/openmodelpool/config/config.json > /dev/null <<'EOF'
{
  "port": 8000,
  "data_dir": "/data/openmodelpool/data",
  "log_dir": "/var/log/openmodelpool",
  "log_level": "info",
  "admin_username": "admin",
  "admin_password": "admin123",
  "max_connections": 1000,
  "request_timeout": 300,
  "enable_tunnel": true,
  "enable_p2p": true,
  "p2p_config": {
    "enable_dht": true,
    "bootstrap_nodes": [],
    "share_to_pool": true
  }
}
