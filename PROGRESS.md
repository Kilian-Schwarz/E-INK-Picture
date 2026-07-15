# PROGRESS

Manager-geführtes Log für den Weg zu v1.0. Nach jedem Task aktualisiert.
Gates: L1 statisch | L2 Render-Verifikation | L3 Hardware-in-the-Loop | L4 Panel-Foto | L5 Review.

## Aktueller Stand

E1 komplett | E2.1 gemergt (E2.5-Hardware-Gate offen) | **E3 komplett** (E3.1–E3.6; E3.7-Feinschliff als Backlog) | E5.1 KOMPLETT + E5.6 gemergt | E6.1 aktiv | Branch: main

## HIL-Lauf 2 (2026-07-15, Deploy cd053b4 → 5a093eb, Pi 10.33.0.106)

- **E5.6 L3 PASS***: Server-RSS settled 98,2 → **47,8 MB** (−51 %); GOMEMLIMIT + Semaphore aktiv laut Startup-Log. *Einschränkung: VmHWM unverändert (~109 MB) — Peak kommt aus dem Bild-Element-Resize-Pfad (dokumentiertes E5.6-Non-Goal) → Backlog „Bild-Element-Puffer-Diät". Das native 25-MB-Ziel bleibt für den Nativ-Betrieb zu messen (E2.5).
- **E1.6 L3 (technisch) PASS**: Kalibriertes Dithering nachweislich auf dem Panel (Artefakt pixel-identisch zur Server-Preview, exakt 6 Farben). Visuell: Foto drastisch ruhiger, Hauttöne warm, Schwarz entschieden. **Kilians A/B am physischen Panel bleibt das finale Urteil.**
- **E5.1-Übergang PASS**: Ohne Passwort alles offen wie vorher, 0×401 beim Client, stündliche Warn-Erinnerung wie designt.
- Keine Regressionen: Panel-Write 19,9 s, 333 Requests alle 2xx, 2 Panel-Refreshes gesamt (beide autonom). Evidenz: artifacts/hil-2/.

## E3.7-Backlog (Feinschliff, gesammelt aus Verifikationen)

- **Topbar-Overflow Desktop**: kein Wrap-/Overflow-Handling bei 1024–1594 px (Bestand; Theme/Settings/Save teils abgeschnitten). Präzise Lücke: 1024–1279 px hat KEINEN erreichbaren Logout (ab 1280 sichtbar; 768–1023 Tablet-Burger ok).
- favicon.ico-Route fehlt (404 eingeloggt / 401 anonym — Konsolen-Rauschen).
- Properties-Panel zeigt beim Guide-Snap transient die rohe Position (Kosmetik, dokumentiert in E3.4-Spec).
- Crop-Modal: Listener-Akkumulation bei Reopen via X/Overlay (vorbestehend, benign — E3.1-Review-Finding).
- Resize-Snap an Guides (bewusstes E3.4-Non-Goal, Folgetask-Kandidat).
- Sprachmix im Renderer: Forecast-Wochentage/Wetterbeschreibungen englisch neben deutschen Datumszeilen (Bestand; Chore: Lokalisierung der Widget-Strings).
- weather.go:156 holt hart forecast_days=4 — week-planner-Template verspricht 7 Tage (Backend erhöhen oder Template-Beschreibung anpassen).
- Bild-Element-Puffer-Diät (E5.6-Non-Goal): VmHWM ~109 MB kommt aus dem Image-Resize-Pfad (Kernel-Temp pro Bild-Element, Worst Case ~150 MB transient bei großen Fotos) — Kandidat für E5-Folgetask, nötig fürs native 25-MB-Ziel bei Foto-Designs.
- Wizard Schritt 5: früher Karten-Tap kann mit laufendem Queue-Render überlappen (max 2 in flight gemessen; Server serialisiert eh — Fix: auf Queue-Leerlauf warten vor Erfolgs-Render).
- Dashboard-Use-Flow kopiert den inerten templateSlot:"location"-Marker mit (harmlos, Renderer ignoriert ihn).

## Test-Hardware (Baseline 2026-07-14, hardware-validator)

- Host: **10.33.0.106** (seit 2026-07-15; vorher 10.33.0.121 — DHCP, Reservation empfohlen), SSH: ksch@ mit Key ~/.ssh/id_ed25519_10.33.0.121 (Config deckt beide IPs ab; Konnektivität + Identität am 15.07. verifiziert, Container laufen)
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
| E1.2 | Client schreibt eink_last_sent.png (atomar, fehlertolerant, EINK_LAST_SENT_PATH) | L1✅ L5✅ (APPROVE, Mutationstest 5/5) **L3✅ PASS** (HIL 2026-07-15: beide Bilder exakt 6 Palettenfarben, statisch pixelidentisch, Diff 0,325 % = Widget-Zeitversatz; Evidenz artifacts/hil-e12/) L4 – | 614ac53 |
| gofmt-Chore | Format-Drift in 6 Server-Dateien bereinigt (rein mechanisch, Tests grün) | L1✅ | 86507fa |
| E1.1 | Golden-File-Harness: 4 Golden-PNGs, TestPaletteExactness (3 Quality × 2 Profile), Determinismus, Font-Pinning | L1✅ L2✅ (Reviewer: eigene Farbzählung + Sichtprüfung) L5✅ APPROVE | 7752a1a |
| E1.3 | Konsistenter Display-Default via EINK_DISPLAY_TYPE (Fallback waveshare_7in3_e, settings.json gewinnt immer) + CHANGELOG.md angelegt | L1✅ L2✅ (Goldens unverändert) L5✅ APPROVE | d8419e6 |
| E1.4 | Client-Resize-Guard: NEAREST statt LANCZOS, WARNING mit beiden Größen | L1✅ (30 Tests, Negativprobe rot unter LANCZOS) L5✅ APPROVE | a4bc72e |
| E1.5 | Element-Rotation im Renderer (Fabric-Semantik, exakte 90°-Koeffizienten, rotiertes Culling); alte Goldens sha-identisch | L1✅ L2✅ (Reviewer: eigene Sichtprüfung + Farbzählung + Negativprobe) L5✅ APPROVE | 89cc9f4 |
| E1.6 | Kalibriertes Dithering (gemessene Spectra-6-Farben, Index-Swap auf Treiber-Codes), Atkinson wählbar, Precomp-LUT, Kalibrier-Testbild; Fluchtweg calibration="off" byte-identisch legacy | L1✅ L2✅ (Reviewer: Matrix-Nachrechnung, Palette-Zensus, Vorher/Nachher) L5✅ APPROVE; L3 (Kilians A/B am Panel) offen | 3616b9d |
| E6.1 | GitHub-Actions-CI: gofmt/vet/test (inkl. Golden-Suite), Python 3.11 unittest, Cross-Builds arm64/armv7/armv6/amd64 mit Artefakten | L1✅ L5✅ APPROVE; L2✅ erster Runner-Lauf GRÜN auf 865dcb2 (Golden-Suite auf Linux byte-identisch — Font-Pinning bewiesen) | 865dcb2 |
| E3.1 | Pointer Events durchgängig: Fabric enablePointerEvents, Crop-Dialog pointer*+Capture, Rename-Button; 0 mouse*-Handler übrig | L1✅ L5✅ (Statik APPROVE); L2✅ PASS headless (Puppeteer: Touch-Drag/Resize/Crop/Rename mit API-JSON-Beweis, mobil+Tablet+Desktop, 0 JS-Fehler) — Chrome-Extension war offline, echtes iPhone bleibt L3-Empfehlung | af69822 |
| E3.3 | Responsive Layout: <768px Bottom-Sheets + Tabbar + Zoom-Fit, 768–1024px Icon-Rails mit Flyouts, Desktop pixelidentisch (0/1.296.000 Pixel Diff) | L1✅ L2✅ PASS (4 Puppeteer-Protokolle, Drag bei offenem Sheet per API-JSON bewiesen) L5✅ APPROVE | 1676857 |
| E5.6 | Render-Semaphore (Default N=1) + GOMEMLIMIT (64MiB) + Puffer-Diät (TotalAlloc 36,4→11,4 MiB, Goldens byte-identisch) + Font-Race-Fix (Parse-Cache statt Face-Cache) | L1✅ (inkl. -race-Suiten) L5: REQUEST_CHANGES (Reviewer fand echten Font-Race bei N≥2 inkl. Panic) → Fix test-first → APPROVE (Gegenprobe rot, N=4-Stress sauber); L3 (RSS-Messung auf dem Pi) offen | f583c70 |
| E3.2 | Pinch-Zoom + Two-Finger-Pan (eine Geste, Anker-Drift 0,01 px), Long-Press-Kontextmenü, Touch-Handle-Clamp; Overflow jetzt voll scrollbar (vorbestehender Desktop-Mangel mitbehoben) | L1✅ L2✅ (CDP-2-Punkt-Touch, 22/22+23/23) L5: REQUEST_CHANGES (Spec-Geometrie übersah Flex-Zentrierung → Amendment 1) → Fix → APPROVE | 60354b5 |
| E2.1+E2.2 | One-Command-Installer: install.sh-Bootstrap + setup.sh komplett neu (headless, Waveshare-Pin + Sparse-Fetch, kein stiller Preview-only, SPI nonint, Idempotenz, --update, Docker-Preflight, --dry-run, 8 Skript-Tests) | L1✅ (shellcheck 0, 8/8, Dry-Run-Matrix mutationsfrei) L5✅ APPROVE (Funktionsverlust-Audit: keiner); L3/E2.5 (frisches OS → Panel) wartet auf Test-Pi-Umstellung | 06e0882 |
| E5.1 | Auth: Deny-by-default-Guard (55 Routen), bcrypt-Admin-Passwort, Sessions (7d sliding), Client-Token für 4 Endpoints, Rate-Limit, CSRF (Lax+Origin), CORS-Rework, SECRET_KEY entfernt, Token-Generierung in Setup-Skripten, Security-Doku | L1✅ (-race, e2e 32/32) L5✅ Doppel-Review: Code APPROVE (nach AC9-Nacharbeit) + Security nicht blockierend (33 Bypass-Probes gehalten, TOCTOU gefixt); L3 (bcrypt-Timing Pi) offen | 05ae8df |
| E5.1-Client | Python-Client sendet X-Client-Token (zentrale Wrapper, 401 einmal-pro-Zustandswechsel, Secrets-Hygiene-Test) | L1✅ (38 Tests) L5✅ APPROVE (Mutations-Gegenprobe, Server-Guard-Matrix) | 262a4dd |
| E3.6 | Preview-Modal zeigt Panel-Palette als Default (quantisiert inkl. Kalibrierung), Panel/Original-Toggle mit Cache, 503-Handling, Objekt-URL-Leak-Fix | L1✅ L2✅ (Farbzensus 6 bzw. 2 exakt, Request-/Revoke-Zähler, Late-Response-Race, Mobil-Tap) L5✅ APPROVE | 7e97126 |
| E5.1-Frontend | auth.js: 401-Interceptor (loop-sicher), Setup-Banner + Dialog mit Auto-Login, Icon-Logout (Desktop sichtbar ab 1280, Burger-Menü mobil) | L1✅ L2✅ (38+17 Asserts) L5: REQUEST_CHANGES (Logout unsichtbar 1024–1594) → Icon-Fix → APPROVE | aae87b2 |
| E3.5 | Template-Galerie: 8 durchkomponierte Designs (embedded JSONs), panel-echte Previews (sequenziell, Cache, 503-fest), Use-Flow mit Token-/Foto-Slot-Substitution, Go-Lint+Render-Testsuite (offline erzwungen) | L1✅ (24 Subtests) L2✅ (34/34, 16 Renders gesichtet) L5✅ APPROVE (Design-Veto nicht gezogen) | 3f1e23c |
| E5.3 (Server) | Nachtfenster sleep_start/sleep_end (halboffen, Mitternachts-Wrap, fail-open), manueller Trigger durchbricht strukturell, reason-Feld in refresh_status | L1✅ (-race) L5✅ APPROVE (Pfad-Beweis + Grenz-Negativproben); L3 (Fenster-Probe auf Pi) offen | 890d540 |
| E5.2 (Client) | Content-Skip via SHA-256 der Wire-Bytes (5 konservative Konjunkte, 24h-Guard, Kill-Switch, in-memory); statische Designs ≈ 0 Panel-Zyklen | L1✅ (54 Tests, 2 Schärfe-Negativproben) L5✅ APPROVE (Refactor-Drift-Audit sauber); L3 (Skip-Zählung 1h auf Pi) offen | 76bdbe8 |
| E2.3 | Erst-Setup-Wizard: 5 Schritte (Display/Standort/Intervall/Passwort/Startdesign), server-seitiges Erscheinen-Kriterium (GET /api/setup/status, Heartbeat-fest), setup_completed-Latch, Galerie-Host-Refactor | L1✅ L2✅ (142 Asserts, Erscheinen-Matrix live) L5✅ APPROVE („produktreifes Onboarding") | 1f0063b |

## Offen / Blockiert

- E1.6-Feintuning: Kilian muss das Kalibrier-Testbild am physischen Panel beurteilen (Design "calibration" importieren, Anleitung in specs/E1.6-panel-calibration.md); Fluchtweg {"calibration":"off"}. Der Pi läuft noch auf Stand cd053b4 (ohne E1.4–E1.6) — nächster HIL-Lauf bringt die Kalibrierung aufs Panel.
- E3 verbleibend: E3.4 (Smart Guides), E3.5 (Template-Galerie), E3.6 (Panel-Palette-Live-Preview), E3.7 (Vervollständigung Multi-Select/Alignment) — Specs ausstehend
- Nächster HIL-Lauf (wenn sinnvoll gebündelt): bringt E1.4–E1.6 + E5.6 aufs Panel — misst Server-RSS nativ gegen das 25-MB-Ziel (L3 E5.6), zeigt kalibriertes Dithering (Kilians A/B, L3 E1.6)
- NEU (aus HIL-Lauf): Server-RSS ~98 MB im Docker-Betrieb — weit über dem 25-MB-Ziel aus dem v1.0-Auftrag; Preview-Render dauert 4,1 s. Braucht eigenen E5-Task (Speicherprofil: Render-Buffer, GOMEMLIMIT, Render-Semaphore). CLAUDE.md-Angabe "~10 MB" ist überholt.
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
