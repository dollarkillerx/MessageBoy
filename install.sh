#!/bin/bash
set -e

INSTALL_DIR="/opt/messageboy"
REPO="dollarkillerx/MessageBoy"
BINARY_NAME="messageboy-linux"

echo "下载 MessageBoy..."
DOWNLOAD_URL=$(curl -s https://fileoss.hacksnews.top/messageboy-linux | grep "browser_download_url.*${BINARY_NAME}" | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "获取下载地址失败"
    exit 1
fi

mkdir -p ${INSTALL_DIR}
curl -L -o ${INSTALL_DIR}/messageboy "$DOWNLOAD_URL"
chmod +x ${INSTALL_DIR}/messageboy

if [ ! -f ${INSTALL_DIR}/config.json ]; then
    ${INSTALL_DIR}/messageboy
fi

cat > /etc/systemd/system/messageboy.service << 'EOF'
[Unit]
Description=MessageBoy TCP Forwarder
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/messageboy
ExecStart=/opt/messageboy/messageboy config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

echo ""
echo "安装完成!"
echo "配置文件: ${INSTALL_DIR}/config.json"
echo ""
echo "启动命令:"
echo "  systemctl enable messageboy"
echo "  systemctl start messageboy"
