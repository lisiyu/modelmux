#!/bin/bash
# OpenModelPool 一键部署脚本
# 版本: v3.3.0-P2P-重构版
# 日期: 2026-07-09
# 用途: 在扣子云主机上部署OpenModelPool节点

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 默认配置
DEFAULT_PORT=8000
DEFAULT_DATA_DIR="/data/openmodelpool"
DEFAULT_LOG_DIR="/var/log/openmodelpool"
GITHUB_REPO="https://github.com/lisiyu/openmodelpool.git"
MIN_GO_VERSION="1.23.0"

# 解析命令行参数
NODE_PORT=${1:-$DEFAULT_PORT}
DATA_DIR=${2:-$DEFAULT_DATA_DIR}
LOG_DIR=${3:-$DEFAULT_LOG_DIR}

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  OpenModelPool 一键部署脚本${NC}"
echo -e "${GREEN}  v3.3.0-P2P-重构版${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}配置参数:${NC}"
echo "  端口: $NODE_PORT"
echo "  数据目录: $DATA_DIR"
echo "  日志目录: $LOG_DIR"
echo ""

# 1. 检查系统要求
echo -e "${YELLOW}[1/7] 检查系统环境...${NC}"
check_system() {
    # 检查是否root
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}错误: 需要root权限运行此脚本${NC}"
        exit 1
    fi

    # 检查操作系统
    if [ ! -f /etc/os-release ]; then
        echo -e "${RED}错误: 无法识别操作系统${NC}"
        exit 1
    fi

    # 检查Go版本
    if ! command -v go &> /dev/null; then
        echo -e "${RED}错误: Go未安装${NC}"
        echo "请安装 Go ${MIN_GO_VERSION} 或更高版本: https://golang.org/dl/"
        exit 1
    fi

    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo "  Go版本: $GO_VERSION"
    
    # 简单版本比较
    REQUIRED_MAJOR=$(echo $MIN_GO_VERSION | cut -d. -f1)
    REQUIRED_MINOR=$(echo $MIN_GO_VERSION | cut -d. -f2)
    CURRENT_MAJOR=$(echo $GO_VERSION | cut -d. -f1)
    CURRENT_MINOR=$(echo $GO_VERSION | cut -d. -f2)
    
    if [ "$CURRENT_MAJOR" -lt "$REQUIRED_MAJOR" ] || \
       ([ "$CURRENT_MAJOR" -eq "$REQUIRED_MAJOR" ] && [ "$CURRENT_MINOR" -lt "$REQUIRED_MINOR" ]); then
        echo -e "${RED}错误: Go版本过低，需要 ${MIN_GO_VERSION} 或更高${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ 系统环境检查通过${NC}"
}

# 2. 安装依赖
echo -e "${YELLOW}[2/7] 安装系统依赖...${NC}"
install_dependencies() {
    if command -v apt-get &> /dev/null; then
        # Ubuntu/Debian
        apt-get update -qq
        apt-get install -y -qq git curl wget build-essential
    elif command -v yum &> /dev/null; then
        # CentOS/RHEL
        yum install -y -q git curl wget gcc gcc-c++ make
    else
        echo -e "${YELLOW}警告: 无法自动安装依赖，请手动安装: git curl wget build-essential${NC}"
    fi
    echo -e "${GREEN}✓ 依赖安装完成${NC}"
}

# 3. 下载代码
echo -e "${YELLOW}[3/7] 下载代码...${NC}"
CODE_DIR="/opt/openmodelpool"
download_code() {
    if [ -d "$CODE_DIR" ]; then
        echo "  代码目录已存在，更新中..."
        cd "$CODE_DIR"
        git fetch origin
        git reset --hard origin/main
    else
        echo "  克隆代码到 $CODE_DIR..."
        git clone "$GITHUB_REPO" "$CODE_DIR"
        cd "$CODE_DIR"
    fi
    echo -e "${GREEN}✓ 代码下载完成${NC}"
}

# 4. 编译代码
echo -e "${YELLOW}[4/7] 编译代码...${NC}"
build_code() {
    cd "$CODE_DIR"
    
    # 清理旧构建
    rm -f modelmux
    
    # 编译
    echo "  编译中..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o modelmux .
    
    if [ ! -f "modelmux" ]; then
        echo -e "${RED}错误: 编译失败${NC}"
        exit 1
    fi
    
    # 移动到系统路径
    cp modelmux /usr/local/bin/
    chmod +x /usr/local/bin/modelmux
    
    echo -e "${GREEN}✓ 编译完成${NC}"
}

# 5. 创建目录和配置
echo -e "${YELLOW}[5/7] 创建目录和配置...${NC}"
setup_directories() {
    # 创建数据目录
    mkdir -p "$DATA_DIR"/{config,data,logs,keys}
    mkdir -p "$LOG_DIR"
    
    # 生成配置文件
    CONFIG_FILE="$DATA_DIR/config/config.json"
    if [ ! -f "$CONFIG_FILE" ]; then
        cat > "$CONFIG_FILE" <<EOFCONFIG
{
  "port": $NODE_PORT,
  "data_dir": "$DATA_DIR/data",
  "log_dir": "$LOG_DIR",
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
EOFCONFIG
        echo "  生成配置文件: $CONFIG_FILE"
    fi
    
    # 设置权限
    chmod -R 755 "$DATA_DIR"
    chmod -R 755 "$LOG_DIR"
    
    echo -e "${GREEN}✓ 目录和配置创建完成${NC}"
}

# 6. 创建systemd服务
echo -e "${YELLOW}[6/7] 创建系统服务...${NC}"
create_service() {
    SERVICE_FILE="/etc/systemd/system/openmodelpool.service"
    
    cat > "$SERVICE_FILE" <<EOFSERVICE
[Unit]
Description=OpenModelPool P2P AI Model Proxy
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$CODE_DIR
ExecStart=/usr/local/bin/modelmux --config $DATA_DIR/config/config.json
Restart=always
RestartSec=10
StandardOutput=append:$LOG_DIR/modelmux.log
StandardError=append:$LOG_DIR/modelmux-error.log

# 安全增强
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR

[Install]
WantedBy=multi-user.target
EOFSERVICE
    
    # 重新加载systemd
    systemctl daemon-reload
    systemctl enable openmodelpool
    
    echo -e "${GREEN}✓ 系统服务创建完成${NC}"
}

# 7. 启动服务
echo -e "${YELLOW}[7/7] 启动服务...${NC}"
start_service() {
    systemctl start openmodelpool
    sleep 3
    
    # 检查服务状态
    if systemctl is-active --quiet openmodelpool; then
        echo -e "${GREEN}✓ 服务启动成功${NC}"
    else
        echo -e "${RED}错误: 服务启动失败${NC}"
        systemctl status openmodelpool --no-pager
        exit 1
    fi
}

# 8. 健康检查
health_check() {
    echo ""
    echo -e "${YELLOW}健康检查...${NC}"
    
    # 等待服务就绪
    sleep 5
    
    # 检查端口
    if netstat -tuln | grep -q ":$NODE_PORT "; then
        echo -e "${GREEN}✓ 端口 $NODE_PORT 监听正常${NC}"
    else
        echo -e "${RED}警告: 端口 $NODE_PORT 未监听${NC}"
    fi
    
    # 检查HTTP
    if curl -s -f "http://localhost:$NODE_PORT/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ HTTP健康检查通过${NC}"
    else
        echo -e "${YELLOW}警告: HTTP健康检查失败（服务可能仍在初始化）${NC}"
    fi
}

# 9. 输出信息
print_info() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  部署完成!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${YELLOW}访问信息:${NC}"
    echo "  管理后台: http://$(hostname -I | awk '{print $1}'):$NODE_PORT"
    echo "  API端点: http://$(hostname -I | awk '{print $1}'):$NODE_PORT/v1"
    echo ""
    echo -e "${YELLOW}登录凭据:${NC}"
    echo "  用户名: admin"
    echo "  密码: admin123"
    echo -e "  ${RED}⚠ 请立即修改默认密码!${NC}"
    echo ""
    echo -e "${YELLOW}管理命令:${NC}"
    echo "  查看状态: systemctl status openmodelpool"
    echo "  查看日志: tail -f $LOG_DIR/modelmux.log"
    echo "  重启服务: systemctl restart openmodelpool"
    echo "  停止服务: systemctl stop openmodelpool"
    echo ""
    echo -e "${YELLOW}重要提示:${NC}"
    echo "  1. 请确保防火墙/安全组开放端口: TCP $NODE_PORT"
    echo "  2. 首次登录后立即修改admin密码"
    echo "  3. 配置AI模型提供商API Keys"
    echo "  4. 查看文档: $CODE_DIR/docs/"
    echo ""
}

# 执行所有步骤
check_system
install_dependencies
download_code
build_code
setup_directories
create_service
start_service
health_check
print_info

exit 0
