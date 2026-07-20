# F7: Jahresfortschritt-Widget (`widget_progress`)

> **Pilot-Task.** Dieses Widget etabliert das Widget-Rezept für fünf weitere
> Widgets in dieser Runde. Die Registrierungspunkte sind deshalb genauso
> verbindlich wie das Feature selbst. Nach dem Merge leitet der **docs-writer**
> `docs/adding-a-widget.md` aus **dieser Implementierung** ab (nicht aus diesem
> Spec-Text) — was hier nicht sauber registriert ist, wird als falsches Rezept
> in die Doku kopiert und fünfmal wiederholt.

## Ziel

Ein Element vom Typ `widget_progress` zeigt an, wie viel der laufenden Periode
(Jahr | Monat | Woche | Tag) verstrichen ist — rein lokal berechnet, ohne
Netzwerk —, und wird von Canvas-Preview und Panel-Renderer aus **derselben**
`WidgetTextContent`-Dispatch bedient.

## Kontext

### Zentrale Dispatch (unverändert lassen, nur erweitern)

`server/internal/services/preview.go:355` —
`WidgetTextContent(elemType string, props map[string]any) (content string, ok bool)`,
`switch` bei `:356-380`. Konsumenten:
- `drawElement` (`preview.go:401`) — Panel-Render
- `handlers/widgets.go:43` — `POST /api/widget_content`

**Verifiziert:** beide lesen aus dieser einen Quelle. Das muss wahr bleiben.
Es darf **keine** zweite Content-Formatierung für `widget_progress` entstehen
(insbesondere nicht in `widgets.js`, siehe Punkt 4).

### Registrierungspunkte — ACHT, nicht sieben

Die Kartographie nannte sieben; verifiziert wurden acht. Punkt 8
(`widgets/layouts.go`) fehlte in der Vorlage, ist aber zwingend, sobald das
Widget eine `layout`-Property hat (der Properties-Panel füllt sein
Layout-Dropdown über `GET /api/widgets/{type}/layouts` →
`widgets.GetLayouts`; ohne Eintrag liefert die Funktion nur den generischen
`default`-Eintrag, `layouts.go:11-17`).

| # | Datei | Anker (HEAD 7e71604) | Was eintragen |
|---|-------|----------------------|---------------|
| 1 | `server/static/js/element-factory.js` | `defaultSizes` **:117-127** | `widget_progress: { w: 300, h: 60 }` |
| 2 | `server/static/js/element-factory.js` | `getDefaultProperties` **:194-281** | Property-Defaults (Schema unten) |
| 3 | `server/static/js/properties-panel.js` | `getWidgetPropertyDefs` **:1074-1134** | Feld-Definitionen (Schema unten) |
| 4 | `server/static/js/widgets.js` | `getPreviewContent` **:58-86**, Passthrough-Cases **:67-73** | `case 'widget_progress':` in den **Passthrough-Block** (Server-Content verbatim), NICHT als eigene `build*Content`-Funktion |
| 5 | `server/static/js/widgets.js` | `getDefaultLayout` **:92-103** und Font-Größen **:216-226** | `widget_progress: 'bar_percent'` bzw. `widget_progress: 18` |
| 6 | `server/templates/designer.html` | Palette **:92-127** (nicht :92-124; letzter Eintrag `widget_hass` endet bei :127) | `<div class="widget-item" data-type="widget_progress">` mit Icon `%` und Label `Progress` |
| 7 | `server/internal/services/preview.go` | Font-Größen-`switch` **:259-276** | `case "widget_progress": defaultFontSize = 18` |
| 8 | `server/internal/services/widgets/layouts.go` | `allLayouts` **:19-67**, `allPlaceholders` **:78-107** | Layout-Liste + Placeholder-Liste (unten) |

Abweichungen der Zeilenanker gegenüber der Vorlage: #1 (117-127 statt
118-126), #4 (58-86 statt 63-83), #6 (92-127 statt 92-124). Alle anderen
Anker stimmten exakt.

### Bekanntes Duplikat — NICHT in diesem Task fixen

`preview.go:259-276` (Go) und `widgets.js:216-226` (JS) halten **dieselbe**
Tabelle „Widget-Typ → Default-Font-Size" doppelt. Beide Seiten driften
unbemerkt auseinander (Beleg: `widget_weather` steht in JS mit 18, fehlt im
Go-`switch` komplett und trifft nur zufällig denselben Wert über
`defaultFontSize := 18`).

In diesem Task: **beide Seiten konsistent** auf `18` setzen, Duplikat **nicht**
auflösen (Scope). Stattdessen billige Absicherung: Test `TestWidgetDefaultFontSizesMatchFrontend`
in `server/internal/services/` liest `server/static/js/widgets.js` (der Pfad ist
vom Test aus erreichbar), parst den `defaults`-Block bei `:216-226` per Regex
`(widget_\w+):\s*(\d+)` und vergleicht ihn gegen die Go-Tabelle. Voraussetzung
dafür: die Go-Tabelle wird aus dem `switch` in eine paketprivate
`var widgetDefaultFontSizes = map[string]int{...}` gehoben, die der `switch`
konsultiert (rein mechanisch, ~10 Zeilen). Findet der Test die JS-Datei nicht,
`t.Skip` — kein flakiger Test.

### Zeit-Semantik — definierte Entscheidung, kein Zufall

**Verifiziert:** `settings.go` hat **keine** Timezone-Konfiguration; das
Sleep-Window rechnet auf lokaler Serverzeit (`services/settings.go:122-132`,
`now.Hour()*60+now.Minute()`). `fillClockContent` (`preview.go:483-493`) hat
dagegen eine eigene `timezone`-Property und fällt bei leerem Wert auf
`time.Now().Location()` zurück.

**Gefahr:** Server läuft im Container typischerweise auf UTC, der Pi steht in
Europa/Berlin. „Tag 201 von 365" und ein Jahreswechsel wären dann bis zu 2
Stunden versetzt.

**Entscheidung für F7:** `widget_progress` folgt exakt dem
`widget_clock`-Präzedenzfall:

1. Property `timezone` (String, Default `""`).
2. `timezone != ""` → `time.LoadLocation(tz)`; bei Fehler stillschweigend
   Fallback auf 3.
3. `timezone == ""` → `time.Now().Location()` (Serverzeit; im Container UTC).

Alle Periodengrenzen werden **in dieser Location** bestimmt, nicht in UTC und
nicht gemischt. Die Container-UTC-vs-Pi-Local-Falle wird als Kommentar über
der Berechnungsfunktion dokumentiert und per Test festgenagelt (AC-T4).

### Kein Grafik-Pfad — Balken ist Text

Die Dispatch liefert einen String, den `renderTextV` zeichnet
(`preview.go:401-412`). Ein gezeichneter Balken bräuchte einen eigenen
`drawElement`-Zweig und würde die Single-Source-Eigenschaft brechen (der
Canvas könnte ihn nicht verbatim übernehmen). Der Balken ist deshalb ein
**ASCII-Textbalken**. Unicode-Blockzeichen (U+2588/U+2591) sind verboten: die
eingebettete Fallback-Schrift `goregular` enthält diese Glyphen nicht, das
Panel würde Leerraum oder Tofu rendern.

## Property-Schema

Minimal, acht Felder, alle optional:

| Property | Typ | Default | Wertebereich |
|---|---|---|---|
| `period` | string | `"year"` | `year` \| `month` \| `week` \| `day` |
| `layout` | string | `"bar_percent"` | `bar` \| `percent` \| `bar_percent` \| `count` \| `full` \| `custom` |
| `barWidth` | number | `20` | 5..60 (Zeichen) |
| `timezone` | string | `""` | IANA-Name; `""` = Serverzeit |
| `customTemplate` | string | `"%bar% %percent%"` | nur bei `layout=custom` |
| `fontSize` | number | `18` | 8..200 |
| `color` | string | `"#000000"` | Palettenfarbe |
| `textAlign` | string | `"left"` | left \| center \| right |

Kein `label`, kein `showX`-Flag, keine Farb-/Zeichen-Konfiguration des Balkens.

### Layouts (`layouts.go`)

| ID | Name | Beispiel-Ausgabe (2026-07-20, `period=year`) |
|---|---|---|
| `bar` | Bar | `[###########---------]` |
| `percent` | Percent | `54%` |
| `bar_percent` | Bar + Percent | `[###########---------] 54%` |
| `count` | Count | `Tag 201 von 365` |
| `full` | Full | `Tag 201 von 365\n[###########---------] 54%` |
| `custom` | Custom Template | frei |

### Placeholders (`allPlaceholders`)

`%bar%`, `%percent%`, `%current%`, `%total%`, `%remaining%`, `%period%`

### Formatregeln (verbindlich, weil goldene Datei daran hängt)

- Balken: `[` + `#`×`filled` + `-`×(`barWidth`−`filled`) + `]`,
  `filled = int(ratio * barWidth)` (Abschneiden, nicht Runden),
  `0 ≤ filled ≤ barWidth`.
- `%percent%`: `int(ratio*100)` (Abschneiden) + `%`. Erreicht nie `100%` vor
  dem tatsächlichen Periodenende.
- `count`: `Tag N von M` (Jahr/Monat), `Tag N von 7` (Woche),
  `Stunde N von 24` (Tag). Deutsch, konsistent mit `formatGermanDate`.
- `%remaining%` = `total − current`.

### Periodenmathematik

`ratio = elapsed / total`, beides als `time.Duration` zwischen den
Periodengrenzen in der gewählten Location:

- `year`: `[1. Jan 00:00, 1. Jan 00:00 des Folgejahrs)`
- `month`: `[1. des Monats 00:00, 1. des Folgemonats 00:00)`
- `week`: **ISO-8601**, Woche beginnt **Montag 00:00**, endet Montag 00:00
- `day`: `[00:00, 00:00 des Folgetags)`

Grenzen werden über `time.Date(..., loc)` konstruiert und voneinander
subtrahiert — **nicht** über `24*time.Hour`-Arithmetik. Nur so stimmen
DST-Tage (23h/25h) und Schaltjahre.

`current` in der `count`-Darstellung ist 1-basiert (`YearDay()`,
`Day()`, ISO-Wochentag 1..7, `Hour()+1`).

## Akzeptanzkriterien

**AC1 — Dispatch bleibt Single Source.**
`WidgetTextContent("widget_progress", props)` gibt `ok == true` zurück.
`grep -c "widget_progress" server/static/js/widgets.js` findet den Typ
ausschließlich in `getPreviewContent`s Passthrough-Case, `getDefaultLayout`
und der Font-Size-Tabelle — **keine** `buildProgressContent`-Funktion.

**AC2 — Canvas == Panel, inhaltlich identisch.**
Für eine feste Property-Menge liefert `POST /api/widget_content`
(`{"type":"widget_progress","properties":{...}}`) exakt denselben String, den
`drawElement` zeichnet — bewiesen dadurch, dass beide durch
`WidgetTextContent` laufen; abgesichert durch einen Test, der den
Handler-Response byteweise gegen den Direktaufruf vergleicht
(Muster: `widget_content_test.go:46,73`). Der Canvas rendert diesen String
verbatim via `label.set('text', ...)` (`widgets.js:251`) — kein `innerHTML`.

**AC3 — Alle acht Registrierungspunkte belegt.**
Ein Test/Skript prüft mechanisch je Punkt eine Zeichenkette:
`widget_progress` kommt vor in `element-factory.js` (2×: `defaultSizes`,
`getDefaultProperties`), `properties-panel.js` (1×), `widgets.js` (3×),
`designer.html` (1× als `data-type="widget_progress"`), `preview.go`
(Dispatch + Font-Size), `layouts.go` (Layouts + Placeholders).

**AC4 — Jahresgrenze.**
`period=year`, Location `Europe/Berlin`:
- `2026-01-01 00:00:00` → `ratio == 0`, `0%`, `filled == 0`, `Tag 1 von 365`
- `2026-12-31 23:59:59` → `99%`, `filled == 19` (bei `barWidth=20`),
  `Tag 365 von 365`
- **Kein** Zeitpunkt innerhalb 2026 liefert `100%` oder `Tag 366 von 365`.

**AC5 — Schaltjahr.**
`2028-12-31 12:00` (Schaltjahr) → `Tag 366 von 366`.
`2028-02-29 00:00` ist gültig und liefert `Tag 60 von 366`.
`period=month`, `2028-02-15` → `von 29`; `2026-02-15` → `von 28`.

**AC6 — Wochengrenze ISO-8601.**
`2026-07-20` ist ein Montag: `period=week`, `00:00:00` → `ratio == 0`,
`Tag 1 von 7`. `2026-07-19` (Sonntag) `23:59` → `Tag 7 von 7`, `99%`.
Ein Sonntag darf **nie** `Tag 1` sein.

**AC7 — DST.**
`period=day`, `Europe/Berlin`, `2026-03-29` (23-Stunden-Tag): `total == 23h`;
um `12:00` lokal ist `ratio` = 12h/23h → `52%`, nicht 50 %.
`2026-10-25` (25-Stunden-Tag): `total == 25h`.
Mit `timezone="UTC"` haben beide Tage `total == 24h`.

**AC8 — Timezone-Verhalten definiert.**
`timezone: "UTC"` und `timezone: "Europe/Berlin"` liefern am
`2026-01-01 00:30 UTC` unterschiedliche Ergebnisse (`Tag 1 von 365` vs.
`Tag 1 von 365` bei gleichem Tag, aber unterschiedlichem `ratio`), und
`2025-12-31 23:30 UTC` liefert mit `Europe/Berlin` bereits `Tag 1 von 365`
des Jahres 2026. `timezone: "Mars/Olympus"` (ungültig) fällt still auf
Serverzeit zurück und wirft **keinen** Fehler.

**AC9 — Fehler-/Leerfall.**
- `props == nil` oder `{}` → gültige Ausgabe mit allen Defaults
  (`period=year`, `layout=bar_percent`, `barWidth=20`), **nie** leerer String,
  **nie** Panic.
- `period: "fortnight"` (unbekannt) → Fallback `year`, kein Fehler.
- `barWidth: 0` / `-5` / `9999` → geklemmt auf `[5,60]`.
- `layout: "custom"` mit leerem `customTemplate` → Default-Template
  `"%bar% %percent%"`.
- `POST /api/widget_content` mit `{"type":"widget_progress"}` (ohne
  `properties`) → HTTP 200 mit nichtleerem `content`, nicht 400.

**AC10 — Beide Display-Typen.**
Das Widget rendert auf `waveshare_7in5_v2` (S/W) und `waveshare_7in3_e`
(6 Farben). `assertPaletteExactness` (`golden_test.go:220-280`, arbeitet
generisch über `cfg.Colors`) läuft für die neue Golden-Design ohne Anpassung
grün, d. h. keine Fremdfarbe im Output und ≥ 2 Palettenfarben genutzt.

**AC11 — Kein Netzwerk, keine Dependency.**
`go.mod`/`go.sum` unverändert. Die Content-Funktion enthält keinen
`http`-Aufruf; ein Test mit deaktiviertem Netz (kein `httptest`-Server,
keine Weather-/HTTP-Client-Nutzung) liefert vollständigen Content.

**AC12 — ASCII-Only.**
Jedes Byte des von `layout ∈ {bar, percent, bar_percent}` erzeugten Strings
ist `< 0x80`. (`count`/`full` dürfen deutsche Wörter enthalten, aber keine
Blockzeichen.) Test: `for _, r := range content { r < 128 }`.

**AC13 — Font-Size-Parität.**
`preview.go` und `widgets.js` melden für `widget_progress` beide `18`,
verifiziert durch `TestWidgetDefaultFontSizesMatchFrontend` (siehe oben).

## Test-Anforderungen

Neue Datei `server/internal/services/widget_progress_test.go`.

**Voraussetzung — Clock-Seam.** Verifiziert: `PreviewService` hat **keine**
injizierbare Uhr, alle `fill*Content` rufen `time.Now()` direkt. Es existiert
aber bereits das Muster `testClock`/`setNow` im selben Paket
(`negcache_test.go:16-28`, genutzt in `nominatim_test.go`). F7 fügt
`PreviewService` ein Feld `now func() time.Time` hinzu (Default `time.Now`,
gesetzt in `NewPreviewService`) und nutzt es **ausschließlich** in
`fillProgressContent`. Bestehende `fill*`-Funktionen bleiben unangetastet
(Scope). Ohne diesen Seam ist keines von AC4–AC8 und keine Golden-Datei
deterministisch testbar.

Pflicht-Tests:

1. `TestProgressPeriodMath` — Tabellentest über `(zeit, tz, period, layout)`
   → erwarteter String. Deckt AC4 (Jahresanfang/-ende), AC5 (Schaltjahr 2028,
   Februar 28 vs. 29), AC6 (ISO-Montag/Sonntag), AC7 (DST 2026-03-29 /
   2026-10-25 in `Europe/Berlin` vs. `UTC`).
2. `TestProgressDefaultsAndInvalidInput` — AC9 vollständig, inkl. `nil`-Props.
3. `TestProgressCanvasPanelParity` — AC2: Handler-Response vs.
   `WidgetTextContent`-Direktaufruf, byteidentisch.
4. `TestProgressASCIIOnly` — AC12.
5. `TestWidgetDefaultFontSizesMatchFrontend` — AC13.
6. `TestProgressNoNetwork` — AC11.

**Golden-File-Eintrag.**
`golden_test.go` (verifiziert): `-update`-Flag `:30`, `goldenDesigns` `:33`
(aktuell `basic, gradient, rotation, calibration, rounding`), `goldenDisplays`
`:36` (beide Profile). **Wichtig, weicht von der Vorlage ab:** die bestehenden
Golden-Designs enthalten **null** `widget_*`-Elemente (verifiziert per grep) —
genau weil jedes Widget zeitabhängig ist. F7 ist damit das erste Widget im
Golden-Harness und muss die Determinismus-Lücke selbst schließen:

- Neues Design `server/internal/services/testdata/designs/progress.json` mit
  vier `widget_progress`-Elementen (je ein `period`), `fontFamily`
  `testfont.ttf` gepinnt (Pflicht, siehe `golden_test.go:41-51`),
  `timezone: "Europe/Berlin"` explizit gesetzt.
- `"progress"` in `goldenDesigns` (`:33`) ergänzen.
- `setupGoldenServices` / `newGoldenPreviewService` (`:52-91`) setzt auf dem
  erzeugten `PreviewService` die Uhr auf einen festen Zeitpunkt
  (`2026-07-20 12:00:00 Europe/Berlin`). Ohne das ist die Golden-Datei nach
  einer Minute rot.
- Golden-PNGs für **beide** Displays erzeugen und **im selben Commit** wie der
  Renderer-Code einchecken (Konvention `golden_test.go:26-29`).
- Die fünf bestehenden Golden-Dateien müssen **sha-identisch** bleiben.

## Non-Goals

- **Kein** grafisch gezeichneter Fortschrittsbalken (kein neuer
  `drawElement`-Zweig, keine Rects/Gradienten).
- **Keine** Auflösung des Font-Size-Duplikats `preview.go` ↔ `widgets.js`
  (nur konsistent befüllen + Paritätstest).
- **Keine** globale Timezone-Einstellung in `settings.go` / `.env.example`.
  Nur die widget-lokale `timezone`-Property.
- **Keine** Umstellung anderer `fill*Content`-Funktionen auf den neuen
  Clock-Seam.
- **Keine** neuen Dependencies, kein Build-Step, kein Netzwerkzugriff.
- **Keine** Client-Änderungen (`client/`).
- **Kein** `docs/adding-a-widget.md` in diesem Task — das schreibt der
  docs-writer **nach** dem Merge aus dem gemergten Code.
- **Keine** Lokalisierung/i18n-Infrastruktur; deutsche Strings hartkodiert wie
  bei `formatGermanDate` (`preview.go:521`).

Diff-Budget: **≤ 400 Zeilen** (ohne Golden-PNGs und ohne `progress.json`).

## Verifikation

**L1 — statisch**
```
cd server && gofmt -l . && go vet ./... && go test ./...
```
Erwartung: `gofmt -l` leer, `go vet` still, alle Tests grün inkl. der sechs
neuen und `TestGoldenRender` (7 Designs × 2 Displays).

**L2 — Render-Verifikation**
```
cd server && go test ./internal/services -run 'TestGoldenRender|TestPaletteExactness' -v
git diff --stat server/internal/services/testdata/golden/
```
Erwartung: nur die zwei **neuen** `progress__*.png` erscheinen im Diff; die
zehn bestehenden Golden-Dateien unverändert. Reviewer öffnet beide neuen PNGs
und prüft per Auge: Balken sichtbar, kein Tofu, Text nicht abgeschnitten.

Regenerierung (nur bewusst):
```
cd server && go test ./internal/services -run TestGoldenRender -update
```

**Canvas-Parität manuell**
```
cd server && go run .
```
Widget aus der Palette ziehen, `period` durchschalten; der Canvas-Text muss
zeichengleich dem Panel-Preview (`GET /preview`) entsprechen.

**L5 — Review**
Reviewer prüft explizit: (a) alle acht Registrierungspunkte, (b) keine zweite
Content-Formatierung in JS, (c) Zeitgrenzen über `time.Date(..., loc)` statt
`24*time.Hour`, (d) Clock-Seam nur in `fillProgressContent` verwendet.

L3/L4 nicht erforderlich (keine Hardware-Semantik; das Widget ist reiner
Content im bestehenden Render-Pfad).

## Risiken

| Risiko | Wirkung | Gegenmaßnahme / Rollback |
|---|---|---|
| Golden-Datei wird durch die echte Uhr zeitabhängig | `TestGoldenRender` wird flaky, CI rot für alle | Clock-Seam ist **Voraussetzung**, nicht Kür. Fällt er weg: Design aus `goldenDesigns` entfernen, Widget bleibt (nur Unit-Tests) |
| Clock-Seam in `NewPreviewService` vergessen → `now == nil` | Nil-Panic im Render-Pfad, Panel bleibt schwarz | Defensive Lookup-Helper `s.nowOrDefault()`; Test mit `PreviewService{}`-Zerowert |
| Font-Size-Paritätstest parst JS per Regex | Bricht bei Umformatierung von `widgets.js` | `t.Skip` bei nicht gefundener Datei/Block; Test ist Frühwarnung, kein Gate-Blocker |
| Unicode-Blockzeichen schleichen sich ein | Tofu/Leerraum auf dem Panel, auf dem Canvas sieht es korrekt aus | AC12 als harter Test |
| DST-Rechnung über `24*time.Hour` | `ratio > 1` oder `101%` an zwei Tagen im Jahr | AC7 + Clamping `ratio ∈ [0,1)` |
| Container-UTC vs. Pi-Local | Jahreswechsel/Tageswechsel bis zu 2 h versetzt | Dokumentiertes Verhalten (Serverzeit) + `timezone`-Property als Ausweg; im Widget-Panel als Feld sichtbar |
| Registrierungspunkt vergessen | Widget fehlt in der Palette / ohne Properties / falsche Schriftgröße auf dem Panel — und der docs-writer schreibt den Fehler fest | AC3 als mechanischer Test |

**Rollback:** Ein Commit, isoliert. `git revert` entfernt Widget, Golden-Design
und Golden-PNGs zusammen; bestehende Golden-Dateien und alle anderen Widgets
sind unberührt, weil kein bestehender Code-Pfad verändert wird (außer der
mechanischen `switch`→`map`-Umstellung der Font-Size-Tabelle).

## Nach dem Merge

Der **docs-writer** leitet `docs/adding-a-widget.md` aus der gemergten
F7-Implementierung ab: die acht Registrierungspunkte mit den dann gültigen
Zeilenankern, das Passthrough-Muster für Daten-Widgets, die
Clock-Seam-Konvention für zeitabhängige Widgets und die Golden-File-Checkliste.
Die fünf Folge-Widgets dieser Runde folgen dieser Doku, nicht diesem Spec.
