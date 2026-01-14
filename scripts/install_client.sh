#!/bin/bash
# MessageBoy Client 安装脚本
set -e

SERVER_URL=""
TOKEN=""
INSTALL_DIR="/opt/messageboy"

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
        --dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$SERVER_URL" ] || [ -z "$TOKEN" ]; then
    echo "Usage: $0 --server <server_url> --token <token> [--dir <install_dir>]"
    exit 1
fi

echo "Installing MessageBoy Client..."
echo "Server: $SERVER_URL"
echo "Install Dir: $INSTALL_DIR"

# 检测系统架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BINARY="messageboy-client-linux-amd64"
        ;;
    aarch64)
        BINARY="messageboy-client-linux-arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# 创建目录
mkdir -p ${INSTALL_DIR}

# 下载二进制文件
echo "Downloading ${BINARY}..."
curl -L -o ${INSTALL_DIR}/messageboy-client "${SERVER_URL}/download/${BINARY}"
chmod +x ${INSTALL_DIR}/messageboy-client

# 生成配置文件
cat > ${INSTALL_DIR}/client.toml << EOF
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
cat > /etc/systemd/system/messageboy-client.service << 'EOF'
[Unit]
Description=MessageBoy Client
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/messageboy
ExecStart=/opt/messageboy/messageboy-client --config /opt/messageboy/client.toml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 重载 systemd
systemctl daemon-reload

echo ""
echo "Installation completed!"
echo ""
echo "Commands:"
echo "  Start:   systemctl start messageboy-client"
echo "  Enable:  systemctl enable messageboy-client"
echo "  Status:  systemctl status messageboy-client"
echo "  Logs:    journalctl -u messageboy-client -f"
