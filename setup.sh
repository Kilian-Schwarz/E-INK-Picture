#!/bin/bash
# E-Ink Picture — Native Setup for Raspberry Pi Zero 2 W
# Installs all dependencies, builds server, sets up client venv
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

GO_VERSION="1.24.1"
REQUIRED_GO_MAJOR=1
REQUIRED_GO_MINOR=24

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

echo "=========================================="
echo "  E-Ink Picture — Native Pi Setup"
echo "=========================================="
echo ""

# ----- 1. OS Detection -----
info "Detecting platform..."
ARCH=$(uname -m)
OS=$(uname -s)

if [ "$OS" != "Linux" ]; then
    error "This script only supports Linux (got: $OS)"
    exit 1
fi

case "$ARCH" in
    aarch64) GO_ARCH="arm64"; GO_ARM="" ;;
    armv7l)  GO_ARCH="arm";   GO_ARM="7" ;;
    armv6l)  GO_ARCH="arm";   GO_ARM="6" ;;
    x86_64)  GO_ARCH="amd64"; GO_ARM="" ;;
    *)       error "Unsupported architecture: $ARCH"; exit 1 ;;
esac

info "Platform: $OS $ARCH (Go: $GO_ARCH)"

# ----- 2. System Packages -----
info "Installing system packages..."
sudo apt-get update -qq

sudo apt-get install -y -qq \
    python3 python3-pip python3-venv python3-dev \
    git curl wget \
    libopenjp2-7-dev libtiff-dev libfreetype6-dev libjpeg-dev zlib1g-dev \
    libgpiod2 libgpiod-dev \
    fonts-noto-core fonts-dejavu-core fontconfig \
    logrotate \
    2>/dev/null || {
        warn "Some packages may not be available, trying essential ones..."
        sudo apt-get install -y python3 python3-pip python3-venv git curl wget \
            libopenjp2-7-dev libjpeg-dev zlib1g-dev 2>/dev/null || true
    }

# ----- 3. SPI Check -----
info "Checking SPI interface..."
if ls /dev/spidev* 1>/dev/null 2>&1; then
    info "SPI is enabled"
else
    warn "SPI not detected!"
    if command -v raspi-config &>/dev/null; then
        read -p "  Enable SPI now? [Y/n] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Nn]$ ]]; then
            sudo raspi-config nonint do_spi 0
            info "SPI enabled — reboot required after setup"
        fi
    else
        warn "Enable SPI manually: sudo raspi-config -> Interface Options -> SPI"
    fi
fi

# ----- 4. Go Installation -----
info "Checking Go installation..."

go_installed=false
if command -v go &>/dev/null; then
    GO_CURRENT=$(go version | grep -oP 'go(\d+\.\d+)' | head -1 | sed 's/go//')
    GO_CUR_MAJOR=$(echo "$GO_CURRENT" | cut -d. -f1)
    GO_CUR_MINOR=$(echo "$GO_CURRENT" | cut -d. -f2)
    if [ "$GO_CUR_MAJOR" -ge "$REQUIRED_GO_MAJOR" ] && [ "$GO_CUR_MINOR" -ge "$REQUIRED_GO_MINOR" ]; then
        info "Go $GO_CURRENT already installed (>= $REQUIRED_GO_MAJOR.$REQUIRED_GO_MINOR)"
        go_installed=true
    else
        warn "Go $GO_CURRENT too old (need >= $REQUIRED_GO_MAJOR.$REQUIRED_GO_MINOR)"
    fi
fi

if [ "$go_installed" = false ]; then
    info "Installing Go $GO_VERSION for $GO_ARCH..."
    GO_TAR="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    GO_URL="https://go.dev/dl/${GO_TAR}"

    wget -q "$GO_URL" -O "/tmp/$GO_TAR"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/$GO_TAR"
    rm -f "/tmp/$GO_TAR"

    # Add to PATH for current session
    export PATH="/usr/local/go/bin:$PATH"

    # Add to profile if not already there
    if ! grep -q '/usr/local/go/bin' "$HOME/.profile" 2>/dev/null; then
        echo 'export PATH="/usr/local/go/bin:$PATH"' >> "$HOME/.profile"
        info "Added Go to PATH in ~/.profile"
    fi

    info "Go $(go version | grep -oP 'go[\d.]+') installed"
fi

# ----- 5. Build Go Server -----
info "Building Go server..."
cd "$SCRIPT_DIR/server"

export CGO_ENABLED=0
export GOOS=linux
export GOARCH=$GO_ARCH
if [ -n "$GO_ARM" ]; then
    export GOARM=$GO_ARM
fi

go mod download
go build -ldflags="-s -w" -o "$SCRIPT_DIR/server/eink-server" .
info "Server binary built: server/eink-server ($(du -h "$SCRIPT_DIR/server/eink-server" | cut -f1))"

cd "$SCRIPT_DIR"

# ----- 6. Python Virtual Environment -----
info "Setting up Python virtual environment..."
if [ ! -d "$SCRIPT_DIR/venv" ]; then
    python3 -m venv "$SCRIPT_DIR/venv"
    info "Virtual environment created: venv/"
else
    info "Virtual environment already exists"
fi

# Activate and install dependencies
source "$SCRIPT_DIR/venv/bin/activate"

pip install --no-cache-dir --upgrade pip setuptools wheel 2>/dev/null || true

info "Installing Python dependencies..."
pip install --no-cache-dir \
    requests>=2.31.0 \
    Pillow>=10.0.0 \
    2>/dev/null

# Hardware dependencies (may fail on non-Pi)
pip install --no-cache-dir \
    RPi.GPIO>=0.7.1 \
    spidev>=3.6 \
    gpiod>=2.0.2 \
    gpiozero>=2.0 \
    lgpio>=0.2 \
    2>/dev/null || warn "Some GPIO packages failed (normal on non-Pi hardware)"

# Waveshare EPD driver
info "Installing Waveshare E-Paper drivers..."
pip install --no-cache-dir \
    "git+https://github.com/waveshare/e-Paper.git#subdirectory=RaspberryPi_JetsonNano/python" \
    2>/dev/null || warn "Waveshare driver install failed (will run in preview-only mode)"

deactivate

# ----- 7. Data Directories -----
info "Creating data directories..."
mkdir -p "$SCRIPT_DIR/data/designs/history"
mkdir -p "$SCRIPT_DIR/data/uploaded_images/thumbs"
mkdir -p "$SCRIPT_DIR/data/fonts"
mkdir -p "$SCRIPT_DIR/data/weather_styles"
mkdir -p "$SCRIPT_DIR/logs"

# ----- 8. Environment File -----
if [ ! -f "$SCRIPT_DIR/.env" ]; then
    info "Creating .env from template..."
    sed 's|DATA_DIR=/app/data|DATA_DIR=./data|' "$SCRIPT_DIR/.env.example" > "$SCRIPT_DIR/.env"
    info ".env created — edit to customize settings"
else
    info ".env already exists"
fi

# ----- 9. Log Rotation -----
info "Setting up log rotation..."
sudo tee /etc/logrotate.d/eink-picture > /dev/null <<EOF
$SCRIPT_DIR/logs/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
    size 10M
}
EOF

# ----- 10. Make scripts executable -----
chmod +x "$SCRIPT_DIR/eink.sh"

# ----- 11. Systemd Services (optional) -----
echo ""
read -p "Install systemd services for autostart on boot? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    info "Installing systemd services..."

    # Server service
    sudo tee /etc/systemd/system/eink-server.service > /dev/null <<EOF
[Unit]
Description=E-Ink Picture Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$SCRIPT_DIR/server
EnvironmentFile=$SCRIPT_DIR/.env
Environment=DATA_DIR=$SCRIPT_DIR/data
ExecStart=$SCRIPT_DIR/server/eink-server
Restart=on-failure
RestartSec=5
StandardOutput=append:$SCRIPT_DIR/logs/server.log
StandardError=append:$SCRIPT_DIR/logs/server.log

[Install]
WantedBy=multi-user.target
EOF

    # Client service
    sudo tee /etc/systemd/system/eink-client.service > /dev/null <<EOF
[Unit]
Description=E-Ink Picture Client
After=eink-server.service
Requires=eink-server.service

[Service]
Type=simple
User=$USER
WorkingDirectory=$SCRIPT_DIR/client
EnvironmentFile=$SCRIPT_DIR/.env
Environment=EINK_SERVER_URL=http://localhost:5000
ExecStart=$SCRIPT_DIR/venv/bin/python3 $SCRIPT_DIR/client/client.py
Restart=on-failure
RestartSec=10
StandardOutput=append:$SCRIPT_DIR/logs/client.log
StandardError=append:$SCRIPT_DIR/logs/client.log

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable eink-server.service eink-client.service
    info "Systemd services installed and enabled"
    info "Use: sudo systemctl start eink-server eink-client"
fi

# ----- 12. Summary -----
echo ""
echo "=========================================="
echo "  Setup Complete!"
echo "=========================================="
echo ""
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo "  Start:     ./eink.sh start"
echo "  Stop:      ./eink.sh stop"
echo "  Status:    ./eink.sh status"
echo "  Logs:      ./eink.sh logs"
echo ""
echo "  Designer:  http://${IP:-localhost}:5000/designer"
echo "  Health:    http://${IP:-localhost}:5000/health"
echo ""
if ls /dev/spidev* 1>/dev/null 2>&1; then
    echo "  SPI: enabled"
else
    echo "  SPI: NOT enabled — reboot may be required"
fi
echo ""
