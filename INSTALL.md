# Installation — Raspberry Pi (Native)

## One-Command Install

On a fresh Raspberry Pi OS installation, one command sets up everything —
server build, Python venv with the pinned Waveshare driver, SPI, and systemd
autostart:

```bash
curl -fsSL https://raw.githubusercontent.com/Kilian-Schwarz/E-INK-Picture/main/install.sh | bash
```

The installer clones the repo into `$HOME/E-INK-Picture` (override with
`EINK_INSTALL_DIR`), builds the Go server, creates the client venv, enables
SPI, and installs + starts both systemd services (`enable --now`). It runs
fully non-interactively and fails loudly instead of degrading silently — a
driver failure on Pi hardware aborts the setup unless you explicitly allow
preview-only mode.

Running the same command again on an existing installation performs an
update (`git pull --ff-only`, rebuild, dependency sync, service restart).

### Flags

All flags work for both `install.sh` and `setup.sh`. With the one-liner,
pass them after `bash -s --`:

```bash
curl -fsSL https://raw.githubusercontent.com/Kilian-Schwarz/E-INK-Picture/main/install.sh | bash -s -- --update
```

| Flag | Effect |
|---|---|
| `--headless`, `--yes` | No prompts; defaults: enable SPI = yes, install systemd services = yes. Auto-enabled when stdin is not a TTY (`curl \| bash`). |
| `--update` | Update mode: `git pull --ff-only`, rebuild server, sync pinned dependencies, re-render units, restart services. Auto-detected by `install.sh` when the target directory is already a checkout. |
| `--allow-preview-only` | On Pi hardware, continue even if the display driver setup fails (client then runs in preview-only mode). Without this flag a driver failure on a Pi aborts with a clear error. |
| `--dry-run` | Print every mutating action as `[DRY-RUN] <cmd>` instead of executing it. Nothing is changed; useful to inspect the plan. |

### Environment Variables

| Variable | Default | Effect |
|---|---|---|
| `EINK_INSTALL_DIR` | `$HOME/E-INK-Picture` | Target directory for the checkout. |

### Notes

- **SPI + reboot:** If SPI was just enabled, a reboot is required before the
  display works. The services start again automatically after the reboot
  (`Restart=always`).
- **Docker conflict:** If a Docker-based installation of this project is
  running on the same machine, the installer aborts before changing anything.
  Run `docker compose down` in the old checkout (and back up / migrate its
  `data/` directory) first.
- **sudo:** The one-liner needs passwordless sudo (default on Raspberry Pi
  OS). If sudo asks for a password, run `sudo -v` first, or use the manual
  route below.
- **Waveshare driver pin:** The e-Paper driver is installed from a pinned
  upstream commit (see `WAVESHARE_EPD_PIN` in `setup.sh`) for reproducible
  installs. Pin bumps are deliberate single commits.
- **GPIO pin factory (kernel ≥ 6.6):** On current Raspberry Pi OS
  (Bookworm/Trixie, kernel ≥ 6.6) `lgpio` is the only working gpiozero pin
  factory — the legacy `native`/sysfs factory is gone and RPi.GPIO edge
  detection fails. The installer therefore installs the distro package
  `python3-lgpio` (exposed to the venv via `--system-site-packages`), removes
  the `Jetson.GPIO` shim that the pinned Waveshare package drags in (it shadows
  the real `RPi.GPIO`), and pins `GPIOZERO_PIN_FACTORY=lgpio` in `.env`. On an
  older kernel where `lgpio` is unavailable it falls back to `rpigpio`. If the
  driver still cannot be imported on Pi hardware the setup aborts with a clear
  error; use `--allow-preview-only` to continue without a display.

## Manual Install (alternative)

If you prefer to review before running:

```bash
git clone https://github.com/Kilian-Schwarz/E-INK-Picture.git
cd E-INK-Picture
./setup.sh
```

Run in a terminal, `setup.sh` asks interactively (SPI, systemd); the flags
above work here too, e.g. `./setup.sh --headless` or `./setup.sh --update`.

## Usage

With systemd (default after install):

```bash
sudo systemctl status eink-server eink-client
sudo systemctl restart eink-server eink-client
```

Without systemd (manual alternative):

```bash
./eink.sh start     # Start server + client
./eink.sh stop      # Stop all
./eink.sh restart   # Restart
./eink.sh status    # Show status + recent logs
./eink.sh logs      # Follow logs (tail -f)
```

## Uninstall

```bash
sudo systemctl disable --now eink-server eink-client
sudo rm /etc/systemd/system/eink-server.service /etc/systemd/system/eink-client.service
sudo systemctl daemon-reload
sudo rm /etc/logrotate.d/eink-picture
rm -rf ~/E-INK-Picture
```

The SPI flag may harmlessly stay enabled.

## Configuration

Edit `.env` to change port, display driver, timezone, etc. The `.env` file is
never overwritten by setup or update runs.

## Security

- **Set an admin password right after installation** (login page /
  `POST /api/auth/setup`, or once via `EINK_ADMIN_PASSWORD` in `.env`).
  Until a password is set, the server is completely open on the LAN
  (compatible with older versions; the server warns loudly), and anyone on
  the network could claim the device by setting the first password.
- The installer generates `EINK_CLIENT_TOKEN` into `.env` (and appends it on
  updates of older installs, never overwriting an existing value). Both
  systemd units read the same `.env`, so the display client keeps working
  once a password is set. Manual generation: `openssl rand -hex 32`.
- The server speaks plain HTTP — without TLS the session cookie is readable
  on the wire (LAN threat model: curious participants and CSRF, not an active
  MITM). Behind a TLS reverse proxy, set `EINK_COOKIE_SECURE=true`.
- **Forgot the password?** Delete `data/auth.json` in the install directory
  and restart (`sudo systemctl restart eink-server`) — the server returns to
  the open no-password state.
- Login/setup are rate-limited (5 attempts / 60 s per source IP). In Docker
  deployments all LAN clients share the bridge NAT source IP, so the limit
  acts globally (a third party can temporarily block logins; self-healing
  after 60 s) — native systemd mode is unaffected. IPv6 address rotation can
  sidestep the per-IP limit (named residual risk); bcrypt (~1 s per attempt
  on a Pi) remains the effective brute-force brake.

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
- Internet connection (for initial setup only)
- Waveshare E-Paper Display (epd7in3e or epd7in5_V2)

SPI is enabled automatically by the installer (a reboot may be required).
