#!/bin/bash
set -e

INSTALL_DIR="/opt/messageboy"
SERVICE_NAME="messageboy-client"

# 默认值
SERVER_URL=""
TOKEN=""

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --server)
            SERVER_URL="$2"
            shift 2
            ;;
        --token)
            TOKEN="$2"
            shift 2
            ;;
        *)
            echo "未知参数: $1"
            echo "用法: $0 --server <server_url> --token <token>"
            exit 1
            ;;
    esac
done

# 检查必要参数
if [ -z "$SERVER_URL" ] || [ -z "$TOKEN" ]; then
    echo "错误: 必须提供 --server 和 --token 参数"
    echo "用法: $0 --server <server_url> --token <token>"
    echo ""
    echo "示例:"
    echo "  curl -sSL http://your-server:8080/install.sh | bash -s -- --server http://your-server:8080 --token your-token"
    exit 1
fi

DOWNLOAD_URL="https://fileoss.hacksnews.top/messageboy-client-linux-amd64"

echo "正在安装 MessageBoy Client..."
echo "服务器: $SERVER_URL"
echo ""

# 检查服务是否已存在，如果存在则停止并清理
if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
    echo "检测到服务正在运行，停止服务..."
    systemctl stop ${SERVICE_NAME}
fi

# 如果旧的二进制文件存在，删除它
if [ -f "${INSTALL_DIR}/messageboy-client" ]; then
    echo "删除旧版本..."
    rm -f ${INSTALL_DIR}/messageboy-client
fi

# 创建安装目录
mkdir -p ${INSTALL_DIR}

# 下载客户端
echo "下载客户端..."
curl -sSL -o ${INSTALL_DIR}/messageboy-client "$DOWNLOAD_URL"
chmod +x ${INSTALL_DIR}/messageboy-client

# 生成配置文件
echo "生成配置文件..."
cat > ${INSTALL_DIR}/client.toml << EOF
# MessageBoy Client 配置文件

[Client]
ServerURL = "${SERVER_URL}"
Token = "${TOKEN}"

[Connection]
ReconnectInterval = 5
MaxReconnectInterval = 60
HeartbeatInterval = 30

[Logging]
Level = "info"
File = ""

[Forwarder]
BufferSize = 32768
ConnectTimeout = 10
IdleTimeout = 300
EOF

# 创建 systemd 服务
echo "配置 systemd 服务..."
cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=MessageBoy Client
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/messageboy-client --config ${INSTALL_DIR}/client.toml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable ${SERVICE_NAME}
systemctl start ${SERVICE_NAME}

echo ""
echo "安装完成!"
echo ""
echo "配置文件: ${INSTALL_DIR}/client.toml"
echo ""
echo "查看状态:"
echo "  systemctl status ${SERVICE_NAME}"
echo "  journalctl -u ${SERVICE_NAME} -f"
