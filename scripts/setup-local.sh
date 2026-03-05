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

# Copy default weather styles if data dir is empty
if [ -z "$(ls -A data/weather_styles 2>/dev/null)" ] && [ -d app/weather_styles ]; then
    cp app/weather_styles/*.json data/weather_styles/
    echo "Copied default weather styles"
fi

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
