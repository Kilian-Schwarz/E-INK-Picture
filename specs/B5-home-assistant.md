# B5: Home-Assistant-Widget (Live-Daten: Temperatur / Alarm / Anwesenheit)

## Ziel
Nach diesem Task existiert ein neues Daten-Widget `widget_hass`, das im Designer
platziert wird und im Panel-Render (`GET /preview`) LIVE-Werte aus der
Home-Assistant-Instanz `http://10.33.0.5:8123` anzeigt — in einem von drei
konfigurierbaren Anzeige-Modi (Temperatur, Alarm-Zustand, Anwesenheit) für eine
oder mehrere ausgewählte Entities. Der HA-Zugang (Base-URL + Long-Lived-Token)
ist **einmalige Admin-Konfiguration**, serverseitig in `data/hass.json` (mode
0600, atomar geschrieben) abgelegt; der Token verlässt den Server **nie** — nicht
im Design-JSON, nicht in einer `Element.Property`, nicht in `/preview`, nicht in
einem Log. Der HA-Fetch läuft über einen **allowlist-basierten Safe-Fetch-Helper**
(nur der konfigurierte Host:Port, Redirects aus, Timeout + Größenlimit), der
generische „unavailable"-Fehler nach außen gibt und niemals interne Host-/IP-/
Fehlerdetails leakt.

Der Content wird über **genau eine** Quelle erzeugt: eine neue Methode
`fillHassContent(props)` in `server/internal/services/preview.go`, registriert im
B4a-Dispatcher `WidgetTextContent`. Panel und Canvas-Editor beziehen denselben
`{content}`-String über `POST /api/widget_content` — kein zweiter Renderpfad, kein
Client-seitiges Nachbauen der Formatierung. Alle user-sichtbaren Ausgabestrings
sind **Deutsch**, geführt aus der bestehenden Single-Source `locale.go`.

## Kontext

### Vorbedingung — B4a MUSS gemergt sein (harte Abhängigkeit)
B5 baut auf der in `specs/B4-unify.md` beschriebenen Unified-Pipeline auf, NICHT
auf dem alten Doppelpfad (`/api/widgets/*` + `services/widgets/*.go`). Konkret
setzt B5b voraus:
- `PreviewService.WidgetTextContent(elemType string, props map[string]any) (string, bool)`
  existiert als EINZIGER Content-Dispatch-Einstieg (B4-Ziel-Architektur,
  `specs/B4-unify.md:149-168`); `drawElement` (`preview.go:334-378`) ruft ihn.
- `POST /api/widget_content` (`handlers/widgets.go` `Content`, Route in
  `main.go`) liefert `{content}` verbatim aus `WidgetTextContent`.
- Canvas (`widgets.js`) bezieht Daten-Widget-Content über diesen einen
  Passthrough-Zweig statt über `build*Content`-Formatter.

**B5a** (Config-Storage + Safe-Fetch-Helper) ist von B4a UNABHÄNGIG und kann
sofort starten. **B5b/B5c** erfordern B4a gemergt. Falls B5 vor B4a fertig sein
muss, ist das eine Eskalation an Kilian — NICHT den alten Doppelpfad wiederbeleben.

### Sicherheit ist konsentiert, nicht neu zu verhandeln
Kilian hat das Widget freigegeben und einen read-only Long-Lived-Token geliefert
(`PROGRESS.md:44`). Die Security-Policy aus dem Review ist FIX und wird unten in
harte Akzeptanzkriterien gegossen. `security-reviewer` muss vor Merge APPROVE
geben (L5). Der echte Token liegt in `secrets.local.md` (gitignored,
`.gitignore:34`) und wird **nie** in dieser Spec, in Code, in Tests oder in Logs
inline geschrieben.

### Verifizierte Integrationspunkte (file:line)

**Token-Speicher-Muster (spiegeln):**
- `server/internal/auth/auth.go`: `Manager` mit `writeLocked` (`:153-182`) —
  `json.Marshal` → `os.CreateTemp(dir, name+".tmp-*")` → `tmp.Chmod(0600)` →
  `tmp.Write` → `tmp.Close` → `os.Rename`. Bootstrap-Semantik (`:136-149`):
  vorhandene Datei gewinnt IMMER, Env wird sonst ignoriert (mit Warnung).
- `.gitignore:47` deckt `data/hass.json` bereits ab (separat hinzugefügt).

**Config-Plumbing:**
- `server/internal/config/config.go`: `Config`-Struct + `Load()` (`:32-55`),
  `getEnv` (`:92`). Bootstrap-Env kommt hier rein.
- `server/main.go`: Service-Konstruktion `:96-135`, Route-Registrierung
  `:166-246`, `cfg := config.Load()` `:289`.

**Fetch-/Offline-Muster (Safe-Fetch orientiert sich daran, ersetzt es NICHT):**
- `server/internal/services/helpers.go`: `defaultHTTPClient = http.Client{Timeout:
  10*time.Second}` (`:16`), `readLimitedBody(r, limit)` via `io.LimitReader`
  (`:19`). **Wichtig:** `defaultHTTPClient` folgt Redirects und hat KEINE
  Host-Allowlist — der HA-Fetch nutzt ihn NICHT, sondern einen eigenen,
  gehärteten Client.
- `server/internal/services/negcache.go`: `failCache` (`:50`), `blocked`/
  `markFailure`/`markSuccess`/`markFailureValue`, `negativeCacheTTL = 2min`
  (`:22`), Test-Hooks `setNow`/`reset` (`:106-118`). Keys bisher `url:<url>`,
  `weather:<lat,lon>` — HA nutzt `hass:<host>|<entity_id>`.

**Content-Dispatch (Ziel B4a):**
- `preview.go` `drawElement` (`:334-378`), Content-Builder-Muster mit Fetch:
  `fillCustomContent` (`:819-828`), `fillCalendarContent` (`:606-691`,
  Negative-Cache-Nutzung als Vorlage). Per-Typ-`fontSize`-Default-Switch
  (`:246-259`) — dort kommt `widget_hass` dazu.
- `GetPropString` (`design.go:977`), `GetPropInt` (`:1005`), `GetPropBool`
  (`:1020`) — robuste `float64`/`string`/`bool`-Coercion für den JSON-Body.

**Deutsche Ausgabestrings = `server/internal/services/locale.go` (Single Source):**
- `locale.go` ist laut E3.7a die einzige Quelle deutscher Panel-Strings
  (`germanWeekdaysFull :12`, `germanMonths :19`, `germanWMODesc :51`,
  `germanWMOUnknown = "Unbekannt" :66`). Die HA-Zustands- und Fehlertexte kommen
  HIER dazu (neue Tabellen `germanHassAlarm`, Präsenz-/Fehlerkonstanten) —
  KEINE verstreuten Konstanten in `preview.go`/`hass.go`. `fillHassContent` liest
  ausschließlich aus `locale.go`. `fillHassContent` (preview.go) und `locale.go`
  liegen beide in `package services` → direkter Zugriff, kein Export nötig.

**Anzeige-Modus = dediziertes `hassMode`-Prop (NICHT `layout` überladen):**
- Der Anzeige-Modus (`temperature`/`alarm`/`presence`) ist ein EIGENES Widget-Prop
  `hassMode`, umgesetzt als `select`-Feld in `getWidgetPropertyDefs`
  (`properties-panel.js:769-820`); der `select`-Typ mit `options`-Array wird von
  `renderWidgetProperties` bereits gerendert (`:570-577`). `layout` bleibt für
  generische Layout-Semantik reserviert und wird von `fillHassContent` NICHT
  ausgewertet. Begründung: saubere Prop-Semantik — genau das B4-Ziel; keine
  Überladung von `layout`.
- KEIN `allLayouts["widget_hass"]`-Eintrag in
  `server/internal/services/widgets/layouts.go`. `renderWidgetProperties`
  (`:539`) rendert zwar immer einen generischen „Layout"-Dropdown (aus
  `GetLayouts`, `layouts.go:11`), der ohne Eintrag den Fallback-Default
  (`{id:"default"}`, `layouts.go:14`) zeigt — für `widget_hass` funktional neutral
  (der Fill ignoriert `layout`). Route `GET /api/widget_layouts/{type}`
  (`main.go:243`) bleibt unverändert.

**Frontend:**
- `element-factory.js`: `createWidget` `defaultSizes` (`:112-121`),
  `getDefaultProperties` (`:188-263`). Hier `widget_hass` (Default-Größe +
  Default-Props inkl. `hassMode` — **kein Token-Feld**).
- `properties-panel.js`: `getWidgetPropertyDefs` (`:769-820`),
  `renderWidgetProperties` (`:539`) rendert automatisch die per
  `getWidgetPropertyDefs` deklarierten Felder. HA-Props = `hassMode` (select) +
  `entityId` (text) + `label` (text) + `fontSize` (number) — **kein Token**.
- `widgets.js`: `fetchWidgetData` (`:19`) — post-B4b der eine Passthrough-Zweig
  nach `POST /api/widget_content`; `getPreviewContent` (`:67`); der Content wird
  in die Fabric-Textbox gesetzt (`label.set('text', displayText)`, `:363`).
  `widget_hass` MUSS in diesen Passthrough-Zweig, NICHT in einen eigenen
  Fetch/Fake.
- `designer.html`: Palette `.widget-item[data-type]` (`:92-120`) — neuer Eintrag
  `widget_hass`. Settings-Modal `#settings-modal` (`:235-275`) — Ort der
  HA-Verbindungs-Config (Base-URL + Token).
- `designer.js`: Settings-Laden/Speichern-Verdrahtung (`:350-475`, Muster
  `fetch('/settings')` + `POST /update_settings`). Die HA-Config nutzt EIGENE
  Endpoints (nicht `/update_settings`, s. u.), aber dasselbe UI-Wiring-Muster.

**Auth/Guard (unverändert übernehmen):**
- `middleware/auth.go`: `publicRoutes` (`:36-44`), `clientRoutes` (`:51-56`),
  CSRF-Origin-Check für mutierende Cookie-Requests (`:123-125`). Neue HA-Routen
  sind WEDER public NOCH client-routes → session-pflichtig sobald Passwort
  gesetzt; POST ist mutierend → CSRF wie `POST /api/preview_live`.

### Home-Assistant-REST-Vertrag (für die Formatierungs-ACs)
`GET <base>/api/states/<entity_id>` mit Header `Authorization: Bearer <token>`
liefert JSON:
```json
{"entity_id":"sensor.x","state":"21.5",
 "attributes":{"unit_of_measurement":"°C","friendly_name":"Wohnzimmer"},
 "last_changed":"...","last_updated":"..."}
```
- **Temperatur** (`sensor.*`): `state` = numerischer String, Einheit in
  `attributes.unit_of_measurement`.
- **Alarm** (`alarm_control_panel.*`): `state` ∈ {`disarmed`, `armed_home`,
  `armed_away`, `armed_night`, `armed_vacation`, `armed_custom_bypass`,
  `arming`, `disarming`, `pending`, `triggered`}.
- **Anwesenheit** (`person.*` / `device_tracker.*`): `state` = `home`,
  `not_home` oder ein Zonenname; `attributes.friendly_name` als Anzeigename.
- Fehlende Entity → HTTP 404. Nicht erreichbar → Transport-Fehler/Timeout.
  Sensor ohne Wert → `state` = `unavailable`/`unknown`.

## Ziel-Architektur

### 1. Config-Storage: `server/internal/hass/hass.go` (neues Paket, spiegelt `auth`)
Neues Paket `server/internal/hass` mit `Manager`:
- Datei `data/hass.json`, Format `{"base_url":"...","token":"..."}`.
- `writeLocked` **byte-strukturgleich** zu `auth.go:153-182` (tmp+rename,
  `Chmod(0600)`).
- `NewManager(dataDir string) (*Manager, error)` — lädt vorhandene Config;
  fehlende Datei = „nicht konfiguriert" (kein Fehler).
- `SetConfig(baseURL, token string) error` — validiert `baseURL` (Scheme ∈
  {http,https}, Host nicht leer, parsebar); persistiert atomar; hält Config unter
  `sync.RWMutex`.
- `Config() (baseURL, token string, ok bool)` — für den Fetch-Pfad;
  `ok=false` wenn Base-URL ODER Token leer.
- `Status() (baseURL string, configured bool)` — für die UI; gibt den Token
  **NIE** zurück (nur `configured` + Base-URL).
- `Bootstrap(envURL, envToken string) error` — Semantik wie `auth.Bootstrap`:
  vorhandene `data/hass.json` gewinnt IMMER; sonst werden nicht-leere Env-Werte
  persistiert. Nur-Token-ohne-URL wird gespeichert, bleibt aber `ok=false` bis
  eine URL gesetzt ist.

Config-Env in `config.go` ergänzen: `HassURL` (`EINK_HASS_URL`, optional,
Bootstrap-only), `HassToken` (`EINK_HASS_TOKEN`, optional, Bootstrap-only). In
`.env.example` OHNE Werte dokumentieren.

### 2. Safe-Fetch-Helper (Allowlist-Variante): `server/internal/services/safefetch.go`
Neuer, wiederverwendbarer Helper — bewusst so geschnitten, dass ein späterer
Folgetask die **Block-Variante** für die User-URL-Widgets (icalUrl/feedUrl/custom)
danebenlegen kann (SEPARATES Ticket, s. Non-Goals):
- `allowlistClient(host, port string) *http.Client` — `Timeout: 10s`,
  `CheckRedirect: func(*http.Request, []*http.Request) error { return
  http.ErrUseLastResponse }` (Redirects AUS — eine 3xx wird als Response
  zurückgegeben, nicht verfolgt), plus ein `Transport.DialContext`, der NUR
  `host:port` zulässt und jede andere Adresse mit einem Fehler ablehnt
  (Defense-in-Depth gegen DNS-Rebinding).
- `safeFetchAllowlisted(ctx, client, targetURL, bearer string, limit int64)
  ([]byte, int, error)`:
  - lehnt Scheme ≠ {http,https} ab (Fehler, kein Dial);
  - lehnt Ziel-Host:Port ≠ allowlisted ab (Fehler, kein Dial). Der Vergleich
    **normalisiert Default-Ports**: fehlt der Port, gilt 80 für http bzw. 443 für
    https, damit `10.33.0.5` und `10.33.0.5:80` (http) als gleich zählen;
  - setzt Token NUR als `Authorization: Bearer`-Header (nie in URL/Query);
  - liest Body via `io.LimitReader(resp.Body, limit)`, `limit = 2<<20` (2 MiB);
  - gibt `(body, statusCode, nil)` oder einen **generischen** Fehler zurück
    (kein Host/IP/interner Fehlertext im zurückgegebenen error-String, der je
    nach außen gereicht wird).
- **Rebinding-Härtung für Hostname-Konfig (Hinweis für die Helper-Wiederverwendung):**
  Wird je ein *Hostname* (statt einer IP) als HA-Host konfiguriert, MUSS die beim
  ersten Auflösen erhaltene IP gepinnt werden (einmal auflösen, dann diese IP im
  `DialContext` erzwingen), damit ein zwischenzeitlicher DNS-Wechsel den Pin nicht
  umgeht. Für das MVP (`10.33.0.5:8123`, feste IP) ist das *moot* — der
  `DialContext`-Pin auf die konfigurierte Adresse deckt den IP-Fall bereits ab;
  die Hostname-Auflösungs-Pinning-Variante ist als AC-Hinweis für die spätere
  Block-Helper-Wiederverwendung notiert (AC-SEC6).
- Transport ist für Tests injizierbar (Feld/Parameter), analog zu den Stubs in
  `offline_render_test.go` (`hostStubTransport`, `countingTransport`).

### 3. HA-Fetch-Service: `server/internal/services/hass.go`
`HassService`:
- hält `*hass.Manager` + den Allowlist-Client (lazy pro Host aufgebaut, bei
  Config-Änderung neu).
- `FetchEntity(ctx, entityID string) (*HassEntity, error)`:
  - liest `base, token, ok := manager.Config()`; `!ok` → `ErrHassNotConfigured`.
  - Negative-Cache-Key `hass:<host>|<entityID>`; bei `failCache.blocked` →
    generischer „unavailable"-Pfad ohne Netz.
  - baut `<base>/api/states/<url.PathEscape(entityID)>`; ruft
    `safeFetchAllowlisted`; 200 → JSON parsen (`state`, `attributes`); 404 →
    `ErrHassEntityUnknown`; sonst/Transport-Fehler → `failCache.markFailure` +
    generischer Fehler.
- `HassEntity` = `{EntityID, State string; Unit, FriendlyName string}`.
- Keine Schreib-/Steuer-Aufrufe (read-only, s. Non-Goals).

### 4. Content-Fill: `fillHassContent` in `preview.go`, registriert im Dispatcher
- `PreviewService` bekommt ein Feld `hass *HassService` (analog `weather`),
  gesetzt via `NewPreviewService`-Parameter. `fillHassContent` ist **nil-tolerant**
  (`s.hass == nil` → `germanHassNotConfigured`), damit bestehende Test-
  Konstruktionen von `PreviewService` nicht brechen.
- `fillHassContent(props map[string]any) string`:
  - `mode := GetPropString(props, "hassMode", "temperature")` — der Anzeige-Modus
    kommt aus dem **dedizierten `hassMode`-Prop**, NICHT aus `layout`.
  - `entityRaw := GetPropString(props, "entityId", "")`; leer →
    `germanHassNoEntity`. Mehrere Entities: kommagetrennt, per `strings.Split` +
    `TrimSpace`.
  - `label := GetPropString(props, "label", "")`.
  - Pro Entity `s.hass.FetchEntity`; Fehler-Mapping (s. ACs).
  - Formatierung je Modus (exakte deutsche Strings in AC-HA1..HA6), alle
    Zustandstexte aus `locale.go`.
- **`locale.go`-Erweiterung** (Single Source, `package services`):
  - `germanHassAlarm map[string]string` — Alarm-State → deutscher Text (Werte
    s. AC-HA2); unbekannter State fällt auf den rohen State zurück.
  - Präsenz-Konstanten `germanHassHome = "Zuhause"`,
    `germanHassAway = "Abwesend"`, `germanHassNobodyHome = "Niemand zuhause"`
    (Zonennamen werden unverändert durchgereicht).
  - Fehlerkonstanten `germanHassNotConfigured = "HA nicht konfiguriert"`,
    `germanHassUnavailable = "Nicht verfügbar"`, `germanHassNoEntity =
    "Keine Entity"`, sowie das Unbekannt-Präfix `germanHassUnknownPrefix =
    "Unbekannt: "` (wiederverwendet das bestehende Wort `germanWMOUnknown =
    "Unbekannt"` als Basis).
- `WidgetTextContent`-Switch (B4a) bekommt Case `widget_hass` →
  `fillHassContent`; `drawElement` (`:334-378`) den analogen Case. Per-Typ-
  `fontSize`-Default (`preview.go:246-259`): `widget_hass` → 18.

### 5. Admin-Config-Endpoints + Handler: `server/internal/handlers/hass.go`
- `HassHandler{svc *services.HassService, mgr *hass.Manager}`.
- `POST /api/hass/config` — Body `{"base_url":"...","token":"..."}` →
  `mgr.SetConfig`; 400 bei ungültiger URL; Antwort = `Status()`-Shape
  **ohne Token**.
- `GET /api/hass/config` — `{"configured":bool,"base_url":string,"token_set":bool}`
  — **niemals** das Token.
- (optional, empfohlen) `POST /api/hass/test` — serverseitiger
  Konnektivitätscheck gegen eine Test-Entity; Antwort strikt
  `{"ok":bool}` / generisch „unavailable"; keine internen Details. Wenn zu groß:
  DEFERRED, im PR vermerken.
- Routen in `main.go` neben den übrigen `/api/*`-Routen; NICHT in `publicRoutes`,
  NICHT in `clientRoutes` → session-pflichtig + CSRF für POST (wie
  `/api/preview_live`).

### 6. Frontend
- `element-factory.js`: `defaultSizes.widget_hass = {w:220,h:80}`;
  `getDefaultProperties.widget_hass = {hassMode:'temperature', entityId:'',
  label:'', fontSize:18, color:'#000000', textAlign:'left'}` — **kein Token,
  keine Base-URL**.
- `properties-panel.js`: `getWidgetPropertyDefs.widget_hass = {
  hassMode:{label:'Mode', type:'select', options:['temperature','alarm',
  'presence'], default:'temperature'}, entityId:{label:'Entity ID(s)',
  type:'text', default:''}, label:{label:'Label', type:'text', default:''},
  fontSize:{label:'Font Size', type:'number', default:18, min:8, max:200} }`. Der
  generische Layout-Dropdown bleibt (zeigt „Default"), ist für `widget_hass`
  neutral.
- `widgets.js`: `widget_hass` in den B4b-Passthrough-Zweig aufnehmen (POST
  `/api/widget_content`, `{content}` verbatim, gesetzt via `label.set('text',
  content)`); KEIN Fake, KEIN eigener Fetch, KEIN `innerHTML` (AC-SEC11).
- `designer.html`: Palette-Eintrag `widget_hass`; im Settings-Modal ein
  „Home Assistant"-Abschnitt (Base-URL-Input + Token-Input als
  `type="password"`, „Save"-Button, Status-Zeile „configured/not configured").
- `designer.js`: Wiring für den HA-Config-Abschnitt — `GET /api/hass/config` beim
  Öffnen (füllt Base-URL + Status, NIE das Token-Feld), `POST /api/hass/config`
  beim Speichern. Das Token-Input wird nach dem Speichern geleert und nie aus dem
  Server rückbefüllt. Status/Base-URL werden per `textContent` gesetzt (nie
  `innerHTML`).

## Akzeptanzkriterien (hart, überprüfbar — Security zuerst)

### Security (blockierend)
- **AC-SEC1 — Token nie im Design-JSON.** Ein Design mit einem `widget_hass`-
  Element wird gespeichert und via `GET`/Export geladen; das serialisierte
  JSON enthält KEIN `token`/`base_url`/Bearer-Feld. Test: Design mit
  `properties={hassMode,entityId,label}` durch Save→Load; grep/Assert im
  serialisierten `Properties`-Map auf `token`/`bearer` → 0 Treffer. Das
  Element-Schema (`getDefaultProperties`, `getWidgetPropertyDefs`) hat kein
  Token-Feld (grep in `server/static/js/` auf `token` im HA-Kontext → 0).
- **AC-SEC2 — Token nie in Logs.** Über den gesamten Fetch- und Config-Pfad wird
  der Token in KEINER `slog`-Zeile ausgegeben — auch nicht auf Debug-Level, auch
  nicht als Teil einer URL oder eines Fehlers. Test: Fetch/SetConfig mit einem
  Sentinel-Token laufen lassen, `slog`-Output in einen Buffer umleiten,
  assertieren, dass der Sentinel NICHT vorkommt. Statischer Beleg: grep über
  BEIDE HA-Dateien — `server/internal/hass/hass.go` (Config-Manager, persistiert
  den Token) UND `server/internal/services/hass.go` (Fetch/Fill) — sowie
  `server/internal/services/safefetch.go` und `server/internal/handlers/hass.go`:
  keine `slog.*`-Zeile führt das Token-Feld / den Bearer-Wert als Argument.
- **AC-SEC3 — Token nie in `/preview`-Output.** Der von `fillHassContent`
  erzeugte Content besteht ausschließlich aus Entity-Zustandstext (Wert, Einheit,
  human-readable State, Namen); der Token erscheint nie im gerenderten String.
  Test: `fillHassContent` gegen eine gemockte HA-Antwort; Assert, dass der Content
  den Sentinel-Token nicht enthält.
- **AC-SEC4 — Token nie als Property/URL/Query.** `safeFetchAllowlisted` setzt den
  Token ausschließlich als `Authorization: Bearer`-Header; die Request-URL
  (inkl. Query) enthält ihn nie. Test: Stub-Transport fängt den Request ab,
  assertiert `req.Header.Get("Authorization") == "Bearer <sentinel>"` UND
  `req.URL.String()` enthält den Sentinel NICHT.
- **AC-SEC5 — Redirect auf fremden Host wird NICHT verfolgt.** Stub-Transport
  antwortet mit 302 → `http://evil.example/`. Der Client verfolgt NICHT
  (CheckRedirect → `ErrUseLastResponse`); `FetchEntity` sieht Status 302 → non-200
  → generisches `germanHassUnavailable`. Assert: der Transport wird NIE mit einem
  Request an `evil.example` aufgerufen (Counting-Stub).
- **AC-SEC6 — Nicht-allowlisteter Host wird abgelehnt (Port-normalisiert).**
  `safeFetchAllowlisted` mit einem targetURL-Host:Port ≠ konfiguriertem Host:Port
  gibt einen Fehler zurück OHNE zu dialen (Assert: `DialContext` nie aufgerufen).
  Der Vergleich normalisiert Default-Ports (http→80, https→443), sodass
  `http://<host>` und `http://<host>:80` als gleich gelten (Test beide Richtungen).
  **Hinweis (nicht MVP-blockierend):** Bei künftiger Hostname-Konfig ist die
  aufgelöste IP zu pinnen (Rebinding-Schutz, s. Ziel-Architektur §2) — für die
  feste IP `10.33.0.5:8123` deckt der `DialContext`-Adress-Pin bereits alles ab.
- **AC-SEC7 — Nicht-http(s)-Scheme wird abgelehnt.** (a) `SetConfig` mit
  `base_url` `file://...`/`gopher://...`/leer → 400, nichts persistiert.
  (b) `safeFetchAllowlisted` mit non-http(s) targetURL → Fehler, kein Dial.
- **AC-SEC8 — Übergroße Antwort wird gedeckelt.** Stub liefert einen 5-MiB-Body;
  `safeFetchAllowlisted` liest höchstens 2 MiB (`io.LimitReader`); Assert
  `len(body) <= 2<<20`, kein OOM, Fetch terminiert.
- **AC-SEC9 — Config-Datei 0600 + atomar.** Nach `SetConfig` hat `data/hass.json`
  mode `0600` (Assert via `os.Stat`); der Schreibpfad nutzt tmp+rename (kein
  partielles Schreiben sichtbar). `GET /api/hass/config` gibt `token_set:true`,
  aber niemals den Token-String zurück (Assert im Response-Body).
- **AC-SEC10 — Guard/CSRF.** `POST /api/hass/config` ohne Session (bei gesetztem
  Passwort) → 401; mit Session + fremdem `Origin` → 403; mit Session +
  same-origin → 200. Muster `middleware/auth_test.go` (Session-Expiry / CSRF).
  Die HA-Routen sind NICHT in `clientRoutes` (der E-Ink-Client erreicht sie nie).
- **AC-SEC11 — Kein XSS aus HA-Freitext.** HA-abgeleiteter Freitext
  (`friendly_name`, Zonenname, roher State) kann von einer bösartigen/
  kompromittierten HA-Instanz stammen und ist als nicht-vertrauenswürdig zu
  behandeln. Der Canvas-Passthrough setzt den `{content}` ausschließlich über die
  Fabric-Textbox (`label.set('text', content)`, `widgets.js:363`) —
  Canvas-Glyph-Rendering, KEIN HTML-Parsing. Wird HA-Content jemals im DOM
  angezeigt (Status-/Debug-Zeile o. ä.), MUSS er via `textContent` gesetzt werden,
  NIE via `innerHTML`. grep-Gate: der über `/api/widget_content` gelieferte
  `content` fließt in keinem Pfad in eine `innerHTML`-Zuweisung; das
  Entity-Freitextfeld im Properties-Panel wird als input-`value` gesetzt
  (String-Kontext), nicht als Markup.

### Funktion / Content (gegen GEMOCKTE HA-Antworten; Strings aus `locale.go`)
- **AC-HA1 — Temperatur.** `hassMode=temperature`,
  `entityId="sensor.wohnzimmer"`, gemockte Antwort `state="21.5"`,
  `unit_of_measurement="°C"` → Content `"21.5°C"`. Mehrere Entities
  (`"sensor.a,sensor.b"`) → je Zeile `"<friendly_name>: <state><unit>"`.
  Nicht-numerischer `state` (`unavailable`/`unknown`) → graceful `"—"` für diese
  Entity.
- **AC-HA2 — Alarm.** `hassMode=alarm`, `entityId="alarm_control_panel.home"`.
  State→Text-Mapping exakt (aus `germanHassAlarm` in `locale.go`):
  `disarmed`→`"Unscharf"`, `armed_home`→`"Scharf (Anwesend)"`,
  `armed_away`→`"Scharf (Abwesend)"`, `armed_night`→`"Scharf (Nacht)"`,
  `armed_vacation`→`"Scharf (Urlaub)"`, `armed_custom_bypass`→
  `"Scharf (Benutzerdefiniert)"`, `arming`→`"Wird scharf…"`,
  `disarming`→`"Wird unscharf…"`, `pending`→`"Wird scharf…"`,
  `triggered`→`"Ausgelöst"`; unbekannter State → der rohe State-String. Optionales
  `label` wird vorangestellt (`"<label>: <text>"`).
- **AC-HA3 — Anwesenheit.** `hassMode=presence`.
  - Ein Entity (`person.kilian`): `state=home` → `"Zuhause"`, `state=not_home` →
    `"Abwesend"`, Zonenname → der Zonenname unverändert. `label` wird als Präfix
    genutzt.
  - Mehrere Entities: erste Zeile `"<N> zuhause"` (N = Anzahl mit `state==home`,
    N>0) bzw. `"Niemand zuhause"` (N==0); danach je anwesende Person deren
    `friendly_name`.
- **AC-HA4 — Graceful: Token/Base nicht konfiguriert.** `data/hass.json` fehlt /
  `Config().ok==false` → Content `"HA nicht konfiguriert"`
  (`germanHassNotConfigured`; kein Netzaufruf, kein Panic).
- **AC-HA5 — Graceful: HA unerreichbar.** Transport-Fehler/Timeout/non-200 (außer
  404) → Content `"Nicht verfügbar"` (`germanHassUnavailable`); interne Host/IP/
  Fehlertexte tauchen NICHT im Content auf. Negative-Cache: ein zweiter Render
  innerhalb `negativeCacheTTL` erzeugt denselben String OHNE erneuten Netzaufruf
  (Attempt-Count-Assert wie in `offline_render_test.go`).
- **AC-HA6 — Graceful: Entity unbekannt.** HTTP 404 auf `/api/states/<id>` →
  Content `"Unbekannt: <entity_id>"` (`germanHassUnknownPrefix + entityID`) — kein
  „Nicht verfügbar", damit Tippfehler in der Entity-ID erkennbar bleiben.
- **AC-HA7 — Deutsche Strings aus `locale.go` (Single Source).** Alle
  user-sichtbaren HA-Zustands-/Fehlertexte stammen aus den `locale.go`-Tabellen/
  -Konstanten (`germanHassAlarm`, `germanHassHome/Away/NobodyHome`,
  `germanHassNotConfigured/Unavailable/NoEntity/UnknownPrefix`); `fillHassContent`
  definiert KEINE eigenen String-Literale für diese Texte. Beleg: die B5b-Tests
  assertieren gegen die `locale.go`-Konstanten (kein hartkodiertes Duplikat), und
  grep zeigt die Zustands-/Fehlertexte nur in `locale.go`, nicht in `preview.go`/
  `hass.go`.

### Pipeline-Integrität (B4a-konform)
- **AC-PIPE1 — Eine Content-Quelle.** `widget_hass`-Content wird ausschließlich
  über `PreviewService.WidgetTextContent` erzeugt (Panel via `drawElement`,
  Canvas via `POST /api/widget_content`). grep in `server/static/js/`: keine
  HA-spezifische Content-Formatierung (kein `buildHassContent`, kein
  HA-`switch`-Fake in `getPreviewContent`).
- **AC-PIPE2 — Canvas↔Panel-Gleichheit.** Ein Go-Handler-Test POSTet
  `{type:"widget_hass", properties}` an `/api/widget_content` und assertiert, dass
  der zurückgegebene `content` EXAKT `previewSvc.WidgetTextContent("widget_hass",
  properties)` (Direktaufruf, gleiche Service-Instanz, gleicher gemockter
  HA-Transport) entspricht — für die Modi temperature/alarm/presence und die
  Graceful-Fälle (AC-HA4..HA6).
- **AC-PIPE3 — Golden byte-identisch.** `TestGoldenRender` bleibt OHNE `-update`
  grün (die Golden-Designs enthalten kein `widget_hass`; die Dispatcher-Erweiterung
  bewegt keinen Panel-Pixel bestehender Designs).

### Statik/Build
- **AC-BUILD1 — Grün.** `gofmt -l server/` leer, `go vet ./...` clean,
  `go build ./...` clean, `go test ./... -count=1` grün inkl. `-race` auf
  `./internal/services` und `./internal/handlers`.
- **AC-BUILD2 — .env.example.** `EINK_HASS_URL` und `EINK_HASS_TOKEN` mit
  Ein-Zeilen-Kommentar dokumentiert, OHNE echte Werte.

## Non-Goals (ausdrücklich NICHT in B5)
- **Kein Schreiben/Steuern.** Read-only. Keine Service-Calls
  (`/api/services/...`), kein Arm/Disarm, kein Schalten. Das Widget zeigt nur an.
- **Kein Fix der bestehenden User-URL-SSRF-Lücke.** Die vorhandenen Widgets
  calendar/news/custom (`icalUrl`/`feedUrl`/`url`) fetchen über
  `defaultHTTPClient` OHNE Redirect-/Host-Härtung. Das ist eine **vorbestehende,
  SEPARATE** Schwachstelle → eigenes Ticket (Block-Variante des Safe-Fetch-
  Helpers). B5 legt den Helper so an, dass dieses Folge-Ticket die Block-Variante
  danebenlegen kann, härtet die Alt-Widgets aber NICHT.
- **Nur die drei MVP-Entity-Typen** (`sensor.*` Temperatur,
  `alarm_control_panel.*`, `person.*`/`device_tracker.*`). Keine generischen
  Entity-Karten, keine Templates, keine Icons/Grafiken, keine History-Graphen.
- **Kein WebSocket / kein Push.** Live = Wert beim Render (Panel-Refresh-Takt);
  kein HA-Event-Stream.
- **Kein Anfassen** von WeatherService, Quantisierung/Kalibrierung, Auth-/CSRF-
  Regeln, Layout-Endpoint, clock/timer-Client-Live-Insel.
- **Kein Reaktivieren des alten `/api/widgets/*`-Doppelpfads** (B4c löscht ihn).

## Dekomposition (Diff > ~400 Zeilen → drei geordnete Sub-Tasks)

Sichere Reihenfolge: **B5a → B5b → B5c**. B5a ist B4a-unabhängig und sofort
startbar; B5b/B5c erfordern B4a gemergt. Diese Reconciliation (dediziertes
`hassMode`, deutsche Strings via `locale.go`, XSS-/Port-Härtung) betrifft primär
B5b/B5c — B5a (Config + Safe-Fetch) ist davon unberührt und startet unabhängig.

### B5a — Config-Storage + Safe-Fetch-Helper + Admin-Endpoints (B4a-unabhängig)
**Scope:**
- Neues Paket `server/internal/hass` (`Manager` + `writeLocked` spiegelt `auth.go`
  + `Bootstrap`).
- `config.go`: `EINK_HASS_URL`/`EINK_HASS_TOKEN` (Bootstrap-only); `.env.example`.
- `services/safefetch.go`: Allowlist-Client + `safeFetchAllowlisted`
  (Redirect-aus, Host:Port-Pinning mit Default-Port-Normalisierung, Scheme-Check,
  `io.LimitReader` 2 MiB, Bearer-Header, injizierbarer Transport).
- `services/hass.go`: `HassService.FetchEntity` (+ Negative-Cache-Key `hass:...`).
- `handlers/hass.go` + Routen `GET|POST /api/hass/config` (+ optional
  `POST /api/hass/test`); Wiring in `main.go` (Manager/Service/Handler
  konstruieren, `hass.Manager.Bootstrap` neben `authMgr.Bootstrap`).
**Akzeptanz:** AC-SEC1(Schema-Teil: kein Token-Feld serverseitig), AC-SEC2,
AC-SEC4, AC-SEC5, AC-SEC6, AC-SEC7, AC-SEC8, AC-SEC9, AC-SEC10, AC-BUILD1,
AC-BUILD2. `FetchEntity` gegen gemockten Transport (200/404/Transport-Fehler).
**Warum zuerst:** rein additiv, keine Render-/Canvas-Berührung, keine
B4a-Abhängigkeit; liefert die sichere Datensenke, bevor irgendein Konsument sie
nutzt.

### B5b — `fillHassContent` + Dispatcher-Registrierung + gemockte Content-Tests
**(erfordert B4a gemergt)**
**Scope:**
- `PreviewService`-Feld `hass *HassService` (Konstruktor-Param, nil-tolerant);
  Call-Sites (`main.go`, Tests) nachziehen.
- `locale.go` um die HA-Tabellen/-Konstanten erweitern (`germanHassAlarm`,
  Präsenz-/Fehlerkonstanten) — Single Source, KEINE Literale in `preview.go`.
- `fillHassContent` in `preview.go` (liest `hassMode`, Strings aus `locale.go`);
  Case in `WidgetTextContent` + `drawElement`; `fontSize`-Default
  (`preview.go:246-259`) `widget_hass`→18.
- KEIN `layouts.go`-Eintrag für `widget_hass` (Modus läuft über `hassMode`, nicht
  über den Layout-Dropdown).
- Tests: AC-HA1..HA7 gegen gemockte HA-Antworten; AC-PIPE2 (Handler-Gleichheit
  über `/api/widget_content`); AC-SEC3 (Token nie im Content).
**Akzeptanz:** AC-HA1..HA7, AC-PIPE1(Server-Teil), AC-PIPE2, AC-PIPE3, AC-SEC3,
AC-BUILD1.
**Warum als zweites:** Server-seitige Verhaltensänderung; nutzt B5a-Senke + B4a-
Dispatcher; noch keine UI. Revert = `fillHassContent`-Case entfernen.

### B5c — Frontend: Palette + Props + Config-UI + Passthrough
**(erfordert B4b gemergt, damit der Passthrough-Zweig existiert)**
**Scope:**
- `element-factory.js` (Default-Größe + Default-Props inkl. `hassMode`, kein
  Token), `properties-panel.js` (`getWidgetPropertyDefs.widget_hass` mit
  `hassMode` als `select`), `widgets.js` (`widget_hass` in den Passthrough-Zweig,
  Content via `label.set('text', …)`, nie `innerHTML`), `designer.html`
  (Palette-Eintrag + HA-Config-Abschnitt im Settings-Modal), `designer.js`
  (Config-UI-Wiring gegen `/api/hass/config`, Token-Feld write-only, Status via
  `textContent`).
**Akzeptanz:** AC-PIPE1(Canvas-Teil: kein Fake/keine HA-Formatierung im JS),
AC-SEC1(Frontend-Teil: kein Token-Feld in den Props), AC-SEC11 (Content über
Fabric `.set('text')`/`textContent`, nie `innerHTML`), plus L2-Browser-Beleg
(Editor-Label == Panel-Render für die drei Modi; Config speichern → Widget zeigt
Live-Wert; Token nie im DOM/localStorage/Design-JSON).
**Warum zuletzt:** UI hängt an B5b-Content + B4b-Passthrough. Revert =
Frontend-Diff zurücknehmen, Server bleibt funktional.

## Verifikation

### Gate L1 — Statik + Unit (lokal, Mac; erforderlich vor Commit)
- `cd server && gofmt -l .` (leer), `go vet ./...`, `go build ./...`.
- `cd server && go test ./... -count=1` inkl. `-race` auf `./internal/services`
  und `./internal/handlers` — deckt AC-SEC*, AC-HA1..HA7, AC-PIPE1..PIPE3,
  AC-BUILD*.
- grep-Gates: kein Token-Feld im Element-Schema (`static/js/`); keine
  `slog`-Zeile mit Token/Bearer in `internal/hass/hass.go`,
  `internal/services/hass.go`, `internal/services/safefetch.go`,
  `internal/handlers/hass.go`; keine HA-Content-Formatierung im Canvas-JS; kein
  `innerHTML` mit `/api/widget_content`-Content; HA-Zustands-/Fehlertexte nur in
  `locale.go`.

### Gate L2 — Content-Gleichheit + Browser (auf dem Mac)
- Handler-Test AC-PIPE2 grün (Canvas↔Panel für alle Modi + Graceful).
- Browser: `cd server && go run .`, HA-Config im Settings-Modal setzen (gegen
  einen lokalen HA-Stub ODER die reale Instanz, falls erreichbar); je Modus ein
  `widget_hass` platzieren; Editor-Label gegen `GET /preview`-Panel-Render
  vergleichen. Beleg AC-SEC1-Frontend: Design exportieren, im JSON grep auf Token
  → 0. Beleg AC-SEC11: DevTools — der HA-Content erscheint als Fabric-Canvas-Text,
  nicht als DOM-Markup; ein Entity mit `friendly_name` wie
  `<img src=x onerror=alert(1)>` rendert als sichtbarer Text, führt KEIN Skript
  aus.

### Gate L5 — Security-Review (Pflicht-APPROVE vor Merge)
`security-reviewer` bestätigt: (a) Token nur in `data/hass.json` (0600, atomar),
nie in Prop/URL/Log/Preview/Design-Export; (b) SSRF-Allowlist wirkt (Redirect aus,
Host:Port-Pinning mit Port-Normalisierung, Scheme-Check, Größenlimit, generische
Fehler); (c) Read-only, keine Steuer-Calls; (d) Guard/CSRF für die Admin-Endpoints,
HA-Routen nicht als client-routes; (e) kein XSS aus HA-Freitext (Fabric
`.set('text')`/`textContent`, nie `innerHTML`); (f) die vorbestehende
User-URL-SSRF-Lücke ist als SEPARATES Ticket vermerkt und NICHT mit-angefasst;
(g) Safe-Fetch-Helper ist so geschnitten, dass die Block-Variante später
danebenpasst (inkl. Hostname-IP-Pinning-Hinweis).

### Gate L3 — Live gegen reale HA (DEFERRED → hardware-validator)
Gegen `http://10.33.0.5:8123` mit dem Token aus `secrets.local.md` (NIE inline):
je Modus eine reale Entity abfragen, Panel-Render prüfen, Fehlerfälle
(HA aus / Entity gelöscht) live durchspielen. MARKIERT DEFERRED — erfordert
LAN-Zugang zur HA-Instanz; Messwerte/Screenshots im PR nachreichen.

## Risiken
- **B4a nicht gemergt.** B5b/B5c hängen hart an `WidgetTextContent` +
  `/api/widget_content`. **Mitigation:** B5a vorziehen (unabhängig); B5b/B5c erst
  nach B4a-Merge starten; sonst Eskalation an Kilian statt Doppelpfad-Revival.
- **Token-Leak über einen übersehenen Pfad** (Log/Export/Fehlertext).
  **Mitigation:** AC-SEC1..SEC4 mit Sentinel-Token-Assertions + grep-Gates (beide
  `hass.go`) + L5. Rollback: `data/hass.json` löschen invalidiert den Token sofort
  (kein Schema-Migrationsbedarf).
- **SSRF trotz Allowlist** (Redirect/Rebinding/Port-Trick). **Mitigation:**
  Redirects aus (AC-SEC5) UND Host:Port-Pinning im Dial mit Port-Normalisierung
  (AC-SEC6) — zwei unabhängige Kontrollen; Scheme-Check (AC-SEC7); Größenlimit
  (AC-SEC8); Hostname-IP-Pinning als Härtungs-Hinweis für die Wiederverwendung.
  Rollback: HA-Config leeren → `FetchEntity` liefert nur noch
  „HA nicht konfiguriert".
- **XSS aus kompromittierter HA-Instanz.** **Mitigation:** HA-Freitext ist
  nicht-vertrauenswürdig und fließt nur in Fabric-Canvas-Text (`.set('text')`) /
  `textContent`, nie in `innerHTML` (AC-SEC11, L2-Beleg mit Payload). Rollback:
  Frontend-Diff (B5c) zurücknehmen.
- **Live-Fetch-Last auf Pi Zero / HA-Timeout blockiert Render.** **Mitigation:**
  `HassService`-Client hat 10s-Timeout; Negative-Cache (2 min) bündelt
  Fehlversuche (AC-HA5); der Fetch läuft — wie custom/calendar — innerhalb des
  Render-Semaphors, konkurriert also nicht zusätzlich. `_dataCache` (60s) im
  Canvas dämpft Prop-Tipp-Stürme (via B4a-Debounce). Rollback: Widget aus dem
  Design entfernen.
- **`NewPreviewService`-Signaturänderung bricht Tests.** **Mitigation:**
  `fillHassContent` nil-tolerant; alle Konstruktions-Call-Sites in einem Commit
  nachziehen; `go build ./...` als Gate.
- **Entity-Typ-/Alarm-Backend-Annahme falsch** (alarmo statt
  `alarm_control_panel`, andere Presence-States). **Mitigation:** unbekannte
  States fallen auf den rohen State-String zurück (AC-HA2/HA3) — nie Panic, nie
  leer; die exakten Entity-IDs klärt Kilian (s. u.), bevor L3 gemessen wird.

## Offene Fragen an Kilian
1. **Exakte Entity-IDs** je Modus (mind. je ein `sensor.*` Temperatur, ein
   `alarm_control_panel.*` bzw. Alarm-Entity, ein `person.*`/`device_tracker.*`)
   — für die L3-Live-Prüfung. Für L1/L2 genügen gemockte IDs.
2. **Alarm-Integration:** natives `alarm_control_panel.*` ODER `alarmo`
   (letzteres exponiert i. d. R. ebenfalls `alarm_control_panel.*`, aber mit
   eigenen `attributes`)? Bestimmt nur das State→Text-Mapping (AC-HA2), das sonst
   auf den rohen State zurückfällt.

**Bereits entschieden (Reviewer-Reconciliation):** Anzeige-Modus =
dediziertes `hassMode`-Prop (nicht `layout`); Ausgabesprache = Deutsch, geführt
über `locale.go` (Single Source). Diese Punkte sind in den ACs fixiert und keine
offenen Fragen mehr.
