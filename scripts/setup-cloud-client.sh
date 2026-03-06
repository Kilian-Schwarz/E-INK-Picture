#!/bin/bash
set -e

echo "=== E-Ink Picture - Cloud Client Setup ==="

read -p "Enter server URL (e.g. https://your-server.com): " SERVER_URL

# Create client .env
cat > client/.env <<EOF
EINK_SERVER_URL=${SERVER_URL}
EINK_DISPLAY_DRIVER=epd7in3e
EINK_POLL_INTERVAL=30
EINK_LOG_LEVEL=INFO
EINK_DEPLOYMENT_MODE=cloud
EOF

echo "Created client/.env with SERVER_URL=${SERVER_URL}"

# Install Python dependencies
echo "Installing Python dependencies..."
pip3 install --break-system-packages requests Pillow RPi.GPIO spidev 2>/dev/null || \
pip3 install requests Pillow RPi.GPIO spidev

# Waveshare driver
if ! python3 -c "from waveshare_epd import epd7in3e" 2>/dev/null; then
    echo "Installing Waveshare drivers..."
    pip3 install --break-system-packages \
        git+https://github.com/waveshare/e-Paper.git#subdirectory=RaspberryPi_JetsonNano/python 2>/dev/null || \
    pip3 install \
        git+https://github.com/waveshare/e-Paper.git#subdirectory=RaspberryPi_JetsonNano/python
fi

echo ""
echo "Setup complete. Run the client with:"
echo "  cd client && python3 client.py"
