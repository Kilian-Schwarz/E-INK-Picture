# E-INK-Picture

## Stack
- Server: Go 1.24, net/http, go:embed, golang.org/x/image
- Frontend: Vanilla HTML/CSS/JS (embedded in Go binary)
- Client: Python 3.11, Waveshare epd7in5_V2, Pillow, requests
- Deploy: Docker Compose, Multi-stage ARM64/AMD64 Build
- Target: Raspberry Pi Zero 2 W (512MB RAM)

## Commands
- `docker compose up --build` -- Build + Start (all-in-one)
- `docker compose -f docker-compose.yml -f docker-compose.cloud.yml up -d` -- Cloud mode
- `cd server && go run .` -- Dev Server
- `cd server && go build -o server .` -- Production Build
- `cd server && go test ./...` -- Tests
- `cd server && go vet ./...` -- Static Analysis
- `cd client && python3 client.py` -- E-Ink Client

## Architecture
- server/main.go -- Entrypoint, routing, middleware
- server/internal/handlers/ -- HTTP request handlers
- server/internal/services/ -- Business logic (design, image, weather, preview)
- server/internal/models/ -- Data structs
- server/internal/config/ -- Environment config
- server/internal/middleware/ -- Logging, CORS
- server/static/ -- CSS, JS (embedded via go:embed)
- server/templates/ -- HTML templates (embedded)
- client/ -- Raspberry Pi E-Ink client (Python)
- data/ -- Persistent data (designs, images, fonts, weather_styles)
- scripts/ -- Setup scripts

## Conventions
- Go: gofmt, error returns, no panic, log/slog
- Python: snake_case, type hints, logging module
- Config: Environment variables, .env.example
- Git: Conventional Commits, trunk-based

## Niemals bearbeiten
- server/static/ (extracted from HTML, treat as generated)
- node_modules/, dist/, .next/ (don't exist but standard rule)

## Important
- Environment: see .env.example
- Display: 800x480px, Waveshare epd7in5_V2
- Weather API: open-meteo.com (free, no key)
- Two deployment modes: local (all-in-one) and cloud
- License: GPL-3.0
