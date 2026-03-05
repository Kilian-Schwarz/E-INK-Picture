# E-Ink Picture Client

Python client for the E-Ink Picture project. Fetches a pre-rendered preview image from the Go server and displays it on a Waveshare E-Ink display.

## Hardware Requirements

- **Raspberry Pi Zero 2 W** (or any Raspberry Pi with GPIO header)
- **Waveshare 7.5 inch E-Paper V2** (800x480, black/white)
- SPI connection between Pi and display via the included ribbon cable

## Prerequisites

### Enable SPI

SPI must be enabled on the Raspberry Pi for communication with the E-Ink display:

```bash
sudo raspi-config
```

Navigate to: **Interface Options** -> **SPI** -> **Enable**

Reboot after enabling:

```bash
sudo reboot
```

Verify SPI is active:

```bash
ls /dev/spidev*
# Expected output: /dev/spidev0.0  /dev/spidev0.1
```

## Setup

### Option A: Automated Setup (Local Server + Client)

If the Go server runs on the same Pi:

```bash
./scripts/setup-local.sh
```

### Option B: Automated Setup (Cloud Client Only)

If the Go server runs on a remote machine:

```bash
./scripts/setup-cloud-client.sh
```

### Option C: Manual Setup

1. Install Python dependencies:

```bash
cd client
pip3 install -r requirements.txt
```

2. Install the Waveshare EPD library (on Raspberry Pi):

```bash
pip3 install waveshare-epd
```

3. Copy and configure the environment file:

```bash
cp .env.example .env
# Edit .env with your server URL and display settings
```

4. Run the client:

```bash
python3 client.py
```

## Configuration

All configuration is done via environment variables (or a `.env` file):

| Variable | Default | Description |
|---|---|---|
| `EINK_SERVER_URL` | `http://localhost:5000` | URL of the Go server |
| `EINK_WIDTH` | `800` | Display width in pixels |
| `EINK_HEIGHT` | `480` | Display height in pixels |
| `EINK_OFFSET_X` | `0` | Horizontal offset for cropping |
| `EINK_OFFSET_Y` | `0` | Vertical offset for cropping |
| `EINK_REFRESH_INTERVAL` | `300` | Seconds between display refreshes |
| `EINK_DEPLOYMENT_MODE` | `local` | `local` (5s timeout) or `cloud` (15s timeout) |
| `EINK_LOG_LEVEL` | `INFO` | Logging level (DEBUG, INFO, WARNING, ERROR) |

## Autostart with systemd

Create a systemd service to start the client automatically on boot:

```bash
sudo nano /etc/systemd/system/eink-client.service
```

Paste the following content (adjust paths as needed):

```ini
[Unit]
Description=E-Ink Picture Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi/E-INK-Picture/client
EnvironmentFile=/home/pi/E-INK-Picture/client/.env
ExecStart=/usr/bin/python3 /home/pi/E-INK-Picture/client/client.py
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable eink-client.service
sudo systemctl start eink-client.service
```

Check status:

```bash
sudo systemctl status eink-client.service
journalctl -u eink-client.service -f
```

## Troubleshooting

### SPI not active

If the display does not respond, verify SPI is enabled:

```bash
ls /dev/spidev*
```

If no devices are listed, enable SPI via `sudo raspi-config` and reboot.

### Server unreachable

The client logs connection errors and retries every `EINK_REFRESH_INTERVAL` seconds. Check:

- Is the server running? (`curl http://<server-ip>:5000/preview`)
- Is the URL in `.env` correct?
- Are firewall rules blocking port 5000?

For cloud deployments, ensure `EINK_DEPLOYMENT_MODE=cloud` is set to use the longer timeout.

### Display errors

- Check ribbon cable connection between Pi and display
- Ensure no other process is using the SPI bus
- Run with `EINK_LOG_LEVEL=DEBUG` for detailed output
- The Waveshare library requires root access in some configurations — try running with `sudo` if permission errors occur

### Preview mode (no display)

When running without E-Ink hardware (e.g., for development), the client saves the fetched image as `preview_output.png` in the working directory instead of displaying it.
