# PROGRESS

Manager-geführtes Log für den Weg zu v1.0. Nach jedem Task aktualisiert.
Gates: L1 statisch | L2 Render-Verifikation | L3 Hardware-in-the-Loop | L4 Panel-Foto | L5 Review.

## Finishing-Runde v1.0 — Bug-/Feature-Backlog B1–B7 (2026-07-16, Manager)

**Ist-Stand verifiziert** (read-only Workflow `wf_6f2fa7c5-1d0`, 4 Agents, 0 Fehler): Alle §1-Behauptungen gegen `main` **bestätigt**. HEAD = `3ecf4ce` (= origin/main, tree clean) — der Header unten nennt noch `27f2643`, ist also 2 Docs-Commits veraltet (1d87c52 README + 3ecf4ce E6.3). E1–E6 gemergt (Belege: auth/, negcache.go, memlimit.go, widgets/*.go, quantize.go, calibration.go, ci.yml+release.yml). E2.5a-Fix in setup.sh vorhanden, **native L3 weiter AUSSTEHEND**. Deps aktuell: client `requests>=2.31.0,<3` / `Pillow>=10.0.0,<13` / `lgpio>=0.2`; go.mod `go 1.24.0`, `x/crypto v0.43.0`, `x/image v0.36.0`, `x/text v0.34.0` — 17 Dependabot-Vulns offen. Alle 6 HW-/Kilian-Gates offen; `git tag -l` leer.

**⚠️ Test-Pi OFFLINE (2026-07-16):** `.106` + `.121` antworten weder SSH noch ICMP (ARP incomplete); LAN sonst gesund (Gateway 0 % Loss, Nachbarn .101/.103/.105 da) → Pi stromlos oder neuer DHCP-Lease. hardware-validator protokollgerecht gestoppt (kein Scan, nichts verändert). **Blockiert alle L3-Gates** (B3-Timing, B6-Skip-Nachweis, E2.5a-nativ, Deps-Pi-Retest, HW-Gates). Kilian: Pi-Strom/LED prüfen + IP aus Router-DHCP holen. Software/L1+L2 läuft Pi-unabhängig weiter.

**Reihenfolge diese Runde (Software-first, solange Pi aus):** B7 → B3(-Spec) → B4 → B2 → B1 → B5; L3-Anteile in Warteschlange bis Pi zurück.

| Bug | Verdikt | Grund-Ursache (Beleg) | Gate | Status |
|---|---|---|---|---|
| **B1** Wetter Standort-/PLZ-Suche | confirmed | Backend `SearchLocation` da (weather.go:445-497, Nominatim), Route `main.go:217` (nicht 221); aber `LocationResult` (weather.go:49-54) liefert **nur display_name/lat/lon** → Enrichment nötig (addressdetails=1, type/postcode). Panel `properties-panel.js:775-785` nur lat/lon. Referenz-UI: `setup-wizard.js:303-421`. Auth: session-pflichtig, expired→/login-Redirect (type-ahead-Falle). | L1+L2+Review, mobil | offen; **✅ Nominatim+enrich (Kilian)** |
| **B2** Rundungen/Parität Panel↔Designer | corrected | Zwei Impls. Divergenzen: (1) single-rx (panel ignoriert ry), (2) Stroke inside-box (preview.go:934-951) vs Fabric centered-edge, (3) **NEU: rx unskaliert exportiert, w/h skaliert** — Shapes sind der einzige Typ, dessen scale nicht in object:modified absorbiert wird (canvas-manager.js ohne shape-branch) → Radius halbiert bei skalierten Shapes, (4) AA nur bei Fast-Quality echt fehlend (sonst Supersample+CatmullRom), (5) Koord-Mapping 1:1 (einkOffset toter Code — Hypothese widerlegt). | L2 Canvas↔Panel-Diff + L3-Foto | offen |
| **B3** Refresh sofort | confirmed | Pull-Poll-Latenz: POLL_INTERVAL=30 (config.py:7), Trigger setzt State sofort (settings.go:305-306), Client sieht's erst beim nächsten Poll (client.py:508-517). Long-Poll **sicher** (refresh_status nutzt NICHT das Render-Semaphore); echte Grenze WriteTimeout=30s (main.go:318) → Hold <~25s. `/refresh-display` PUSH-Pfad ist tot (Client hat keinen HTTP-Server). | L1+L3 (<2s auf Pi) | Spec zuerst |
| **B4** Widget-Parität | confirmed | Systemisch. Kalender-Titel: Zwei-Schichten-Drop (client passthrough widgets.js:232 + handler widgets.go:99-104 forwardet title nie); panel rendert (preview.go:667-669). Voll-Matrix erstellt (~38 Zeilen, s.u.). | L1+L2/Widget+Review | offen; **✅ Vereinheitlichen (Kilian)** |
| **B5** Home Assistant | greenfield | Existiert nicht. Einbau in generische widget_*-Pipeline; Single-Source empfohlen (`fillHassContent` + canvas-passthrough /api/widgets/hass). | L1+L2+L5 Security-APPROVE | zustimmungspflichtig (Scope+Token+SSRF) |
| **B6** nur bei Änderung | **DONE (verifiziert)** | Content-Skip vollständig+korrekt (client.py:209-216 SHA-256 der Wire-Bytes, 393-426 skip, 24h-Guard MAX_SKIP_HOURS). `reason!="interval"`-Scope **absichtlich** (manuelle Trigger schreiben immer) — exakt die Invariante, die B3 braucht. **Kein Code nötig.** | L3 Skip-Zählung | Code-Beweis da; HW-Retest pendet Pi |
| **B7** Vorschau zeigt nichts | corrected | Manager-401-Kette **widerlegt** (auth.js:12-24 macht aus 401 einen /login-Redirect, kein stiller Broken-Image). Beste Kandidaten: **(1) fabric.js vom CDN (designer.html:450) → offline/LAN-Pi → CanvasManager.init wirft VOR Toolbar.init → Button nie verdrahtet**; (2) Session-Expiry→/login; (3-5) 403/500/leeres img (kein onerror, designer.html:290). Mobil/Tablet: Button in Burger-Overflow (responsive-layout.js:17,23). | L1+Review, beide Auth-Zustände × beide Viewports | offen; **Live-Repro zuerst** |

**Widget-Paritäts-Matrix (B4-Bauplan, Kern-Drifts):** verticalAlign (canvas ignoriert), fontFamily (kein Widget-Selector; canvas monospace vs panel Noto/DejaVu), weather/forecast no-data (canvas **erfindet Fake-Daten** vs panel „No data"), clock timezone (canvas Browser-Zeit), calendar title/maxEvents(hardcoded 10)/layout/showTime(panel ignoriert!), news title/showDescription/layout, system showLabels(hardcoded true)/compact+custom-layout, custom url/jsonPath (canvas **fetcht nie**, zeigt Fake ‚42'), text strikethrough (tot), text fontFamily (Web-Fonts die der Server nicht hat). Voll-Matrix im Workflow-Journal `wf_6f2fa7c5-1d0`.

**Architektur-Empfehlung (an Kilian eskaliert):** Widget-Text-Content auf **eine** Quelle kollabieren = Go-Renderer (preview.go fill*), Canvas als Passthrough (/api/widgets/<type> → {content} verbatim), dasselbe Format-Code für Panel + Editor. Entfernt die B4-Driftklasse strukturell. Kosten ~1–2 Tage. clock/timer bleiben client-live (Sekundentakt). B2 separat (Go-Raster vs Browser-Vektor — Parität = Spec matchen, nicht Code teilen, ~0.5–1 Tag). Font-Drift eigener Folgetask.

**Entscheidungen Kilian (2026-07-16):** (1) **B4/B5 = Vereinheitlichen** — Go-Renderer wird einzige Content-Quelle, Canvas rendert Server-`{content}` verbatim; clock/timer bleiben client-live; Golden/offline-Tests neu baselinen. (2) **B1 = Nominatim + anreichern** — addressdetails=1 für Adress-/PLZ-Ebene, Debounce + serverseitiger Cache Pflicht (1 req/s + User-Agent).

**Session #2 (2026-07-16, Manager-Fortsetzung):** Backlog + Kilian-Entscheidungen unabhängig re-verifiziert (3 read-only Agenten: State/Repro B1–B7, B4-Voll-Matrix, Pi-Baseline — deckungsgleich, keine inhaltlichen Korrekturen, nur Zeilennummern-Feinschliff). Pi weiterhin offline (`.106`/`.121` tot, Gateway ok). Implementierung startet Software-first: **B7-Live-Repro (AC0) zuerst** → B3-Long-Poll (L1; L3 <2s deferred bis Pi) → B4-Vereinheitlichung (Go Single-Content-Source, Canvas-Passthrough) → B2-Parität → B1-Standortsuche → B5-Home-Assistant (Security-Vorlauf). Specs B3/B7 (+ B4-Matrix) werden mit dem ersten Implementierungs-Branch auf main gelockt (aktuell untracked/uncommitted). Offen an Kilian: Pi-Strom/IP, Dependabot-Liste (Security-Tab), B5-Scope+Token-Modell.

**Pi wieder online (2026-07-16, 10.33.0.106):** Baseline read-only bestätigt — Pi 3B / Debian 13 Trixie / Kernel 6.12.47 / `epd7in3e` / Docker (server+client healthy, HEAD `b061b85`) / keine Kamera / RAM Docker Server 47,5 MB·Client 35,8 MB / `data/` 6,7 MB unangetastet / **0 Panel-Writes**. `vcgencmd get_throttled=0x50000` = **Unterspannung seit Boot aufgetreten** (aktuell inaktiv) → PSU/Kabel bleiben verdächtig (an Kilian). **Alle L3-Gates entblockt.**

**B7-Repro abgeschlossen (AC0 ✅, 2026-07-16):** Live reproduziert. Bestätigte Mechanismen: **[1]** fabric-CDN offline → `ReferenceError` `canvas-manager.js:15` VOR `Toolbar.init` (designer.js:52) → Klick inert (exakter Stacktrace); **[2]** Session expired → 401 → `/login`-Redirect; **[5]** `#preview-image` ohne `onerror` (designer.html:290) → stilles leeres Bild; **[3]** Cross-Origin 403 serverseitig bestätigt (nur hinter Reverse-Proxy). Endpoint-Statuscodes verifiziert (preview_live 200 same-origin/403 foreign/401 unauth; /preview 200-Session·401-ohne·200-Token) = **AC3-Testkontrakt**. Tablet/Mobile nicht live shrinkbar → Burger-Ausblendung per Code+CSS bestätigt (by design, kein Bug). **B7b (fabric+Fonts lokal via go:embed) = eigener Spec empfohlen** ([1] = Kern-Ursache Offline-Pi). Protokoll: `specs/B7-repro-protocol.md`. → B7-Fix (frontend-designer) adressiert AC1 onerror + fabric-undefined-Hinweis + 403/401-Mapping; AC3-Test durch test-engineer.

**Umsetzung B7 + B3 (2026-07-16, Session #2):**
- **B7 DONE** — gemerged `main`@`1e46964` (fast-forward). `toolbar.js` onerror→sichtbarer `#preview-status`-Fehler (kein Loop, kein False-Fire), `designer.js` fabric-undefined-Banner (role=alert, early-return VOR CanvasManager.init), exakt-403-Mapping. AC3-Regressionstest (echter Guard+Handler, 7 Subtests). Gates: L1✅ (gofmt/vet/`go test -race`), L2✅ (Browser live: Banner + Statusfehler + Happy-Path ohne Regression), L5✅ APPROVE. Folge-Spec **B7b** (fabric+Fonts lokal via go:embed für echten Offline-Pi) offen.
- **B3 Server** (`feat/b3-server-longpoll`@`107bb49`): 2 unabhängige Reviews (test-engineer + code-reviewer) → Nebenläufigkeit korrekt (lost-wakeup-safe: Channel vor Status-Check erfasst; kein Lock im select; write-then-notify), race-clean, serverHold 25s<WriteTimeout 30s, renderSem-isoliert, AC1–12 PASS. **Merge-reif** (wartet auf Client-Fix, dann Merge zusammen + L3).
- **B3 Client** (`feat/b3-client-longpoll`@`17cf9b9`): **CHANGES-REQUESTED (Runde 2 läuft)**. Beide Reviewer fanden dieselbe **Busy-Loop-Regression**: Loop backt nur bei `poll_ok=False` ab, aber `should_refresh=true` kommt sofort; ein 2xx-Zyklus ohne Forward-Progress (kein Heartbeat — frischer Boot ohne Bild = unbounded / fehlgeschlagener Refresh = Restart-Storm) → 0-ms-Re-Poll, 100% CPU + hämmert Renderer (nicht auf main). Angepasste Loop-Tests (`process_refresh_cycle→False` gestubbt) maskierten es. Fix-Auftrag an pi-client: Backoff bei No-Progress (Heartbeat-Signal) + echter (nicht gestubbter) Loop-Regressionstest; Manual-Refresh-Erfolg bleibt sofort. L3 (<2s Button→Panel) folgt nach Client-Fix + Merge.

## Aktueller Stand (2026-07-15, nach E5.5-Merge)

E1 komplett | E2.1+E2.3 gemergt — **E2.5a native-GPIO-Fix gemergt (L1✅ L5✅), L3-Bring-up auf Pi AUSSTEHEND** (swig-lgpio-Build muss auf Trixie/Py3.13 real greifen) | E3 komplett + **E3.7a/b/c gemergt** (Sprachmix→Deutsch, forecast_days 7, Logout-Fix, favicon) | **E4 = kein Blocker** (Datenquellen-DoD schon erfüllt); **E4a Dead-Code-Cleanup gemergt** | **E5 software-komplett** (E5.1–E5.6) | E6.1+E6.2 gemergt | Branch: **main @ 27f2643**, alle Merges CI-grün. **→ Software-Backlog für die v1.0-DoD ist im Kern KOMPLETT.** Verbleibend: (a) **E2.5a-L3** = konsolidierter Pi-Deploy (E3.7+E2.5a+E4a) nativ, testet ob die „ein Befehl"-Installation jetzt hochkommt; (b) **Kilian-/Hardware-Gates**: Panel-A/B (E1.6), epd7in5_V2-B/W-Test, Pi Zero 2 W, 72h-Lauf, README-Foto, v0.9.0-RC-Tag (nur mit Freigabe).

**E5.5 (Offline-Hardening) gemergt @ 33059a4:** persistenter Wetter-Cache (data/cache/weather.json, atomar, "stale ok", restart-fest) + In-Memory-Negativ-Cache (2 min pro Quelle, spart 10-s-Timeout pro Render). Reviewer-Blocker (Custom-API-Fallback-Drift "Error" vs. "HTTP <code>") gefixt und durch scharfen Test abgesichert. Hygiene-Nachzug: data/cache/ gitignored + .gitkeep, damit der Runtime-Cache nie versehentlich committet wird. L1✅ L5✅ APPROVE; L3 (Offline-Verhalten auf dem Pi) offen → HIL-Lauf 3.

**Nächste Schritte:**
1. ✅ E3.7a/b/c gemergt + docs (main 1148a76), kombinierte Tests grün, **CI grün (success)**, Branches gelöscht
2. **Aktuelle Version für Kilian aufs Panel** (hardware-validator läuft: Path 1 nativ mit GPIO-Fix, sonst Docker-Rebuild von b061b85) — dann Kilians Panel-A/B (E1.6) in separater Session
3. **E2.5a native-GPIO-Fix (P0)** — setup.sh: lgpio bauen bzw. Pin-Factory erzwingen; L3 = nativer Bring-up auf dem Pi (schaltet zugleich E2.5 + native RAM-Messung E5.6 frei)
4. E4-Datenquellen (P1): Widget-Registry, CalDAV (URL-basiert, kein Key), neue Widgets — Google-OAuth braucht Kilians Keyed-API-Entscheid; E3.7 Rest-Feinschliff
5. Wenn alles nachweislich fertig + auf Pi verifiziert: v0.9.0-RC → Tag (Prozess laut release.yml; NUR mit Kilians Freigabe)
6. Nebenbei prüfen: 17 Dependabot-Vulns (7 kritisch) vor v1.0 aufräumen

## HIL-Lauf 2 (2026-07-15, Deploy cd053b4 → 5a093eb, Pi 10.33.0.106)

- **E5.6 L3 PASS***: Server-RSS settled 98,2 → **47,8 MB** (−51 %); GOMEMLIMIT + Semaphore aktiv laut Startup-Log. *Einschränkung: VmHWM unverändert (~109 MB) — Peak kommt aus dem Bild-Element-Resize-Pfad (dokumentiertes E5.6-Non-Goal) → Backlog „Bild-Element-Puffer-Diät". Das native 25-MB-Ziel bleibt für den Nativ-Betrieb zu messen (E2.5).
- **E1.6 L3 (technisch) PASS**: Kalibriertes Dithering nachweislich auf dem Panel (Artefakt pixel-identisch zur Server-Preview, exakt 6 Farben). Visuell: Foto drastisch ruhiger, Hauttöne warm, Schwarz entschieden. **Kilians A/B am physischen Panel bleibt das finale Urteil.**
- **E5.1-Übergang PASS**: Ohne Passwort alles offen wie vorher, 0×401 beim Client, stündliche Warn-Erinnerung wie designt.
- Keine Regressionen: Panel-Write 19,9 s, 333 Requests alle 2xx, 2 Panel-Refreshes gesamt (beide autonom). Evidenz: artifacts/hil-2/.

## HIL-Lauf 3 (2026-07-15, Pi 10.33.0.106) — Docker→Nativ-Umstieg **fehlgeschlagen (Software)**, Rollback ok

- **Backup:** data/ vor dem Umstieg gesichert + off-device verifiziert (artifacts/hil-3/eink-data-backup-20260715-220052.tgz, 6,58 MB). ✅
- **Native switch FAILED (Gate B):** `setup.sh --headless --update` lief ~19 min (apt, **Go-Build erfolgreich** → server/eink-server aus b061b85, venv ok), brach am **Display-Treiber-Check** ab. Root-Cause = **reine Software, KEIN HW-Defekt**: `lgpio`-Wheel baut nicht → `gpiozero` fällt auf die auf **Kernel 6.12 kaputte NativeFactory** (sysfs, `OSError [Errno 22]`) → `from waveshare_epd import epd7in3e` scheitert → Abbruch vor systemd-Install. Evidenz: artifacts/hil-3/setup-native-FAILED.log.
- **Gegenbeweis HW intakt:** unter Docker lädt exakt derselbe epd7in3e-Treiber, bespielt das Panel in **19,7 s**, geht sauber schlafen. Panel + SPI + Verkabelung OK.
- **Rollback:** `docker compose up -d` → beide Container healthy, UI erreichbar `http://10.33.0.106:5000` (offen, kein Passwort — keine .env). **Nuance:** Rollback nutzt die **alten Images (~5a093eb-Ära), NICHT b061b85** (kein `--build`). Für aktuelle Version: `docker compose up -d --build` oder nativer Fix.
- **eink_last_sent.png = exakt 6 Farben** (epd7in3e-Palette, 800x480) — Dithering erreicht das Panel korrekt (aus altem Image, aber Pipeline-Beweis). artifacts/hil-3/eink_last_sent.png.
- **Native RAM (L3 E5.6) BLOCKIERT** — native Services liefen nie. Docker-Messung (nicht vergleichbar): Server ~47,4 MB / Client ~35,9 MB.
- **Throttle:** vorher/nachher `0x70000` unverändert (nur historische Bits seit Boot, keine aktiven) — der schwere Build hat **keine** akute Unterspannung ausgelöst.
- **E5.2/E5.5 Spot-Checks inkonklusiv** (altes Image): Content-Skip verhaltensbasiert ok (1 Write / 8 Zyklen), aber kein expliziter Skip-Log-Marker; data/cache/weather.json nicht angelegt (Image älter als E5.5 oder kein Wetter-Widget).
- **NEU: v1.0-Blocker E2.5** — die native „ein Befehl"-Installation funktioniert auf Kernel-6.12-RaspiOS NICHT (GPIO-Toolchain). Fix nötig in setup.sh. Spec: specs/E2.5a-native-gpio-fix.md.

### HIL-3 Fortsetzung (b061b85 LIVE via Docker-Rebuild) — Grundwahrheit + L3-Gewinne

- **Kilian testbar:** `http://10.33.0.106:5000`, aktueller Code b061b85 (Docker frisch gebaut), Panel getrieben (19,7 s, exakt 6 Farben, artifacts/hil-3/eink_last_sent_b061b85.png). **Login nötig** — data/auth.json (bcrypt, mtime 22:48) offenbar von Kilian in Parallel-Session gesetzt. Reset falls unbekannt = auth.json löschen (Backup vorhanden, hardware-validator hat es NICHT angefasst → Freigabe nötig). Client-Token in .env gesetzt (Auth aktiv).
- **E2.5a-Grundwahrheit (Path 1 durchgetestet):** Auf Kernel 6.12 funktioniert **NUR `lgpio`** — `native`/sysfs raus (Errno 22), `rpigpio` scheitert an Edge-Detection (RuntimeError), `pigpio` kein Daemon, `lgpio`-Python-Modul fehlt (Wheel-Build gescheitert). Zusatz-Bug: gepinntes `waveshareteam/e-Paper` zieht **Jetson.GPIO**, dessen Shim RPi.GPIO überschattet. Docker geht, weil dort lgpio sauber baut + kein Jetson.GPIO. → **kein Fallback möglich, Fix MUSS lgpio baubar machen** (Spec AC1/AC1b aktualisiert).
- **✅ E5.6 Server nativ L3 BESTANDEN:** nativer Server-RSS **18,3 MB < 25 MB** (voll funktionsfähig gemessen im kurzen Native-Lauf). Client-Native-Steady-State offen (Treiber lud nativ nicht). Panel/HW nachweislich intakt, kein Under-Voltage durch Build.
- **✅ E5.2 Content-Skip L3 bestätigt** (verhaltensbasiert: 1 Write / 7 Zyklen; stiller Skip ohne INFO-Marker). **✅ E5.5 Offline-Cache L3 bestätigt** (data/cache/weather.json 4734 B on-device angelegt). **✅ E1.2/E1.6** eink_last_sent_b061b85.png exakt 6 Farben.
- **E2.5 native Gate: NICHT bestanden** (Panel nativ nicht treibbar) → wartet auf E2.5a lgpio-Fix. Reine Software.

## E3.7-Backlog (Feinschliff, gesammelt aus Verifikationen)

- ✅ **Topbar-Overflow / Logout (E3.7b, 86360ce):** Logout in allen Breiten erreichbar (Burger-Cutoff auf gemessene Fit-Breite 1569.98px). **NEU als Folgekandidat:** 1024–1569px zeigen jetzt das Burger-Menü statt der Desktop-Zeile — Laptop-Breiten (1366/1440) verlieren die geräumige Topbar. Ein echtes Topbar-Overflow-Redesign (kompaktere/icon-only Controls) wäre nötig, um die einzeilige Desktop-Ansicht bis ~1024px zu behalten. Kilians Design-Urteil abwarten.
- ✅ **favicon.ico (E3.7c, 67b03b9):** Route + PNG-Asset, öffentlich — Rauschen weg.
- Properties-Panel zeigt beim Guide-Snap transient die rohe Position (Kosmetik, dokumentiert in E3.4-Spec).
- Crop-Modal: Listener-Akkumulation bei Reopen via X/Overlay (vorbestehend, benign — E3.1-Review-Finding).
- Resize-Snap an Guides (bewusstes E3.4-Non-Goal, Folgetask-Kandidat).
- ✅ **Sprachmix im Renderer (E3.7a, 0d1e648):** Wochentage + Wetterbeschreibungen jetzt Deutsch (locale.go). **Rest-Folgekandidat:** JS-No-Data-Placeholder `22°C Sunny` (widgets.js:147) noch englisch — statischer Demo-Stub für den Kein-Daten-Zustand, kein WMO-Map-Fall, pre-existing.
- ✅ **forecast_days 4→7 (E3.7a, 0d1e648):** Backend liefert jetzt 7 Tage (URL + 2 Parse-Caps); week-planner voll bedient.
- Bild-Element-Puffer-Diät (E5.6-Non-Goal): VmHWM ~109 MB kommt aus dem Image-Resize-Pfad (Kernel-Temp pro Bild-Element, Worst Case ~150 MB transient bei großen Fotos) — Kandidat für E5-Folgetask, nötig fürs native 25-MB-Ziel bei Foto-Designs.
- Wizard Schritt 5: früher Karten-Tap kann mit laufendem Queue-Render überlappen (max 2 in flight gemessen; Server serialisiert eh — Fix: auf Queue-Leerlauf warten vor Erfolgs-Render).
- Dashboard-Use-Flow kopiert den inerten templateSlot:"location"-Marker mit (harmlos, Renderer ignoriert ihn).

## E4 — Live-Datenquellen: Scoping-Ergebnis (2026-07-15, Explore)

- **DoD „Wetter/Kalender/News/Uhr live + selbst-aktualisierend" ist bereits ERFÜLLT.** Alle Daten-Widgets (widget_clock, widget_weather, widget_forecast, widget_calendar/iCal, widget_news/RSS, widget_custom, widget_system, widget_timer) werden vom Panel-Renderer `preview.go` (fill*Content, Dispatch preview.go:335-377) **end-to-end** gerendert, sind über die Designer-Palette addierbar + URL-/Prop-konfigurierbar, und aktualisieren sich, weil jeder Fetch **zur Render-Zeit** läuft (Pull-Modell: Client pollt → /preview → re-render). Recency: Wetter-Positiv-Cache 30 min + E5.5-Negativ-Cache 2 min. Kein separater Scheduler nötig.
- **E4 reduziert sich auf Qualität/Konsolidierung (P1, KEIN v1.0-Blocker):** (a) totes `conditions.go` (239 Z., nie instanziiert), (b) totes `WidgetRenderer`-Interface + 8 leere Render()-Stubs (widgets/renderer.go), (c) 4 verwaiste widgets/-Structs (weather/forecast/clock/timer — Konstruktoren nie aufgerufen), (d) **Doppel-Implementierung**: Panel-Pfad (preview.go) vs. Editor-Preview-Pfad (widgets/ via /api/widgets/*) — zwei parallele Widget-Impls, die driften (widgets/-Kopien ohne E5.5-Negativ-Cache, andere Defaults), (e) `widgets.WeatherFetcher`-Interface hat 0 Implementierer, (f) System-Widget liest `/proc` des **Panel-Hosts** (im Cloud-Modus der Cloud-Server, nicht der Pi — fragwürdig). Spec: specs/E4a-widget-deadcode-cleanup.md.
- **Folge fürs v1.0-Bild:** Nach E2.5a-Merge ist der **Software-Backlog für die v1.0-DoD im Kern komplett** (Datenquellen ✅, Designer ✅, Auth ✅, Render/Kalibrierung ✅, Betrieb/Zuverlässigkeit ✅, CI ✅). Verbleibend: E2.5a-L3 (nativer Install auf Pi), E4a-Cleanup (optional/Qualität), und die **Kilian-/Hardware-Gates** (Panel-A/B E1.6, epd7in5_V2-B/W-Test, Pi Zero 2 W, 72h-Lauf, README-Foto, Release-Tag).

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
| E6.2 (Go) | Versions-Stempel: var version via ldflags, /health liefert version (Rollback-Beweis für E2.4) | L1✅ L5✅ APPROVE | c3a48a6 |
| E6.2 (CI) | release.yml: Tag → Gate → 4 gestempelte Binaries + sha256sums + install.sh, CHANGELOG-Notes (harter Fail), --verify-tag, Injection-sicher | L1✅ (actionlint, 5 Extraktions-Fälle, Injektions-Probe) L5✅ APPROVE; L2 = erster echter Tag (Manager-Folgeschritt) | f64fb70 |
| E5.4 | Treiber-Watchdog: Reset statt Crash (nie sleep über kaputten Bus), init==-1 als Fehler, Eskalation nach 3 Fehlzyklen an systemd, Initial-Retry (<2 min Recovery) | L1✅ (68 Tests, 5 Negativproben) L5✅ APPROVE; L3 (HIL) offen | c1313d8 |
| E5.5 | Offline-Hardening: persistenter Wetter-Cache (data/cache/weather.json, atomar CreateTemp+Rename, ≤1 Write/30min/Ort, fail-open, "stale ok" restart-fest) + In-Memory-Negativ-Cache (2 min/Quelle, Wetter+Forecast teilen 1 Versuch, Hit byte-identisch zum Direktfehler) + gitignore-Hygiene (data/cache/ + .gitkeep) | L1✅ (gofmt/vet/build + Services-Suite inkl. -race) L5: REQUEST_CHANGES (Custom-API-Fallback-Drift "Error" vs "HTTP <code>", Suite war blind) → Fix (exakter Fallback in failureEntry) test-first, Gegenprobe rot → APPROVE; L3 (Offline-Probe Pi) offen → HIL-3 | 33059a4 |
| E3.7a | Sprachmix behoben: deutsche Wochentage (voll + Mo/Di/… statt Byte-Slice) + deutsche WMO-Wetterbeschreibungen an der Quelle; neue locale.go als Single Source of Truth (preview.go/weather.go/JS konsumieren); forecast_days 4→7 (URL + 2 Parse-Caps) | L1✅ (gofmt/vet/build, go test ./… inkl. -race grün) L2✅ (Goldens unverändert — kein Golden hat Wetter-Widget; Determinismus re-verifiziert) L5✅ APPROVE (unabhängig, deutsche Ausgabe per Test bewiesen, kein Englisch-Leak) | 0d1e648 |
| E3.7b | Logout in allen Breiten erreichbar: Burger-Cutoff auf gemessene Fit-Breite 1569.98px angehoben (CSS-Media + JS mqTablet/currentMode konsistent) — 1024–1569px nutzen jetzt Burger statt klippender Zeile | L1✅ L2✅ (echter Chrome-Headless-Sweep 320–1920px, Logout erreichbar+klickbar im Ex-Loch, anonym nie sichtbar, Desktop ≥1570 unverändert) L5✅ APPROVE | 86360ce |
| E3.7c | /favicon.ico serviert (PNG, image/png), öffentlich allow-gelistet (publicRoutes) — beendet 404/401-Konsolenrauschen; Templates verlinken via <link rel="icon"> | L1✅ (gofmt/vet, Tests + -race) L5✅ APPROVE (anon-401-Leak durch Test mit gesetztem Passwort geschlossen) | 67b03b9 |
| E4a | Dead-Code-Cleanup: totes conditions.go (239 Z.) + WidgetRenderer-Interface/Stubs + 4 verwaiste widgets/-Structs (weather/forecast/clock/timer) + WeatherFetcher/WeatherResult — **487 Z. entfernt, 0 Verhaltensänderung** | L1✅ (gofmt/vet/build, go test inkl. -race, Golden byte-identisch) L5✅ APPROVE (unabhängig: alle 25 Symbole grep-tot bewiesen, 4 Editor-Widgets + preview.go-Panelpfad unberührt) | 1454fae |
| E6.3 (README) | v1.0-README: ehrliche Feature-Liste (an CHANGELOG/Code gemappt), Quick-Start nativ+Docker mit „nicht-HW-verifiziert"-Caveat, „editing needs internet"-Hinweis (CDN), Server-/Client-Config-Tabellen aus .env.example, veraltete Claims gefixt (~10MB→~18MB, „1-bit"→Panel-Palette, cron→systemd), Foto-/Screenshot-Platzhalter | Manager-Review (Config-Vars gegen .env.example geprüft, Feature-Claims verifiziert); **Panel-Foto + Screenshots = Kilian-Gate (L4)** | 1d87c52 |
| E2.5a | Nativer GPIO-Fix (setup.sh): lgpio erzwungen (apt python3-lgpio wo vorhanden, sonst **swig-Source-Build = Primärpfad auf Trixie/Py3.13**), Jetson.GPIO-Kontamination entfernt, GPIOZERO_PIN_FACTORY=lgpio via .env/systemd, **set-e-Abort behoben** (killte sonst das Skript vor Fallback/Gate/preview-only) | L1✅ (shellcheck sauber, test-setup 15/15 inkl. neuem Integrationstest, Pi-Dry-Run exit 0) L5: REQUEST_CHANGES (set-e-Abort machte rpigpio-Fallback/actionable Fehler/preview-only unerreichbar; python3-lgpio fehlt in Trixie) → Fix (return 0 + \|\| true, swig-first, Integrationstest rot→grün) → **APPROVE**; **L3 (nativer Bring-up auf Pi) AUSSTEHEND** | 27f2643 |

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
