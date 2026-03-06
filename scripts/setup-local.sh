#!/bin/bash
set -e

echo "=========================================="
echo "  E-Ink Picture - All-in-One Pi Setup"
echo "=========================================="
echo ""

# 1. SPI Check
echo "[1/6] Checking SPI..."
if ls /dev/spidev* 1>/dev/null 2>&1; then
    echo "  SPI is enabled"
else
    echo "  WARNING: SPI not detected!"
    echo "  Run: sudo raspi-config -> Interface Options -> SPI -> Enable"
    echo "  Then reboot and run this script again."
    exit 1
fi

# 2. Docker Check
echo "[2/6] Checking Docker..."
if command -v docker &>/dev/null && docker compose version &>/dev/null; then
    echo "  Docker + Compose available"
else
    echo "  Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker "$USER"
    echo "  Docker installed (re-login required for group change)"
fi

# 3. Environment
echo "[3/6] Setting up environment..."
if [ ! -f .env ]; then
    cp .env.example .env
    echo "  .env created from template"
else
    echo "  .env already exists"
fi

# Create data directories
mkdir -p data/designs data/uploaded_images data/fonts data/weather_styles

# 4. Build and start
echo "[4/6] Building and starting services..."
docker compose up --build -d

# 5. Wait for server
echo "[5/6] Waiting for server..."
for i in $(seq 1 30); do
    if curl -sf http://localhost:5000/health > /dev/null 2>&1; then
        echo "  Server is running!"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "  Server did not start in time. Check: docker compose logs"
    fi
    sleep 2
done

# 6. Summary
echo "[6/6] Done!"
echo ""
echo "=========================================="
echo "  Setup Complete!"
echo "=========================================="
echo ""
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo "  Designer:  http://${IP:-localhost}:5000/designer"
echo "  Health:    http://localhost:5000/health"
echo ""
echo "  View logs:    docker compose logs -f"
echo "  Stop:         docker compose down"
echo "  Restart:      docker compose restart"
echo ""
