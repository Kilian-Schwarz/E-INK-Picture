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

## Memory

The server limits itself at runtime: at most `EINK_MAX_CONCURRENT_RENDERS`
(default 1) preview renders run concurrently, and the Go runtime keeps its
heap under the soft limit `EINK_GOMEMLIMIT` (default `64MiB`; `off` disables
it). Both are set in `.env` — see `.env.example`.

Note for Docker deployments: the `deploy.resources.limits.memory` values in
`docker-compose.yml` (128M server / 64M client) have NO effect on standard
Raspberry Pi OS, because the cgroup memory controller is inactive by default
(verified on hardware). Enabling it would require kernel cmdline changes
(`cgroup_enable=memory`), which this project deliberately does not do — the
in-process limits above are the effective mechanism.

## Requirements

- Raspberry Pi (Zero 2 W, 3, 4, 5) with Raspberry Pi OS
- SPI enabled (`sudo raspi-config` → Interface Options → SPI)
- Internet connection (for initial setup only)
- Waveshare E-Paper Display (epd7in3e or epd7in5_V2)
