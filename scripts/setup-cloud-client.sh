#!/bin/bash
set -e

echo "=== E-Ink Picture - Cloud Client Setup ==="

read -p "Enter server URL (e.g. https://your-server.com): " SERVER_URL

# Create .env for client
cat > .env <<EOF
SERVER_URL=${SERVER_URL}
EOF

echo "Created .env with SERVER_URL=${SERVER_URL}"

# Install Python dependencies
echo "Installing Python dependencies..."
pip install Pillow requests icalendar

# Create data directories for local cache
mkdir -p temp_files

echo ""
echo "Setup complete. Run the client with:"
echo "  python client/client.py"
