# F10: Display-Send-Mode — gedithertes vs. ungedithertes Panel-Bild wählbar

**Status:** Draft
**Abhängigkeiten:** E1.4 (Client-Resize-Guard, gemergt), E1.6 (Panel-Calibration, gemergt), E5.2 (Panel-Care/Content-Skip, gemergt)
**Blockiert:** nichts hart; Voraussetzung für die spätere, smartere per-Region-Antwort auf das Green-Text-Fringing (`23553e3`)
**Zuständig:** server (Go + Frontend) + pi-client (nur `fetch_preview`/`fetch_display_config`-Verdrahtung)
**Querverweis:** Dieses Setting ist der user-seitige Hebel für den Green-Text-Fringing-Trade-off, der nach der Linear-Light-Dithering-Änderung (`23553e3`) auf dem 6-Farb-Panel beobachtet wurde. Eine spätere per-Region-Lösung ersetzt diesen globalen Schalter ggf. — er ist die einfache erste Antwort.

## Ziel

Der Nutzer kann global wählen, ob das an das E-Ink-Panel gesendete Bild das **server-geditherte** (heutiges Verhalten, Default) oder das **ungedithette Original** ist — Letzteres überlässt die Palettenabbildung dem Waveshare-Treiber selbst. Nach dem Deploy ändert sich ohne explizite Nutzeraktion nichts (Default = `dithered`).

## Kontext

### Betroffene Dateien

| Datei | Rolle | Änderung |
|---|---|---|
| `server/internal/models/settings.go` | `Settings`- und `SettingsResponse`-Struct, Enum-Typen/Consts | neues Feld + neuer String-Enum-Typ |
| `server/internal/services/settings.go` | Defaults in `GetSettings` (Z. 146–164, 172–180), `GetSettingsResponse` (Z. 261–276), neuer Getter | Default-Normalisierung + Response-Feld + `GetPanelImageMode()` |
| `server/internal/handlers/settings.go` | `UpdateSettings` (Z. 43–147), Request-Struct + Validierung | Feld annehmen, unbekannte Werte ablehnen (Muster wie `dither_algorithm`, Z. 88–96) |
| `server/internal/handlers/preview.go` | `PreviewHandler.Preview` (Z. 30–61) | `raw` NICHT mehr allein aus Query, sondern Setting-gesteuert (siehe unten) |
| `server/internal/services/preview.go` | `Render(ctx, design, raw)` (Z. 191), `RenderActiveRaw` (Z. 1227) | KEINE Signaturänderung nötig — `raw` existiert bereits Ende-zu-Ende |
| `server/templates/designer.html` | Settings-Modal, direkt nach `#render-quality` (Z. 273–278) | neues `<select>`-Control + Erklärtext |
| `server/static/js/designer.js` | Settings-Verdrahtung (Muster `render-quality`, Z. 459–482) | Laden aus `/settings`, Speichern via `/update_settings` |
| `client/client.py` | `fetch_display_config` (Z. 193–206), `fetch_preview` (Z. 209–230) | Mode aus /settings lesen, `raw=true` nur bei `original` an `/preview` anhängen |

### Verifizierte Fakten (gegen Code geprüft)

1. **`raw` existiert bereits Ende-zu-Ende.** `PreviewService.Render(ctx, design, raw bool)` (preview.go:191): bei `raw==false` läuft `quantizeForDisplay(...)` (Z. 339) gegen die Panel-Palette; bei `raw==true` (Z. 336–337) wird `output = finalImg` OHNE Quantisierung gesetzt. Supersample+Downscale (Z. 322–330) laufen in **beiden** Fällen. `raw==true` liefert also ein voll-farbiges, auf Canvas-Größe (800×480) heruntergerechnetes, NICHT paletten-quantisiertes Bild. `RenderActiveRaw(ctx, raw)` (preview.go:1227) reicht `raw` an `Render` durch.
2. **Handler liest `raw` heute aus der Query.** `PreviewHandler.Preview` (preview.go:36): `raw := r.URL.Query().Get("raw") == "true"`, weitergereicht an `Render`/`RenderActiveRaw`. `PreviewLive` (Z. 71) ebenso. Dieses Feature verdrahtet ein persistiertes Setting an genau diesen bestehenden Pfad — es erfindet keinen neuen Render-Pfad.
3. **Raw-Output ist RGB in Panel-Größe.** `finalImg` ist `*image.RGBA` (800×480), `png.Encode` schreibt Truecolor-PNG; Pillow öffnet es als Mode `"RGB"`. Größe == Panelgröße ⇒ der E1.4-Resize-Guard (`display_image`, client.py:270–282) wird im Normalbetrieb NICHT ausgelöst. Der in E1.4 Punkt 4 dokumentierte RGB-Resize-Hazard (339 Farben nach LANCZOS) liegt damit NICHT auf dem heißen Pfad dieses Features — er ist nur bei zusätzlicher Fehlkonfiguration (falsches Profil) relevant.
4. **Client-Display-Pfad ist bereits Mode-agnostisch.** `display_image` (client.py:284–295): 6-Farb-Panel (`len(colors) > 2`) → `img.convert("RGB")` falls nötig, `epd.getbuffer` macht die Treiber-Palettierung; B/W-Panel → `img.convert("L").point(threshold, "1")`. Ein RGB-Input passt für beide Pfade ohne Codeänderung an `display_image`.
5. **Settings-Konventionen.** `Settings` (settings.go:43–54) nutzt getypte String-Enums (`RenderQuality`, `DitherAlgorithm`, `CalibrationMode`) mit `json:"...,omitempty"`. Defaults werden in `GetSettings` gesetzt (leerer Wert → Default), `UpdateSettings` validiert per `switch` und lehnt Unbekanntes mit `400` ab (Z. 88–96). `SettingsResponse` (settings.go:56–66) spiegelt die Felder OHNE `omitempty`.
6. **Content-Skip (E5.2).** `fetch_preview` (client.py:209–230) hasht die rohen Wire-Bytes (`_last_fetch_hash`, SHA-256 vor Pillow-Decode). Ein Mode-Wechsel ändert die Bytes ⇒ genau EIN zusätzlicher Refresh beim Umschalten. Das ist **korrekt und kein Bug** — siehe Non-Goals.

### KONTRADIKTION zur Aufgabenannahme (bindend für die Umsetzung)

`fetch_display_config` (client.py:193–206) dekodiert zwar die **gesamte** `/settings`-Antwort (`settings = resp.json()`), gibt aber nur das verschachtelte Objekt `settings["display"]` (= `DisplayConfig`, u. a. `colors`, `driver`) zurück. Die Settings-Felder auf oberster Ebene (`render_quality`, `dither_algorithm` …) sind darin NICHT enthalten.

Das neue Feld liegt — konsistent mit `render_quality`/`dither_algorithm` — auf oberster Ebene der `/settings`-Antwort (Geschwister von `display`), NICHT innerhalb von `DisplayConfig`. Folge für die Umsetzung: `fetch_display_config` muss das Top-Level-Feld aus dem bereits dekodierten `settings`-Objekt lesen und in das zurückgegebene Dict aufnehmen (z. B. `display["panel_image_mode"] = settings.get("panel_image_mode", "dithered")`). KEIN neuer Endpoint, KEIN zweiter Request — die Antwort liegt bereits vor. „Existing display-config path" bleibt gewahrt.

### Entscheidung: Name und Typ des Settings

**Gewählt:** getypter String-Enum `PanelImageMode` mit JSON-Tag `panel_image_mode` und den Werten `dithered` (Default) / `original`.

```go
type PanelImageMode string
const (
    PanelImageDithered PanelImageMode = "dithered" // server-dithered, current behaviour (default)
    PanelImageOriginal PanelImageMode = "original" // ungedithered original, driver hard-maps palette
)
```

**Begründung gegen einen bool** (`send_raw` o. ä.):
- Konsistenz: `render_quality`, `dither_algorithm`, `calibration` sind alle getypte String-Enums mit Const + `switch`-Validierung — ein bool bräche das Muster.
- Selbst-dokumentierend in `settings.json` (`"panel_image_mode": "dithered"` statt `"send_raw": false`, dessen Polung man raten muss).
- Erweiterbar: eine spätere per-Region-Lösung (Querverweis) kann einen dritten Wert (`per_region`) ergänzen, ohne Feld-Semantik zu brechen.
- Der Wortstamm meidet das Wort „raw" im User-facing Setting; „raw" bleibt der interne Query-Parameter/Debug-Begriff.

## Akzeptanzkriterien

Jedes AC ist maschinell prüfbar; pro AC ist der beweisende Test/Check benannt.

1. **AC1 — Persistiertes Global-Setting, Default = dithered.** `Settings` trägt `PanelImageMode PanelImageMode` mit `json:"panel_image_mode,omitempty"`. `GetSettings` normalisiert leeren/fehlenden Wert auf `PanelImageDithered` (analog `DitherAlgorithm`, settings.go:175–180) — auch im Not-Exist- und im Unmarshal-Fehler-Zweig (Z. 146–164). Eine `settings.json` OHNE das Feld verhält sich byte-identisch zum heutigen Deploy (gedithert).
   *Beweis:* Go-Unit-Test `TestPanelImageModeDefaults` (fehlende Datei, leeres Feld, korrupte Datei → alle `dithered`).
2. **AC2 — Round-Trip über die API.** `GET /settings` liefert `panel_image_mode` (in `SettingsResponse`, OHNE `omitempty`). `POST /update_settings` mit `{"panel_image_mode":"original"}` persistiert den Wert; ein direkt folgender `GET /settings` liefert `original`.
   *Beweis:* Handler-Test `TestUpdateSettingsPanelImageModeRoundtrip` (Muster: bestehende `dither_algorithm`-Tests in `settings_test.go`).
3. **AC3 — Ungültige Werte werden abgelehnt.** `POST /update_settings` mit `{"panel_image_mode":"raw"}` (oder jedem Wert ≠ `dithered`/`original`) antwortet `400` mit Meldung `invalid panel_image_mode: <wert>` und lässt den persistierten Wert unverändert. Der leere String (`""`) bedeutet „unverändert lassen" (Muster `dither_algorithm`, settings.go-Handler Z. 88).
   *Beweis:* Handler-Test `TestUpdateSettingsPanelImageModeRejectsUnknown`.
4. **AC4 — Server ehrt den Mode am Render-Pfad.** `PreviewHandler.Preview` bestimmt `raw` aus dem persistierten Setting UND dem Query-Param (siehe Non-Goals zur genauen Regel): bei `panel_image_mode=original` rendert `/preview` ungedithert, bei `dithered` gedithert. Ein Render-/Golden-Test pinnt, dass der ungedithette Output **mehr** unique RGB-Farben trägt als die Palette groß ist (6-Farb-Panel: > 6), der gedithette **≤** Palettengröße.
   *Beweis:* `TestRawOutputIsUnquantized` in `server/internal/services` — buntes Test-Design (Foto-artiger Verlauf), `Render(ctx, d, true)` → unique-RGB-Zählung > 6; `Render(ctx, d, false)` → ≤ 6. (Nutzt vorhandene Golden-/Quantize-Test-Infra, `golden_test.go`/`quantize_test.go`.)
5. **AC5 — Client hängt `raw=true` nur bei `original` an.** `fetch_display_config` nimmt `panel_image_mode` in sein Rückgabe-Dict auf (siehe KONTRADIKTION oben). `fetch_preview` ruft `/preview?raw=true` genau dann, wenn der Mode `original` ist, sonst `/preview` unverändert. Kein neuer Endpoint. Der Content-Hash (E5.2) wird wie heute aus den Wire-Bytes gebildet.
   *Beweis:* Python-Test in `client/test_client.py`: gemockter `_server_get` prüft die aufgerufene URL für beide Modes (`raw=true` vorhanden / abwesend).
6. **AC6 — UI-Control mit Trade-off-Erklärung.** Im Settings-Modal (`designer.html`, direkt nach `#render-quality`) existiert `<select id="panel-image-mode">` mit Optionen `dithered` (Default `selected`) / `original`. `designer.js` lädt den Wert aus `/settings` und speichert Änderungen via `/update_settings` (Muster Z. 459–482), inkl. `showNotification`. Ein kurzer Erklärtext nennt den Trade-off wörtlich: *dithered = Fotos glatter, Text kann farbig fransen; original = Text scharf, Fotos können banden.*
   *Beweis:* L5-Review + manueller Klicktest (Wert wechselt, persistiert, überlebt Reload).
7. **AC7 — Bestand bleibt grün.** `go test ./...`, `go vet ./...` und `python3 -m unittest test_client` laufen unverändert durch; kein bestehender Golden-/Settings-Test bricht.
   *Beweis:* Testläufe unter „Verifikation".

## Non-Goals

- **KEIN Widget.** Dieses Setting berührt NICHT die Widget-Recipe/Registrierungspunkte (`widgets.js`, `element-factory.js`, `widgetDefaultFontSizes`, `drawElement`). Es ist ein globales Display-Setting.
- **KEIN Per-Region-/Per-Widget-Modus.** Der ganze Panel-Frame wird gedithert ODER original gesendet. Die smartere per-Region-Antwort (Querverweis) ist ein Folge-Task.
- **KEINE Änderung an `display_image`** (client.py:254–312) — der Display-Pfad ist bereits Mode-agnostisch (Fakt 4). Nur `fetch_preview`/`fetch_display_config` ändern sich.
- **KEINE Änderung der `Render`-Signatur** — `raw bool` existiert bereits.
- **Query-Param-Semantik von `/preview`:** Der bestehende `?raw=true`-Query-Pfad (Browser-Debug) bleibt funktionsfähig. Bindende Regel für den Server: der **Client** entscheidet über den Query-Param anhand des Settings (AC5); der Server ehrt `raw=true`, wenn es kommt. Ob der Server zusätzlich das Setting als Fallback anwendet, wenn der Query-Param fehlt, ist eine Implementierungsentscheidung — sie darf den Browser-Debug (`?raw=true` erzwingt raw, `?raw=false`/fehlend = gedithert) NICHT brechen. Empfehlung: Query-Param hat Vorrang; fehlt er, greift das Setting. Die Live-Vorschau des Designers (`PreviewLive`) bleibt gedithert (WYSIWYG gegen das Panel-Default), unabhängig vom Setting.
- **KEINE Aufnahme in `userSettingsMarkers`** (settings.go:204–210). Das Setting löst die Setup-Wizard-Erkennung (E2.3) NICHT aus — sonst würde ein Mode-Wechsel den Wizard als „konfiguriert" markieren. Bewusst ausgelassen.
- **KEINE `.env`/Config-Variable** — reines persistiertes Setting.

## Verifikation

### Gate L1 — Server-Unit + Static (lokal)

```sh
cd "/Users/ksch/Documents/005 - Hobby/Github/E-INK-Picture/server"
go vet ./...
go test ./... -run 'PanelImageMode|RawOutput|Settings' -v
go test ./...
```

Erwartung: AC1–AC4, AC7 grün; keine Regression.

### Gate L2 — Client-Unit (lokal)

```sh
cd "/Users/ksch/Documents/005 - Hobby/Github/E-INK-Picture/client"
python3 -m py_compile client.py
python3 -m unittest test_client -v
```

Erwartung: neuer URL-Test (AC5) grün, Bestand grün.

### Gate L5 — Review

- Feld-Defaults in ALLEN drei `GetSettings`-Zweigen gesetzt (Not-Exist, Unmarshal-Fehler, Normalisierung).
- `UpdateSettings`-Validierung lehnt Unbekanntes ab; leerer String = unverändert.
- `fetch_display_config` liest das Top-Level-Feld (KONTRADIKTION oben), nicht aus `display`.
- Query-Param-Vorrang bricht Browser-Debug nicht; `PreviewLive` bleibt gedithert.
- UI-Erklärtext ist vorhanden und benennt beide Richtungen des Trade-offs.

### Gate L3 — Hardware (PFLICHT vor Merge des Client-Teils, BEIDE Modes × BEIDE Panels, Mensch schaut aufs Bild)

Dies ist das Substanz-Gate. Der `original`-Mode wurde bisher AUSSCHLIESSLICH für die Browser-Debug-Anzeige benutzt; ein voll-farbiges PNG an ein echtes Panel zu senden ist auf Hardware UNBEWIESEN. „Es ist nicht abgestürzt" ≠ „es sieht richtig aus". Für JEDE Kombination wird `/tmp/eink_last_sent.png` (E1.2-Artefakt) von einem Menschen visuell begutachtet und im Journal auf die stabile Recovery-Logzeile (E5.4) geprüft.

**Panel A — epd7in3e (6-Farb, Test-Pi):**
- Q-A1: Bildet die Treiber-Buffer-Konvertierung (`epd.getbuffer` auf RGB-Input) arbiträres RGB hart auf die 6 Panelfarben ab (nearest, ohne Dither) — wie beabsichtigt — oder wirft sie einen Fehler / erzeugt Garbage?
- Q-A2: Verhält sich der E1.4-Resize-Guard korrekt, jetzt da der Input RGB statt paletted ist? (Erwartung: im Normalbetrieb 800×480 = Panelgröße ⇒ kein Resize; explizit verifizieren, dass keine unerwartete WARNING/Skalierung auftritt.)
- Q-A3: Wie sieht `original` gegen `dithered` bei einem Foto aus (Banding?) und bei grünem Text (Fringing weg?)? Genau dieser Vergleich ist der Existenzgrund des Features (Querverweis `23553e3`).

**Panel B — epd7in5_V2 (B/W, „Jessica", PRODUKTIONSGERÄT):**
- Q-B1: `original` liefert voll-farbiges RGB → der B/W-Pfad `convert("L").point(threshold)` muss auf 1-Bit schwellen. Ergibt das ein sinnvolles Bild oder Matsch?
- Q-B2: Ist der Text im `original`-Mode auf B/W lesbarer/schärfer als im `dithered`-Mode, oder verliert er durch das harte Threshold Anti-Aliasing-Kanten?
- Q-B3: Produktions-Sicherheit — kein Crash, kein `-1` aus `init()`, sauberes `sleep()`; Content-Skip (E5.2) verhält sich beim Mode-Wechsel wie erwartet (genau ein Extra-Refresh, dann Ruhe).

**Abnahmeregel L3:** Ein Mensch bestätigt pro Kombination anhand von `eink_last_sent.png`, dass das Bild „richtig aussieht" (nicht nur „nicht gecrasht"). Fällt Q-A1 oder Q-B1 durch (Treiber erzeugt Garbage/Matsch), wird `original` NICHT als Default freigegeben und ggf. der UI-Text um eine Panel-Einschränkung ergänzt — das Server-/UI-Setting bleibt gemergt, der Default bleibt `dithered`.

## Risiken

| Risiko | Bewertung / Mitigation |
|---|---|
| **`getbuffer` auf RGB erzeugt auf 6-Farb-Panel Garbage** (Kern-Unbekannte) | Bisher nur Browser-Debug erprobt. L3 Q-A1 ist Pflicht. Default bleibt `dithered` ⇒ kein Nutzer ist ohne aktive Umschaltung betroffen. Rollback: UI-Option entfernen, Server-Feld bleibt inert. |
| **B/W-Threshold auf voll-farbigem Original ergibt Matsch** (Produktionsgerät) | L3 Q-B1/Q-B2 Pflicht auf „Jessica". Bis bestätigt, ist `original` auf B/W eine bewusste Nutzerwahl mit Warntext, nicht Default. |
| **E1.4-Guard verhält sich mit RGB- statt Paletted-Input anders** | Im Normalbetrieb greift der Guard nicht (Größe stimmt, Fakt 3). L3 Q-A2 verifiziert empirisch. Bei zusätzlicher Fehlkonfiguration greift NEAREST korrekt für RGB (E1.4 AC2). |
| **Content-Skip meldet Extra-Refresh als Anomalie** | Bewusst dokumentiert (Fakt 6): Mode-Wechsel ändert Wire-Bytes ⇒ genau ein Extra-Refresh. Kein Bug, in AC5/L3 Q-B3 verankert. |
| **Client liest Feld an falscher Stelle** (aus `display` statt Top-Level) | Explizit als KONTRADIKTION dokumentiert; L5-Review-Punkt; AC5-Test prüft die URL-Wahl. |
| **Scope-Creep Richtung per-Region** | Non-Goal; globaler Schalter ist die erste Antwort, per-Region ist ein Folge-Task. |
| **Rollback gesamt** | Additive Änderung: neues optionales Feld (`omitempty`), neues UI-Control, ein Client-Query-Param. Kein Format-Bruch (alte `settings.json` bleibt gültig). `git revert` der Feature-Commits; persistierte `original`-Werte werden dann als unbekannt beim nächsten `GetSettings` zu `dithered` normalisiert. |

## Scope-Budget

Ziel: < ~300 Zeilen Diff exklusive Tests. Richtwert: Model +~10, Settings-Service +~12, Handler +~12, Preview-Handler +~5, `designer.html` +~10, `designer.js` +~25, `client.py` +~8.
