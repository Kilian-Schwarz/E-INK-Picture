# PROGRESS

Manager-geführtes Log für den Weg zu v1.0. Nach jedem Task aktualisiert.
Gates: L1 statisch | L2 Render-Verifikation | L3 Hardware-in-the-Loop | L4 Panel-Foto | L5 Review.

## Aktueller Stand

Epic: E1 | Nächste Tasks: E1.1 (Golden Files) + E1.3 (Default-Konsistenz) — Specs in Arbeit | Branch: main

## Test-Hardware (Baseline 2026-07-14, hardware-validator)

- Host: 10.33.0.121, SSH: ksch@ mit Key ~/.ssh/id_ed25519_10.33.0.121 (BatchMode OK)
- Pi-Modell: **Raspberry Pi 3 Model B Rev 1.2** (aarch64, Debian 13, 64-bit) — NICHT Pi Zero 2 W; Pi-Zero-Verifikation braucht später separates Gerät
- Panel: **epd7in3e** (6-Color Spectra 6, 800x480); SPI aktiv (/dev/spidev0.0/0.1)
- Betriebsart: Docker Compose (e-ink-picture-server-1 healthy + e-ink-picture-client-1), Git-Checkout ~/E-INK-Picture @ main
- Refresh-Dauer laut Logs: Panel-Write ~19,5 s, Gesamtzyklus ~22,7 s (konsistent, 9 Refreshes)
- RAM (im Docker-Betrieb): Server RSS 46 MB / Client RSS 35 MB — RAM-Budgets aus §8 gelten für den nativen Betrieb, Docker-Messwerte nicht 1:1 vergleichbar
- Kamera: **keine** → Gate L4 auf diesem Gerät nicht möglich
- ⚠️ Under-Voltage/Throttling seit Boot aufgetreten (0x50000) — Netzteil/Kabel prüfen (Kilian)
- ⚠️ refresh_interval steht auf 900 s (~96 Voll-Refreshes/Tag) — Panel-Verschleiß; bewusste Entscheidung nötig (Kilian)
- ⚠️ Kein RTC: Log-Zeitstempel direkt nach Boot unzuverlässig — für Timing-Messungen NTP-Sync abwarten

## Epics (Reihenfolge bewusst)

| Epic | Titel | Prio | Status |
|------|-------|------|--------|
| E1 | Display-Support & Render-Korrektheit (G1, G4) | P0 | offen |
| E2 | One-Command-Installation (G7, G10, G4) | P0 | offen |
| E3 | Designer: Canva-Niveau, Handy & PC (G2) | P0 | offen |
| E4 | Live-Datenquellen (G6, G9) | P1 | offen |
| E5 | Betrieb & Zuverlässigkeit (G3, G8) | P1 | offen |
| E6 | CI & Release (G5, G10) | P1 | offen |

## Erledigt

| Task | Beschreibung | Gates | Commit |
|------|--------------|-------|--------|
| Bootstrap | Subagent-Roster (.claude/agents/, 11 Agents) + PROGRESS.md angelegt | n/a (keine Code-Änderung) | 162e3ec |
| CLAUDE.md-Fix | Überholte server/static/-Regel entfernt (handgepflegter Frontend-Code) | n/a (docs-only, Diff manuell geprüft) | f51d46e |
| Discovery | repo-cartographer: Gap-Analyse verifiziert, G1 im Kern widerlegt, E1-Plan überarbeitet | n/a (read-only) | — |
| HW-Baseline | hardware-validator: Pi 3B + epd7in3e + Docker-Modus festgestellt, keine Kamera (L4 entfällt) | n/a (read-only) | — |
| E1.2 | Client schreibt eink_last_sent.png (atomar, fehlertolerant, EINK_LAST_SENT_PATH) | L1✅ L5✅ (APPROVE, Mutationstest 5/5) L2 n/a, L3 folgt, L4 – | 614ac53 |
| gofmt-Chore | Format-Drift in 6 Server-Dateien bereinigt (rein mechanisch, Tests grün) | L1✅ | 86507fa |

## Offen / Blockiert

- E1.1: Spec fertig (specs/E1.1-golden-file-harness.md), Implementierung läuft (test-engineer)
- E1.3: Spec fertig (specs/E1.3-display-default-consistency.md), wartet auf E1.1 (sequenziell, gleiche Package-Nachbarschaft)
- L3-Nachweis E1.2: beim nächsten Hardware-Durchlauf /tmp/eink_last_sent.png vom Pi holen und gegen Server-Preview vergleichen
- Entscheidung Kilian: refresh_interval 900 s auf dem Test-Pi beibehalten oder erhöhen? (Panel-Verschleiß)
- Entscheidung Kilian: Test-Pi von Docker- auf Nativ-Betrieb umstellen für E2-Tests? (data/ wird vorher gesichert)
- Hardware Kilian: Under-Voltage seit Boot (0x50000) — Netzteil/Kabel des Test-Pi prüfen; optional Kamera für Gate L4 nachrüsten

## Übersprungene Gates

- **L4 (Panel-Foto) entfällt für alle Tasks** — keine Kamera am Test-Pi (rpicam-still: "No cameras available"). Visuelle End-to-End-Verifikation nur via /tmp/eink_last_sent.png (L3) möglich, bis eine Kamera nachgerüstet wird.

## Discovery-Ergebnis (repo-cartographer, 2026-07-14) — korrigierte Gap-Analyse

- **G1 im Kern WIDERLEGT:** Server quantisiert bereits exakt auf die 6 Treiber-Farben (preview.go:790-804, Floyd-Steinberg gegen display.go:40) und das Bild erreicht das Panel im Normalfall 1:1 (Treiber-Re-Quantize ist No-Op bei exakter Palette). Echte E1-Baustellen stattdessen:
  - Default-Mismatch: Server-Default `waveshare_7in5_v2` (settings.go:40) vs. Client-Default `epd7in3e` (config.py:5) → frisches 6-Farb-Setup rendert B/W-quantisiert.
  - Dithering gegen Ideal-Primärfarben statt real gemessener (entsättigter) Spectra-6-Panelfarben → Kalibrierung fehlt.
  - Client-Resize mit LANCZOS (client.py:92-94) zerstört Dithering, falls Größe je abweicht (ungeschützt).
  - Rotation im Schema (design.go:120) wird im Renderer nie angewendet → WYSIWYG-Bruch.
  - TestPaletteQuantization prüft nur den Typ, nicht die Farben; keine Golden Files, kein testdata/.
- G2 (kein Touch), G3 (keine Auth + SSRF via icalUrl/feedUrl/custom-URL + CORS *), G5, G6, G8, G9, G10: BESTÄTIGT. G4: Arch-Erkennung in setup.sh ist korrekt (alle 3 Archs), fragil ist das Drumherum (ungepinnte Waveshare-Lib von git HEAD, verschluckte Fehler, interaktive Prompts). G7: setup.sh erzeugt .env selbst; was fehlt, ist geführtes Panel-/Standort-Setup im UI.
- Toter Code: conditions.go (239 Z., nie instanziiert — Conditions-Feature ist wirkungslos!), WidgetRenderer-Interface (8 leere Stubs, 0 Aufrufer), handlers/display.go (pusht an nicht existierenden Client-HTTP-Server), Config.SecretKey ungenutzt, 4 tote Funktionen in preview.go, Widget-Logik doppelt (preview.go vs widgets/*).
- Risiken: Renderer macht synchrone HTTP-Calls im Renderpfad (Preview kann >30 s brauchen); /preview unlimitiert parallel (RAM-Spikes auf 512-MB-Pi); Designer lädt Fabric.js 5.3.1 + Google Fonts per CDN → offline kein Designer; Frontend nutzt Fabric.js (Touch-Basis teils vorhanden), Interaktionen aber mouse-only.

## E1 — überarbeiteter Task-Plan (nach Discovery)

| Task | Inhalt | Agent | Status |
|------|--------|-------|--------|
| E1.1 | Golden-File-Harness: deterministische Testdesigns → Referenz-PNGs + exakte Palette-Assertions (≤6 bzw. ≤2 unique Farben) | test-engineer | Spec in Arbeit |
| E1.2 | Client schreibt /tmp/eink_last_sent.png (atomar, fehlertolerant) | pi-client | Implementiert, Tests laufen |
| E1.3 | Default-Mismatch Server↔Client fixen (ein konsistenter Display-Default) | go-backend | offen |
| E1.4 | Client-Resize-Guard: Paletted-Input nie mit LANCZOS resizen (NEAREST + Warnung) | pi-client | offen (nach E1.2) |
| E1.5 | Rotation im Renderer implementieren (WYSIWYG Designer↔Panel) | render-quality | offen |
| E1.6 | Real-Palette-Kalibrierung: Dither gegen gemessene Spectra-6-Farben → Index-Mapping auf Treiber-Codes; Gamma/Sättigung-Vorkompensation; FS + Atkinson wählbar | render-quality | offen |

(Render-Robustheit — Widget-Fetches aus dem Renderpfad entkoppeln, Render-Semaphore — wandert nach E5.)

## Erkenntnisse

- Test-Pi ist Pi 3B (aarch64/64-bit), nicht Pi Zero 2 W → Pi-Zero-/armv6-Pfad braucht Cross-Build-Checks in CI oder ein zweites Gerät.
- Der Pi läuft im Docker-Modus, nicht nativ — der native setup.sh-Pfad (letzte 3 Commits) ist auf diesem Gerät noch nie gelaufen. Für E2 (One-Command-Install) ist ein Umstieg des Test-Pi auf den nativen Modus nötig; vorher data/ sichern und mit Kilian abstimmen.
- Server-Settings überstimmen Container-Env (refresh_interval 900 vs. EINK_REFRESH_INTERVAL=3600) — Settings-Präzedenz bei E2-Arbeit beachten.
- Dev-Umgebung Mac: Go-Toolchain war nicht installiert → brew install go (1.26.5) am 2026-07-14. gofmt 1.26 meldet Alignment-Drift in 6 Server-Dateien (rein mechanisch) — Chore-Commit folgt; CI (E6) muss Go-Version pinnen.
- pytest ist auf dem Mac nicht installiert — Python-Tests laufen via unittest.
