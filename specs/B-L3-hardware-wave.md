# B-L3-hardware-wave: Konsolidierte Hardware-in-the-Loop-Validierung (ein Deploy)

## Ziel
Nach diesem Task sind alle offenen Pi-L3-Gates der v1.0-Finishing-Runde — B3
(Refresh-Latenz), B6 (Content-Skip-Nachweis), B5-live (Home-Assistant-Widget mit
echten Daten), B2 (Rundungs-Parität auf dem Panel) und B1 (Standortsuche-Ergebnis
auf dem Panel) — in **einem einzigen Deploy** des aktuellen `main` auf den Test-Pi
gemessen und mit harten, überprüfbaren Werten belegt, wobei **alle Nicht-Panel-Checks
zuerst gebündelt** laufen und die Zahl der Panel-Refreshes auf das Minimum (Baseline +
3 getriggerte Writes) begrenzt bleibt — vorher war keines dieser Gates am physischen
Panel gemessen.

## Kontext

### Betroffene Belege (nur Lesen/Messen — kein Code wird geändert)
Dieser Spec beauftragt ausschließlich Messung/Verifikation, keine Code-Änderung.
Referenz-Punkte (file:line), gegen die gemessen wird:
- **Server-Trigger:** `POST /api/trigger_refresh` → `SettingsService.TriggerRefresh`
  (`server/internal/services/settings.go:306-335`), Broadcast nach durablem Write
  (`:333`). Long-Poll: `GET /api/refresh_status` (public route,
  `server/internal/middleware/auth.go:54`).
- **Server-Request-Log (Zeitquelle t0):** `slog.Info("request", "method", …, "path", …,
  "status", …, "duration", …)` (`server/internal/middleware/logging.go:24-28`).
- **Client-Panel-Init (Zeitquelle t1):** `logger.info("Initializing display...")`
  (`client/client.py:262`); Panel-Write-Ende `"Display updated successfully"`
  (`client/client.py:303`); `epd.sleep()` nach jedem Write (`client/client.py:302`).
- **Content-Skip:** `logger.info("skipping panel refresh (content unchanged)")`
  (`client/client.py:440`); Prädikat `_should_skip_panel_write`
  (`client/client.py:354-374`), Guard `reason != "interval" → return False`
  (manuelle Trigger schreiben immer).
- **Panel-Artefakt:** `save_last_sent_artifact` (`client/client.py:233-247`) schreibt
  `config.LAST_SENT_PATH` (Default `/tmp/eink_last_sent.png`, env `EINK_LAST_SENT_PATH`,
  `client/config.py:15`) — die exakten Wire-Bytes ans Panel.
- **HA-Config:** `POST|GET /api/hass/config` → `data/hass.json` (0600, atomar), Token nie
  im GET-Body (B5 AC-SEC9), Token nie im Log (B5 AC-SEC2, per L1-grep-Gate erzwungen).
- **B2-Referenz:** `testdata/parity/rounding__canvas_ref.png` + Golden-Designs
  (`server/internal/services/testdata/designs/rounding.json`, `basic.json`).
- **Settings-Steuerung:** `GET /settings`, `POST /update_settings` (u. a.
  `refresh_interval`), `GET /health` (Versions-Stempel, E6.2).

### Voraussetzungs-Specs (bereits gemergt, L1/L2 grün auf `main`)
`specs/B3-immediate-refresh-longpoll.md` (AC22 = dieses L3), `specs/B6`-Nachweis
(Code-Beweis vorhanden, nur L3-Skip-Zählung offen), `specs/B5-home-assistant.md`
(L3-Live deferred, §"Gate L3"), `specs/B2-rounding-parity.md` (L3-Foto deferred, §L3),
`specs/B1-location-search.md`, `specs/B7-repro-protocol.md` (§2: echter <768px-Viewport
war in-Session NICHT erreichbar).

### Test-Hardware (Stand UNVERIFIZIERT — hardware-validator prüft zuerst selbst)
Zuletzt bekannt, aber die IP ist gedriftet und der Pi war offline (PROGRESS 2026-07-16):
`ksch@10.33.0.106`, SSH-Key `~/.ssh/id_ed25519_10.33.0.121`, **Raspberry Pi 3 Model B
Rev 1.2** (aarch64, Debian 13 Trixie, Kernel 6.12.47), Panel **epd7in3e** (6-Farb
Spectra 6, 800×480), **Docker-Compose-Modus** (`e-ink-picture-server-1` +
`e-ink-picture-client-1`). **Keine Kamera → Gate L4 auf diesem Gerät unmöglich.**
Alle diese Fakten sind bis zur Selbstprüfung (AC-P1) als unbestätigt zu behandeln.

## Preconditions (blockierend — ohne diese kein Deploy)

- **PC1 — Pi online.** Kilian bestätigt Strom/LED und liefert die aktuelle DHCP-IP aus
  dem Router. `ping` + `ssh` antworten. (PROGRESS: `.106`/`.121` waren tot; kein Netz-Scan
  durch den Agenten — Kilian nennt die IP.)
- **PC2 — Kilian am Test verfügbar**, um (a) das HA-Long-Lived-Token via Web-UI
  einzugeben, (b) die Entity-IDs (Temperatur-`sensor.*`, Alarm-Entity, Präsenz-
  `person.*`/`device_tracker.*`) und das Alarm-Backend (`alarm_control_panel.*` vs.
  `alarmo`) zu nennen, (c) im Designer die B1-Standortsuche zu bedienen und (d) einen
  HA-Live-Wert (Temperatur) zur Gegenprobe vorzulesen. Diese Eingaben sind
  **User-Inputs zur Testzeit** — in diesem Spec NICHT hartkodiert.
- **PC3 — Nur `hardware-validator`** verbindet sich mit dem Pi. Kein anderer Agent.
- **PC4 — `data/` sicherbar.** Genug Platz für `tar czf` von `data/`; keine offene
  Parallel-Session schreibt gerade `data/auth.json`/`data/hass.json` (mit Kilian
  abklären; nie überschreiben).
- **PC5 — Host-Uhr während der Messfenster stabil** (kein NTP-Step mitten in einer
  B3-Messung). Da t0 und t1 aus **derselben Host-Uhr** kommen (beide Container-Logs auf
  demselben Pi), ist der **Delta** immun gegen den fehlenden RTC/absolute Zeit — nur ein
  Uhren-Step innerhalb des Fensters wäre schädlich.

## Deploy + Rollback-Prozedur

### Selbstprüfung ZUERST (AC-P1, kein Schreibzugriff bis grün)
1. `cat /proc/device-tree/model` → enthält `Raspberry Pi 3 Model B`. Modell abweichend →
   **STOPP**, an Kilian melden (falsches Gerät / IP-Drift).
2. Panel: `docker compose exec client env | grep EINK_DISPLAY_DRIVER` → `epd7in3e`
   **und** `settings.json` `display_type` = `waveshare_7in3_e`. Abweichung → STOPP.
3. IP-Identität: `hostname` + `ip link` (MAC) gegen den erwarteten Pi; Repo-Marker
   `git -C ~/E-INK-Picture remote -v` + `git log -1`. Fremdes Gerät → STOPP.
4. Betriebsart: `docker compose ps` → beide Container vorhanden. Nativer Modus statt
   Docker → an Kilian (dieser Wave ist Docker; native = separater E2.5a-Gate).

### Backup (AC-P2, vor jeder Änderung)
`tar czf /tmp/eink-data-backup-<UTC-ts>.tgz -C ~/E-INK-Picture data` + `sha256sum` in die
Evidenz. Off-device kopieren (nach Mac `artifacts/b-l3-wave/`). `data/auth.json` und
`data/hass.json` werden dabei **nur gelesen/gesichert, nie neu geschrieben**.

### Deploy (AC-P3)
Aktuellen gemergten `main` via existierender Docker-Local-(All-in-one)-Modus:
`cd ~/E-INK-Picture && git fetch && git checkout main && git pull --ff-only &&
docker compose up -d --build`. `GET /health` liefert 200 + den erwarteten Versions-Stempel
(Deploy-Beweis). Beide Container `healthy`. **E2.5a native Bring-up ist NICHT Teil dieses
Waves** (eigener Gate; nur erwähnt, falls Kilian ihn ausdrücklich einfaltet).

### Rollback
- Deploy fehlerhaft → `git checkout <vorheriger-HEAD> && docker compose up -d --build`,
  oder `docker compose up -d` (alte Images) wenn nur Rückkehr nötig.
- `data/` beschädigt → **nur mit Kilians Freigabe** aus `/tmp/eink-data-backup-*.tgz`
  zurückspielen; `data/auth.json`/`data/hass.json` nie ohne Freigabe überschreiben.
- Nichts Destruktives; nichts außerhalb `/tmp` + Projektverzeichnis schreiben.

### Panel-Schonung (harte Regel, gilt für ALLE Phasen)
- **Max. 1 Panel-Write pro Minute.** `epd.sleep()` läuft nach jedem Write automatisch
  (`client.py:302`) — verifizieren, nicht erzwingen.
- Reihenfolge: **Phase A (keine neuen Writes)** → Phase B (genau 1 Baseline-Write) →
  Phase C (≥60s beabstandete getriggerte Writes). Zielsumme: **1 Baseline + 3 getriggerte
  Writes = 4 Panel-Writes** für die gesamte Wave. B3-Timing wird in einen der Trigger aus
  Phase C **hineingemessen** (kein eigener Write).

## Akzeptanzkriterien (nummeriert, messbar)

### Phase A — Nicht-Panel-Batch (ZUERST, null neue Panel-Writes)

- **AC-A1 — Ressourcen-Budgets erfasst.** Nach dem Deploy (Steady-State, ≥5 min nach
  erstem Cycle) werden erfasst: (a) Container-Totals `docker stats --no-stream`, (b)
  Prozess-RSS je Container via `docker compose exec server cat /proc/1/status | grep VmRSS`
  bzw. `… client …`. Beide Zahlen in die Evidenz.
- **AC-A2 — Client-Budget (harter Gate, beide Modi).** Client-Prozess-RSS **≤ 60 MB** →
  **FAIL bei > 60 MB**. (Docker-Baseline ~35–36 MB, komfortabel darunter.)
- **AC-A3 — Server-Budget (kontext-korrekt).** Die native Vorgabe (Server-Prozess-RSS
  ≤ 25 MB) gilt für den **nativen** Betrieb; unter dem **Docker-Deploy dieser Wave** liest
  der Server-Prozess-RSS erfahrungsgemäß ~45–48 MB (bekanntes Docker-Accounting-Artefakt,
  PROGRESS HIL-2, „nicht 1:1 vergleichbar"). Daher:
  - Der 25-MB-Server-Gate wird **erfasst, aber unter Docker nicht als FAIL gewertet** und
    ist für die echte Messung an die separate **E2.5a-Native-Wave DEFERRED** (dort
    Native-RSS 18,3 MB < 25 MB bereits belegt).
  - **Docker-Regressions-Gate (harter FAIL):** Server-Prozess-RSS **> 60 MB** ODER
    Client **> 60 MB** (materiell über den erfassten Docker-Baselines) → FAIL.
  - Sanity: Summe beider Container-Totals **< 150 MB** (512-MB-Pi-Headroom).
- **AC-A4 — App + Designer erreichbar (B5-live Vorbedingung).** `GET /health` = 200,
  `GET /designer` = 200 (Session vorhanden). Damit ist die Bühne bereit, dass **Kilian**
  das Token via UI eingibt.

### Token-Sicherheit — eigener Abschnitt (siehe unten, VOR Phase B/C für B5)

### Phase B — B6 Content-Skip-Nachweis (genau 1 Baseline-Write)

- **AC-B6.1 — Statisches Design.** Ein **rein statisches** Design (Bild + statischer
  Text/Shapes, **KEINE** Live-Widgets wie weather/hass/clock — deren re-fetch würde den
  Content ändern) wird als aktives Design gesetzt. Begründung protokollieren.
- **AC-B6.2 — Kurzintervall für schnelle Zyklen.** `refresh_interval` temporär auf **60 s**
  (`POST /update_settings`), damit ≥5 Intervall-Zyklen in ~6 min beobachtbar sind. Vorher-
  Wert notieren, nach Phase B **wiederherstellen**.
- **AC-B6.3 — Baseline-Write.** Der erste Cycle nach Setzen des Designs schreibt genau
  **einmal** aufs Panel (`"Display updated successfully"` × 1) und legt
  `eink_last_sent.png` an. Dieser eine Write ist erwartet (kein Vor-Hash / `_initial_
  display_done` false) und zählt NICHT gegen B6.
- **AC-B6.4 — Null Writes über N Zyklen (Kern-AC).** Über ein Fenster mit **≥5**
  Intervall-Poll-Zyklen nach dem Baseline-Write gilt, **verifiziert im Client-Log
  (`docker compose logs --timestamps client`)**:
  - Anzahl `"skipping panel refresh (content unchanged)"` **≥ 5**,
  - Anzahl zusätzlicher `"Display updated successfully"` **= 0** (expliziter Skip-Count:
    **0 Writes**),
  - `eink_last_sent.png`-mtime **unverändert** über das gesamte Fenster:
    `docker compose exec client stat -c %Y <LAST_SENT_PATH>` vor == nach.
  (Beweist zugleich, dass der Intervall-Refresh `reason="interval"` korrekt geskippt wird —
  die Invariante, auf der B3 aufsetzt.)
- **AC-B6.5 — Aufräumen.** `refresh_interval` auf den in AC-B6.2 notierten Vorher-Wert
  zurückgesetzt.

### Phase C — Panel-Gates (≥60 s beabstandet, je 1 Write)

- **AC-B2.1 — rounding.json aufs Panel.** `rounding.json` als aktives Design setzen,
  **ein** manueller Trigger (`POST /api/trigger_refresh`) → 1 Panel-Write. Dieser Trigger
  ist zugleich die **B3-Timing-Messung** (AC-B3.*). `eink_last_sent.png` danach via
  `docker compose cp client:<LAST_SENT_PATH> …` ziehen.
- **AC-B2.2 — Palette-Integrität.** Das gezogene `eink_last_sent.png` hat **≤ 6** unique
  RGB-Werte, alle ∈ `{#000000,#FFFFFF,#FF0000,#FFFF00,#00FF00,#0000FF}` (Palette-Zensus
  wie E1.2). FAIL sonst.
- **AC-B2.3 — Panel == Server-Render (Bytes erreichen das Panel intakt).** Server-
  `GET /preview` für dasselbe aktive `rounding.json` (authentifiziert) ziehen; da
  `rounding.json` **keine** Live-Widgets enthält, ist der Render deterministisch → das
  panel-gebundene `eink_last_sent.png` ist **pixel-identisch** zum Server-`/preview`
  (0 abweichende Pixel; Toleranz 0). FAIL, wenn der Client das Bild verändert.
- **AC-B2.4 — Geometrie-Fix im deployten Binary live.** Auf `eink_last_sent.png` per
  Kanten-Scan an einer bekannten Rundung aus `rounding.json` (bekanntes `R`): der
  Fill-Beginn liegt bei `x+R (±3 px im 800×480-Panelraum)` — also Radius ≈ `R`, **nicht**
  `R/2`. Beweist, dass der B2-Radius-Fix (D0) im deployten Server steckt.
- **AC-B2.5 — Visuelle Parität + L4-Flag.** `eink_last_sent.png` wird neben
  `testdata/parity/rounding__canvas_ref.png` (bzw. einen frischen Designer-Screenshot
  desselben Designs) gelegt; Reviewer bestätigt visuell gleicher Radius / zentrierter
  Stroke. **L4 (physisches Panel-Foto) ist auf diesem Pi UNMÖGLICH (keine Kamera) und MUSS
  in den Ergebnissen ausdrücklich als „L4 offen — keine Kamera" markiert werden, nicht
  still übersprungen.**

- **AC-B3.1 — Trigger→Panel-Init < ~2 s.** Gemessen am Trigger aus AC-B2.1 (und wiederholt
  auf dem B1+B5-Trigger AC-C.*): `t0` = Docker-Log-Timestamp der Server-Zeile
  `path=/api/trigger_refresh method=POST` (`docker compose logs --timestamps server`),
  `t1` = Docker-Log-Timestamp der Client-Zeile `"Initializing display..."`
  (`docker compose logs --timestamps client`). **`t1 − t0 < 2,0 s`** für **jeden** von
  **≥3** manuellen Triggern; min/median/max in die Evidenz.
- **AC-B3.2 — Long-Poll-Beweis.** Im Server-Log fällt die `duration` des zuvor geparkten
  `GET /api/refresh_status` beim Trigger von den ~25-s-Idle-Holds auf einen kurzen Wert —
  belegt, dass der Long-Poll sofort zurückkehrt (kein 30-s-Poll-Gap). Wert notieren.
- **AC-B3.3 — Manueller Trigger wird NIE geskippt.** Jeder Trigger produziert eine
  `"Initializing display..."`-Zeile (nie `"skipping panel refresh"`) — bestätigt den B6-
  Guard `reason != "interval"`.

- **AC-C1 — B1+B5-Kombi-Design (1 Write für beide Gates).** **Kilian** wählt im Designer
  über die **neue Standortsuche** (Autocomplete) einen Ort für ein weather+forecast-Widget
  und platziert ein `widget_hass` in den drei Modi (temperature/alarm/presence) mit den
  von ihm gelieferten Entity-IDs. Design als aktiv setzen, **ein** Trigger → 1 Panel-Write.
  `eink_last_sent.png` + `GET /preview` ziehen.
- **AC-B1.1 — Ort erscheint korrekt (B1).** Das gerenderte weather/forecast-Widget zeigt
  den per Suche gewählten `locationName`-Label **und** eine plausible aktuelle Temperatur;
  im gespeicherten Design-JSON stehen `lat`/`lon`/`locationName` des gewählten Orts.
- **AC-B1.2 — lat/lon lösen den richtigen Ort auf (Gegenprobe).** Die gerenderte
  Temperatur/WMO-Bedingung für die gewählten `lat`/`lon` stimmt mit einer manuellen
  open-meteo-Abfrage (`api.open-meteo.com/v1/forecast?latitude=…&longitude=…&current=…`)
  für dieselben Koordinaten am selben Tag überein (±2 °C bzw. gleicher WMO-Code) — beweist,
  dass die Suche echte Koordinaten setzt, nicht einen Default.
- **AC-B1.3 — Mobil-Viewport = manuell/L2, nicht auto-verifiziert.** Die Autocomplete-
  Interaktion selbst ist browserseitig (L2/manuell). Ein echter <768 px Browser-Viewport
  war in-Session **nicht** erreichbar (`specs/B7-repro-protocol.md` §2, „shrink below
  ~1710 CSS px" nicht möglich) → in den Ergebnissen als „Mobil-Viewport manuell/nicht
  automatisiert verifiziert" flaggen, nicht als PASS behaupten.
- **AC-B5.1 — HA-Widget zeigt LIVE-Werte (B5-live).** Nach Kilians Token-Eingabe (siehe
  Token-Sicherheit) rendert `widget_hass` je Modus **echte** Daten und **keinen** der
  Graceful-Fallbacks (`"HA nicht konfiguriert"`, `"Nicht verfügbar"`,
  `"Unbekannt: <id>"`):
  - temperature: numerischer Wert + Einheit (z. B. `21.5°C`),
  - alarm: deutscher Zustandstext aus `germanHassAlarm` (z. B. `Unscharf`/`Scharf (…)`),
  - presence: `Zuhause`/`Abwesend`/Zonenname bzw. `N zuhause`/`Niemand zuhause`.
  Sichtbar auf `eink_last_sent.png` **und** `GET /preview`.
- **AC-B5.2 — Live-Gegenprobe durch Kilian.** Kilian liest **einen** Wert (Temperatur) aus
  seinem HA-Dashboard vor; er stimmt mit dem gerenderten Wert überein. **Der Validator
  fragt HA NIE selbst ab** (er hat das Token nicht).
- **AC-B5.3 — Budget während Live-Fetch.** Während `widget_hass` live fetcht bleibt der
  Server-Prozess-RSS innerhalb AC-A3 (kein Speicher-Sprung durch den HA-Fetch im
  Render-Pfad). Wert notieren.

## Token-Sicherheit (PRIMÄR und verbindlich — eigener Abschnitt)

**Grundregel: `hardware-validator` sieht/erfragt/echoed/loggt/speichert das Token NIE.**

- **AC-TOK1 — Eingabe nur durch den User via Web-UI.** Das HA-Long-Lived-Token wird
  ausschließlich von **Kilian** im laufenden HA-Settings-Modal eingegeben
  (`POST /api/hass/config` → `data/hass.json`, 0600). Der Validator öffnet die Modal-Bühne
  (App erreichbar, AC-A4), tippt aber selbst **nichts** ein und liest das Token nirgends.
- **AC-TOK2 — Dateirechte.** Nach der Eingabe: `stat -c '%a' data/hass.json` == **600**.
- **AC-TOK3 — GET-Body ohne Token.** `GET /api/hass/config` liefert `configured:true`,
  `token_set:true`, aber **niemals** den Token-String (kein `token`-Wertfeld, kein
  `eyJ…`, kein `Bearer`). Response-Body in die Evidenz (er enthält per Design kein Token).
- **AC-TOK4 — Token in KEINEM Log.** HA-Long-Lived-Tokens sind JWTs (Präfix `eyJ`).
  `docker compose logs server client 2>&1 | grep -aE 'eyJ[A-Za-z0-9_-]{10,}|Bearer |Authorization:'`
  → **0 Treffer**. Zusätzlich alle Datei-Logs / `journalctl -u eink*` (falls vorhanden)
  mit demselben Muster prüfen → 0. (Der Validator braucht den Token-Wert dafür NICHT zu
  kennen; das Muster erkennt jedes Leck. Das Code-Level-No-Log ist bereits per B5-AC-SEC2
  grep-Gate in L1 erzwungen — dies ist der betriebliche Beleg.)
- **AC-TOK5 — Token NUR in `data/hass.json`.** `grep -rlaE 'eyJ[A-Za-z0-9_-]{20,}' data/`
  → **genau** `data/hass.json` und **nichts sonst**; dasselbe Muster über den Repo-Baum
  außerhalb `data/` → 0 Treffer.
- **AC-TOK6 — Keine Parallel-Session zerstören.** `data/auth.json` und `data/hass.json`,
  die Kilian in einer Parallel-Session gesetzt hat, werden **nie** überschrieben oder
  gelöscht; das `data/`-Backup ist eine Nur-Lese-Kopie.
- **AC-TOK7 — Fallback dokumentiert, NICHT genutzt.** Das Token existiert außerhalb des
  Repos auch unter `/Users/ksch/Documents/002 - Unterlagen/001 - Wohnung/homeassistant/
  secrets.local.md` — dies wird **nur dokumentiert**; der Web-UI-Eingabepfad ist
  vorzuziehen, damit **kein Agent das Secret anfasst**. `hardware-validator` **öffnet diese
  Datei NICHT**.
- **Entity-IDs + Alarm-Backend** (`alarm_control_panel.*` vs. `alarmo`) sind **User-Inputs
  zur Testzeit** (via UI), hier bewusst nicht hartkodiert; unbekannte Alarm-States fallen
  panelseitig auf den rohen State-String zurück (B5 AC-HA2), nie Panic/leer.

## Non-Goals (ausdrücklich NICHT in dieser Wave)

- **KEIN L4 (physisches Panel-Foto).** Der Test-Pi hat keine Kamera → L4 ist unmöglich und
  wird als „offen — keine Kamera" geflaggt (AC-B2.5), nicht ersatzweise erfunden.
- **KEIN E2.5a Native-Bring-up.** Der native `setup.sh`-Pfad (lgpio/GPIO) ist ein
  **separater** Gate; dieser Wave ist reiner Docker-Deploy. Nur einfalten, wenn Kilian es
  ausdrücklich anordnet.
- **KEIN Pillow-/requests-Client-Bump-Retest** und **kein** Go-1.25-Deps-Render-Retest —
  eigene Runden (PROGRESS „Go-1.25-Deps-Runde vor Release").
- **KEIN** Release-Tag, **kein** Worktree-Housekeeping, **kein** Doku-Sweep.
- **KEINE Code-Änderung** an Server oder Client — dieser Spec beauftragt ausschließlich
  Messung/Verifikation. Findet der Validator ein Gate rot, wird es **berichtet**, nicht
  „schnell gefixt".
- **KEIN** Anfassen von `data/auth.json`/`data/hass.json`, kein Schreiben außerhalb
  `/tmp` + Projektverzeichnis, nichts Destruktives.

## Verifikation (Gates + konkrete Kommandos)

- **L1/L2 (Voraussetzung, bereits grün auf `main`):** B2/B3/B5/B1 sind statisch + im
  Browser/Golden verifiziert; diese Wave setzt das voraus und misst nur L3.
- **L3 (dieser Wave, `hardware-validator` auf dem Pi):**
  - Selbstprüfung: `cat /proc/device-tree/model`, `docker compose ps`,
    `docker compose exec client env | grep EINK_DISPLAY_DRIVER`.
  - Deploy: `docker compose up -d --build`, `curl -s localhost:5000/health`.
  - B3-Timing: `docker compose logs --timestamps server | grep trigger_refresh` ↔
    `docker compose logs --timestamps client | grep "Initializing display"`; Delta bilden.
  - B6: `POST /update_settings` (`refresh_interval=60`), Fenster beobachten,
    `docker compose logs client | grep -c "skipping panel refresh"` und
    `… grep -c "Display updated successfully"`, `docker compose exec client stat -c %Y
    <LAST_SENT_PATH>` vor/nach; Intervall zurücksetzen.
  - B2: `docker compose cp client:<LAST_SENT_PATH> /tmp/…`, Palette-Zensus (unique RGB),
    Pixel-Diff gegen `GET /preview`, Kanten-Scan Radius.
  - B5/B1: nach Kilians UI-Eingaben `eink_last_sent.png` + `/preview` ziehen; Inhalt gegen
    Erwartung (kein Fallback-String; open-meteo-Gegenprobe für B1).
  - Token: `docker compose logs server client | grep -aE 'eyJ…|Bearer |Authorization:'`
    (0), `grep -rlaE 'eyJ…' data/` (nur hass.json), `stat -c %a data/hass.json` (600),
    `curl -s .../api/hass/config`.
- **L4:** unmöglich (keine Kamera) — geflaggt, nicht übersprungen.
- **L5 (Review):** Reviewer prüft die Evidenz-Mappe: alle ACs mit Zahl/Artefakt belegt,
  Token-Grep sauber, L4-Flag gesetzt, Panel-Write-Count ≤ 4, keine Parallel-Session
  beschädigt.

## Risiken

- **Pi bleibt offline / IP falsch.** → PC1 + AC-P1 blockieren; kein Scan, kein Fremdgerät.
  Rollback: nichts deployt, Wartestellung.
- **Autonomer Intervall-Refresh verfälscht B6-Fenster.** Mitigation: B6 nutzt ein statisch-
  es Design; Intervall-Refreshes werden korrekt geskippt (das ist genau der Nachweis).
  Kurzintervall (60 s) beschleunigt nur die Zyklen, erzeugt **keine** Writes (Skip).
- **Docker-RSS über 25 MB (Server).** Erwartet + bekanntes Artefakt (AC-A3); nur > 60 MB
  ist ein FAIL. Der echte 25-MB-Native-Gate ist DEFERRED an E2.5a.
- **Token-Leck über einen übersehenen Pfad.** Mitigation: AC-TOK4/TOK5 (JWT+Bearer-Grep
  über Logs und `data/`), plus das L1-grep-Gate aus B5. Rollback bei Leck: `data/hass.json`
  löschen **nach Kilians Freigabe** invalidiert das Token sofort — der Validator tut das
  nicht eigenmächtig (AC-TOK6).
- **Panel-Verschleiß.** Max. 1 Write/min, `epd.sleep()` nach jedem Write; Zielsumme 4
  Writes; B3 in bestehende Trigger gemessen (kein Extra-Write).
- **`data/`-Beschädigung durch Deploy.** Backup (AC-P2) + Rollback nur mit Kilians
  Freigabe; `auth.json`/`hass.json` unangetastet.
- **HA unerreichbar / Entity-Tippfehler zur Testzeit.** Panel zeigt Graceful-Fallback
  (kein Crash); AC-B5.1 schlägt sichtbar fehl → als „B5-live rot (HA/Entity)" berichten,
  nicht raten.

## Evidence / Reporting-Checkliste

Alle Artefakte unter `artifacts/b-l3-wave/` (Projektverzeichnis) bzw. `/tmp` ablegen;
Messwerte zusätzlich im PR/PROGRESS-Eintrag festhalten.

- [ ] **Selbstprüfung:** Modell, Panel-Treiber, IP/MAC/Repo-HEAD, `docker compose ps`.
- [ ] **Deploy:** Commit-SHA (`git log -1`), `/health`-Version, Backup-Pfad + `sha256`.
- [ ] **AC-A (Budgets):** `docker stats`-Total + Prozess-VmRSS je Container; PASS/FAIL
      gegen AC-A2/A3; Server-25-MB-Zeile als „DEFERRED (Docker-Artefakt)".
- [ ] **B6:** Skip-Count (≥5), zusätzlicher Write-Count (**0**), `eink_last_sent.png`-mtime
      vorher==nachher, verwendetes statisches Design, Intervall vorher/wiederhergestellt.
- [ ] **B3:** ≥3 Deltas `t1−t0` (min/median/max, alle < 2,0 s), Long-Poll-`duration`-Beleg,
      „manueller Trigger nie geskippt".
- [ ] **B2:** `eink_last_sent.png` (gezogen), Palette-Zensus (≤6, alle ∈ Palette),
      Pixel-Diff gegen `/preview` (0), Radius-Kanten-Scan (`x+R ±3px`), Side-by-side vs.
      `rounding__canvas_ref.png`. **L4-Flag: „keine Kamera — Panel-Foto offen".**
- [ ] **B1:** gewählter `locationName` + `lat`/`lon` aus dem Design-JSON, gerenderter
      Ort/Temp, open-meteo-Gegenprobe (±2 °C/WMO), **Mobil-Viewport = manuell/nicht
      auto-verifiziert** geflaggt.
- [ ] **B5-live:** je Modus der gerenderte Live-String (kein Fallback), Kilians
      Temperatur-Gegenprobe, Budget während Fetch.
- [ ] **Token-Sicherheit:** `stat` 0600, `GET /api/hass/config`-Body (ohne Token),
      Log-Grep (0), `data/`-Grep (nur `hass.json`), Bestätigung „secrets.local.md NICHT
      geöffnet", „Parallel-Session-Dateien unberührt".
- [ ] **Panel-Write-Bilanz:** Gesamtzahl `"Display updated successfully"` über die Wave
      (Ziel ≤ 4), alle ≥ 60 s beabstandet, `epd.sleep()` je Write belegt.
- [ ] **Gesamt-Verdikt** je Gate (PASS/FAIL/DEFERRED) + Rollback-Status.
