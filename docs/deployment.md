# 🚀 Bare Metal Deployment Guide

This guide describes how to deploy a **SOLE Production Node** on a Linux VPS (Ubuntu/Debian) without Docker. We use **Systemd** for process management and **Nginx** for SSL/HTTPS termination.

---

## 📋 Prerequisites

*   **VPS**: Minimum 2GB RAM (BadgerDB requirement), Ubuntu 22.04 LTS recommended.
*   **Domain**: A valid domain (e.g., `sole.nicolocarcagni.dev`) pointing to your VPS IP.
*   **Open Ports (UFW)**:
    *   `3000` (TCP): P2P Network (Must be open to world)
    *   `80/443` (TCP): Web API (via Nginx)

---

## 1. Installation

Update system and install Go.

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y golang-go git nginx certbot python3-certbot-nginx
```

Clone and Build:

```bash
git clone https://github.com/nicolocarcagni/sole.git /opt/sole
cd /opt/sole
# Build binary
go build -o sole-cli .
# Initialize Data
./sole-cli init
```

---

## 2. Systemd Service Config

Create a service unit to keep the node running in background and auto-restart on crash.

**File:** `/etc/systemd/system/sole.service`

```ini
[Unit]
Description=SOLE Blockchain Node
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/sole
# Replace X.X.X.X with your Public IP
ExecStart=/opt/sole/sole-cli startnode \
    --port 3000 \
    --api-port 8080 \
    --public-ip X.X.X.X \
    --api-listen 127.0.0.1
Restart=always
RestartSec=5
LimitNOFILE=4096

[Install]
WantedBy=multi-user.target
```

Enable and Start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable sole
sudo systemctl start sole
```

Check logs: `journalctl -u sole -f`

---

✅ **Done!** Your SOLE node is now online.
