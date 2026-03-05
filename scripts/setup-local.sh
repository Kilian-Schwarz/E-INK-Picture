#!/bin/bash
set -e

echo "=== E-Ink Picture - Local (All-in-One) Setup ==="

# Create .env if not exists
if [ ! -f .env ]; then
    cp .env.example .env
    echo "Created .env from .env.example - edit if needed"
fi

# Create data directories
mkdir -p data/designs data/uploaded_images data/fonts data/weather_styles

# Weather styles are shipped in data/weather_styles/ (no copy needed)

# Build and start server
echo "Building and starting server..."
docker compose up --build -d

echo ""
echo "Server running at http://localhost:5000"
echo "Designer UI at http://localhost:5000/designer"
echo ""
echo "To set up the client, install Python dependencies:"
echo "  pip install Pillow requests icalendar"
echo "  python client/client.py"
