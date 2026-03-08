# Installation — Raspberry Pi Zero 2 W (Native)

## Quick Start

```bash
git clone https://github.com/Kilian-Schwarz/E-INK-Picture.git
cd E-INK-Picture
chmod +x setup.sh eink.sh
./setup.sh
```

## Usage

```bash
./eink.sh start     # Start server + client
./eink.sh stop      # Stop all
./eink.sh restart   # Restart
./eink.sh status    # Show status + recent logs
./eink.sh logs      # Follow logs (tail -f)
```

## Configuration

Edit `.env` to change port, display driver, timezone, etc.

## Requirements

- Raspberry Pi (Zero 2 W, 3, 4, 5) with Raspberry Pi OS
- SPI enabled (`sudo raspi-config` → Interface Options → SPI)
- Internet connection (for initial setup only)
- Waveshare E-Paper Display (epd7in3e or epd7in5_V2)
