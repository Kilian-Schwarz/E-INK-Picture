# F7: Jahresfortschritt-Widget (`widget_progress`)

> **Pilot-Task.** Dieses Widget etabliert das Widget-Rezept für fünf weitere
> Widgets in dieser Runde. Die Registrierungspunkte sind deshalb genauso
> verbindlich wie das Feature selbst. Nach dem Merge leitet der **docs-writer**
> `docs/adding-a-widget.md` aus **dieser Implementierung** ab (nicht aus diesem
> Spec-Text) — was hier nicht sauber registriert ist, wird als falsches Rezept
> in die Doku kopiert und fünfmal wiederholt.

> **Stand: vollständig implementiert.** Dieses Spec ist gegen den gemergten
> Code nachgezogen und beschreibt den Ist-Zustand, nicht mehr den Plan. Alle
> acht Registrierungspunkte sind belegt, alle Akzeptanzkriterien sind durch
> Tests abgedeckt, die Golden-PNGs für beide Displays sind eingecheckt. Es gibt
> keine offenen Punkte mehr — der docs-writer kann `docs/adding-a-widget.md`
> ableiten. Alle Zahlenbeispiele sind nachgerechnet und durch Tabellentests
> gepinnt. Zeilenanker beziehen sich, wo nicht anders vermerkt, auf den Stand
> **nach** der Implementierung.

## Ziel

Ein Element vom Typ `widget_progress` zeigt an, wie viel der laufenden Periode
(Jahr | Monat | Woche | Tag) verstrichen ist — rein lokal berechnet, ohne
Netzwerk —, und wird von Canvas-Preview und Panel-Renderer aus **derselben**
`WidgetTextContent`-Dispatch bedient.

## Kontext

### Zentrale Dispatch (unverändert lassen, nur erweitern)

`server/internal/services/preview.go` —
`WidgetTextContent(elemType string, props map[string]any) (content string, ok bool)`
(Signatur `:389`, `switch` `:390-415`). Konsumenten:
- `drawElement` (`preview.go:427`) — Panel-Render
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

Alle acht Punkte sind belegt. Die Anker zeigen auf den **gemergten** Stand;
`TestWidgetRegistrationCompleteness` prüft sie mechanisch, so dass die Tabelle
auch bei verschobenen Zeilen nicht schweigend falsch wird.

| # | Datei | Anker (gemergt) | Eintrag | Status |
|---|-------|-----------------|---------|--------|
| 1 | `server/static/js/element-factory.js` | `defaultSizes` **:117-131** | `widget_progress: { w: 320, h: 60 }` | erledigt (`:130`) |
| 2 | `server/static/js/element-factory.js` | `getDefaultProperties` **:198-300** | Property-Defaults (Schema unten) | erledigt (`:289-299`) |
| 3 | `server/static/js/properties-panel.js` | `getWidgetPropertyDefs` **:1079-…** | Feld-Definitionen (Schema unten) | erledigt (`:1141`) |
| 4 | `server/static/js/widgets.js` | `getPreviewContent` **:67-93** | `case 'widget_progress':` im **Passthrough-Block** (`:83`), Server-Content verbatim, KEINE eigene `build*Content`-Funktion | erledigt |
| 5 | `server/static/js/widgets.js` | `getDefaultLayout` **:96**, `getPreviewFontSize` **:214** | `widget_progress: 'bar_percent'` (`:105`), `widget_progress: 18` (`:232`) | erledigt |
| 6 | `server/templates/designer.html` | Palette | `<div class="widget-item" data-type="widget_progress">` mit Icon `%` und Label `Progress` | erledigt (`:124`) |
| 7 | `server/internal/services/preview.go` | `widgetDefaultFontSizes` **:357-369** | `"widget_progress": 18` | erledigt (`:368`) |
| 8 | `server/internal/services/widgets/layouts.go` | `allLayouts`, `allPlaceholders` | Layout-Liste + Placeholder-Liste (unten) | erledigt (`:67`, `:115`) |

**Warum `w: 320` und nicht 300** (der nicht offensichtliche Teil — die
Vorlage sagte 300, das wäre zu knapp gewesen): das Canvas-Label ist eine
`fabric.Textbox`, also **umbruchfähig**, und `updatePreview` setzt
`label.set('width', w - 16)` (`widgets.js:263`). Der Default-String des
`bar_percent`-Layouts ist `[##########----------] 54%` — 26 Zeichen
Monospace bei 18 px, also grob 26 × 10,8 px ≈ 281 px Textbreite. Bei `w=300`
bleiben `300−16 = 284` px, d. h. nur ~2-3 px Reserve: jede Fontmetrik-Abweichung
bricht den Balken mitten durch (`… 54%` rutscht in Zeile 2, das Element ist mit
`h=60` einzeilig ausgelegt). Bei `w=320` sind es `320−16 = 304` px, also ~23 px
Reserve. `h=60` trägt außerdem das zweizeilige `full`-Layout.

### Bekanntes Duplikat — NICHT in diesem Task fixen

`preview.go` (Go) und `widgets.js:214ff` (JS) halten **dieselbe**
Tabelle „Widget-Typ → Default-Font-Size" doppelt. Beide Seiten driften
unbemerkt auseinander (Beleg: `widget_weather` stand in JS mit 18, fehlte auf
der Go-Seite komplett und traf nur zufällig denselben Wert über den Fallback).

In diesem Task: **beide Seiten konsistent** auf `18` setzen, Duplikat **nicht**
auflösen (Scope). Stattdessen billige Absicherung:
`TestWidgetDefaultFontSizesMatchFrontend` (erledigt,
`widget_registration_test.go:478`) liest `server/static/js/widgets.js`, parst
den `getPreviewFontSize`-Block per Regex `(widget_\w+):\s*(\d+)` und vergleicht
ihn in **beide** Richtungen gegen die Go-Tabelle. Findet der Test die JS-Datei
nicht, `t.Skip` — kein flakiger Test.

Der `switch`→`map`-Umbau ist **erledigt**: die Go-Tabelle ist die paketprivate
`var widgetDefaultFontSizes = map[string]int{...}` (`preview.go:357-369`), der
Lookup läuft über `defaultFontSizeFor` (`:375-380`) mit
`widgetFallbackFontSize = 18` (`:372`).

**Bewusste Abweichung vom ursprünglichen Non-Goal: `widget_weather` steht
jetzt in der Go-Tabelle** (`preview.go:360`). Der Plan hatte das ausdrücklich
ausgeschlossen; die Implementierung hat es trotzdem getan, und zwar begründet:
`TestWidgetDefaultFontSizesMatchFrontend` vergleicht die beiden Tabellen
symmetrisch und verlangt für jeden Dispatch-Typ einen **expliziten** Eintrag
auf beiden Seiten — denn die Fallbacks unterscheiden sich (Go: 18,
`widgets.js`: `|| 14`), ein Typ auf dem Fallback rendert also auf Canvas und
Panel verschieden groß. Ohne den `widget_weather`-Eintrag bräuchte der Test
eine Ausnahmeliste, und die würde genau die Drift verstecken, gegen die der
Test existiert. **Verifiziert ohne Wirkung auf das Rendering:** der Typ traf
über `widgetFallbackFontSize` bereits dieselben 18; die Golden-PNGs sind
byteidentisch geblieben.

### Zeit-Semantik — definierte Entscheidung, kein Zufall

**Verifiziert:** `settings.go` hat **keine** Timezone-Konfiguration; das
Sleep-Window rechnet auf lokaler Serverzeit (`services/settings.go:122-132`,
`now.Hour()*60+now.Minute()`). `fillClockContent` hat dagegen eine eigene
`timezone`-Property und fällt bei leerem Wert auf `time.Now().Location()`
zurück.

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
der Berechnungsfunktion dokumentiert (umgesetzt: `widget_progress.go:34-41`)
und per Test festgenagelt.

### Kein Grafik-Pfad — Balken ist Text

Die Dispatch liefert einen String, den `renderTextV` zeichnet
(`preview.go:423-434`). Ein gezeichneter Balken bräuchte einen eigenen
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

Referenzzeitpunkt aller Beispiele: **2026-07-20 12:00 Europe/Berlin**,
`period=year`, `barWidth=20`.

Nachrechnung (verbindlich, weil die goldene Datei daran hängt):
`elapsed = 200 Tage 12 h − 1 h` (die Sommerzeitumstellung am 2026-03-29 liegt
im Intervall) `= 4811 h`, `total = 8760 h` → `ratio = 0,549201`.
`filled = int(0,549201 × 20) = int(10,984) = **10**`,
`percent = int(0,549201 × 100) = **54**`.

| ID | Name | Beispiel-Ausgabe |
|---|---|---|
| `bar` | Bar | `[##########----------]` |
| `percent` | Percent | `54%` |
| `bar_percent` | Bar + Percent | `[##########----------] 54%` |
| `count` | Count | `Tag 201 von 365` |
| `full` | Full | `Tag 201 von 365\n[##########----------] 54%` |
| `custom` | Custom Template | frei |

Zehn `#`, zehn `-`. Elf `#` wäre Aufrunden und widerspricht der
Abschneide-Regel unten.

### Placeholders (`allPlaceholders`)

`%bar%`, `%percent%`, `%current%`, `%total%`, `%remaining%`, `%period%`

### Formatregeln (verbindlich, weil goldene Datei daran hängt)

- Balken: `[` + `#`×`filled` + `-`×(`barWidth`−`filled`) + `]`,
  `filled = int(ratio * barWidth)` (Abschneiden, nicht Runden),
  `0 ≤ filled ≤ barWidth`.
- `%percent%`: `int(ratio*100)` (Abschneiden) + `%`. Erreicht nie `100%` vor
  dem tatsächlichen Periodenende.
- `count`: `Tag N von M` (Jahr/Monat), `Tag N von 7` (Woche),
  `Stunde N von 24` (Tag). Deutsch, konsistent mit `formatGermanDate`
  (`preview.go:543`).
- `%remaining%` = `total − current`.

**Bewusste Inkonsistenz an DST-Tagen.** Der `count`-Layout meldet für
`period=day` immer `von 24`, obwohl der Tag 23 bzw. 25 Stunden hat, und
`current = Hour()+1` überspringt bzw. wiederholt eine Stunde. `ratio`/`percent`
rechnen dagegen exakt mit 23 h/25 h. Das ist so gewollt: „Stunde 13 von 24"
bleibt für den Leser sinnvoller als „Stunde 12 von 23". Der Prozentwert bleibt
die genaue Größe.

## Periodenmathematik

`ratio = elapsed / total`, beides als `time.Duration` zwischen den
Periodengrenzen in der gewählten Location:

- `year`: `[1. Jan 00:00, 1. Jan 00:00 des Folgejahrs)`
- `month`: `[1. des Monats 00:00, 1. des Folgemonats 00:00)`
- `week`: **ISO-8601**, Woche beginnt **Montag 00:00**, endet Montag 00:00
- `day`: `[00:00, 00:00 des Folgetags)`

Grenzen werden über `time.Date(..., loc)` konstruiert und voneinander
subtrahiert — **nicht** über `24*time.Hour`-Arithmetik. Nur so stimmen
DST-Tage (23h/25h) und Schaltjahre.

`ratio` wird auf `[0, 1)` geklemmt (`math.Nextafter(1, 0)` als Obergrenze), so
dass `100%` innerhalb der Periode unerreichbar bleibt.

`current` in der `count`-Darstellung ist 1-basiert (`YearDay()`,
`Day()`, ISO-Wochentag 1..7, `Hour()+1`).

## Akzeptanzkriterien

**AC1 — Dispatch bleibt Single Source. (erledigt)**
`WidgetTextContent("widget_progress", props)` gibt `ok == true` zurück
(`preview.go:411-412`). In `widgets.js` kommt der Typ (Kommentare entfernt)
in **genau drei** Rollen vor — Passthrough-Case `:83`, `getDefaultLayout`
`:105`, `getPreviewFontSize` `:232` — und in keiner weiteren; **keine**
`buildProgressContent`-Funktion. `TestProgressCanvasPanelParity` zählt das
mechanisch (Punkt 4 des Tests) und verbietet zusätzlich JS-seitige
Progress-Primitive (`'#'.repeat`, `barWidth`, …).

**AC2 — Canvas == Panel, inhaltlich identisch.**
Für eine feste Property-Menge liefert `POST /api/widget_content`
(`{"type":"widget_progress","properties":{...}}`) exakt denselben String, den
`drawElement` zeichnet — bewiesen dadurch, dass beide durch
`WidgetTextContent` laufen; abgesichert durch einen Test, der den
Handler-Response byteweise gegen den Direktaufruf vergleicht
(Muster: `widget_content_test.go:46,73`). Der Canvas rendert diesen String
verbatim via `label.set('text', ...)` (`widgets.js:251`) — kein `innerHTML`.

**AC3 — Alle acht Registrierungspunkte belegt. (erledigt)**
`TestWidgetRegistrationCompleteness` (`widget_registration_test.go:190`) prüft
mechanisch je Punkt eine Zeichenkette: `widget_progress` kommt vor in
`element-factory.js` (2×: `defaultSizes`, `getDefaultProperties`),
`properties-panel.js` (1×), `widgets.js` (3×), `designer.html` (1× als
`data-type="widget_progress"`), `preview.go` (Dispatch + Font-Size),
`layouts.go` (Layouts + Placeholders).

Wichtig für die Folge-Widgets: der Test leitet die Typenliste per AST aus
`WidgetTextContent` selbst ab, prüft also **jedes neue Widget automatisch**,
sobald es in der Dispatch auftaucht — niemand muss hier eine Liste pflegen.
Die Punkte 5a/8 (Layouts) gelten nur für Widgets, die `props["layout"]`
tatsächlich lesen; das wird in beide Richtungen geprüft, so dass weder eine
fehlende Registrierung noch eine veraltete Ausnahme überlebt.
`TestProgressLayoutsRegistered` deckt Punkt 8 zusätzlich direkt ab.

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
`period=day`, `Europe/Berlin`. An DST-Tagen verschiebt sich **beides**: die
Gesamtlänge des Tages *und* die seit Ortsmitternacht verstrichene Zeit.

- `2026-03-29` (Spring Forward, 23-Stunden-Tag): `total == 23h`. Weil die Uhr
  um 02:00 auf 03:00 springt, liegt `12:00` lokal nur **11 h** nach
  Ortsmitternacht → `ratio = 11/23 = 0,4783` → **`47%`**, nicht 50 %.
- `2026-10-25` (Fall Back, 25-Stunden-Tag): `total == 25h`. `12:00` lokal liegt
  **13 h** nach Ortsmitternacht → `ratio = 13/25 = 0,52` → **`52%`**,
  nicht 50 %.
- Mit `timezone="UTC"` haben beide Tage `total == 24h` und `12:00` → `50%`.

**AC8 — Timezone-Verhalten definiert.**
`timezone: "UTC"` und `timezone: "Europe/Berlin"` liefern am
`2026-01-01 00:30 UTC` beide `Tag 1 von 365`, aber unterschiedliche `ratio`
(Berlin ist dort bereits 01:30 Ortszeit). `2025-12-31 23:30 UTC` liefert mit
`Europe/Berlin` bereits `Tag 1 von 365` des Jahres 2026, mit `UTC` dagegen noch
`Tag 365 von 365`. `timezone: "Mars/Olympus"` (ungültig) fällt still auf
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

**AC10 — Beide Display-Typen. (erledigt)**
Das Widget rendert auf `waveshare_7in5_v2` (S/W) und `waveshare_7in3_e`
(6 Farben). `assertPaletteExactness` (`golden_test.go:220-280`, arbeitet
generisch über `cfg.Colors`) läuft für die neue Golden-Design ohne Anpassung
grün, d. h. keine Fremdfarbe im Output und ≥ 2 Palettenfarben genutzt.

**AC11 — Kein Netzwerk, keine Dependency.**
`go.mod`/`go.sum` unverändert. Die Content-Funktion enthält keinen
`http`-Aufruf; ein Test mit deaktiviertem Netz (kein `httptest`-Server,
keine Weather-/HTTP-Client-Nutzung) liefert vollständigen Content —
abgesichert dadurch, dass alle Progress-Tests auf einem `&PreviewService{}`
ohne jede verdrahtete Abhängigkeit laufen.

**AC12 — ASCII-Only.**
Jedes Byte des von `layout ∈ {bar, percent, bar_percent}` erzeugten Strings
ist `< 0x80`. (`count`/`full` dürfen deutsche Wörter enthalten, aber keine
Blockzeichen.) Test: `TestProgressASCIIOnly`.

**AC13 — Font-Size-Parität. (erledigt)**
`preview.go:368` und `widgets.js:232` melden für `widget_progress` beide `18`,
verifiziert durch `TestWidgetDefaultFontSizesMatchFrontend`. Der
`switch`→`map`-Umbau der Go-Tabelle ist erfolgt; der Test verlangt für jeden
Dispatch-Typ einen expliziten Eintrag auf beiden Seiten (siehe die Begründung
für den `widget_weather`-Nachtrag oben).

## Test-Anforderungen

Datei `server/internal/services/widget_progress_test.go`.

**Voraussetzung — Clock-Seam.** Verifiziert: `PreviewService` hatte **keine**
injizierbare Uhr, alle `fill*Content` riefen `time.Now()` direkt. F7 fügt
`PreviewService` ein Feld `now func() time.Time` hinzu (Default `time.Now`,
gesetzt in `NewPreviewService`, defensiver Lookup über
`nowOrDefault()`, `preview.go:103`) und nutzt es **ausschließlich** in
`fillProgressContent`. Bestehende `fill*`-Funktionen bleiben unangetastet
(Scope). Ohne diesen Seam ist keines von AC4–AC8 und keine Golden-Datei
deterministisch testbar.

Konvention für zeitabhängige Folge-Widgets: `nowOrDefault()` **einmal** in eine
lokale Variable lesen und alles davon ableiten (`widget_progress.go:42-60`).
Zwei Aufrufe können über eine Perioden-/Sekundengrenze hinweg auseinanderfallen.

Tests in `server/internal/services/widget_progress_test.go`:

1. `TestProgressPeriodMath` — Tabellentest über `(zeit, tz, period, layout)`
   → erwarteter String. Deckt AC4 (Jahresanfang/-ende), AC5 (Schaltjahr 2028,
   Februar 28 vs. 29), AC6 (ISO-Montag/Sonntag), AC7 (DST 2026-03-29 → `47%` /
   2026-10-25 → `52%` in `Europe/Berlin`, `50%` in `UTC`).
2. `TestProgressPeriodDurations` — Tageslänge 23h/25h/24h direkt auf
   `progressPeriodBounds`.
3. `TestProgressNeverCompletesWithinPeriod` — AC4, dritter Punkt.
4. `TestProgressTimezoneHandling` — AC8 inkl. ungültiger Zone.
5. `TestProgressDefaultsAndInvalidInput` — AC9 vollständig, inkl. `nil`-Props.
6. `TestProgressZeroValueServiceDoesNotPanic` — Clock-Seam-Nil-Schutz.
7. `TestProgressASCIIOnly` — AC12.
8. `TestProgressLayoutsRegistered` — Registrierungspunkt 8.

In `server/internal/services/widget_registration_test.go` (erledigt):

9. `TestProgressCanvasPanelParity` (`:350`) — AC2: `WidgetTextContent` vs.
   `fillProgressContent` byteidentisch, plus die vier statischen
   JS-Prüfungen (Passthrough-Case, kein `build*Content`, keine Progress-Mathe,
   genau drei Vorkommen).
10. `TestWidgetDefaultFontSizesMatchFrontend` (`:478`) — AC13.
11. `TestWidgetRegistrationCompleteness` (`:190`) + `TestDispatchWidgetTypesFound`
    (`:175`) — AC3 inkl. Selbstschutz des AST-Parsers gegen vakuum-grüne Läufe.
12. `TestDeadPlaceholderRegistry` (`:279`) — pinnt einen **vorbestehenden**
    Defekt (`widget_calendar`/`widget_news` bewerben Placeholder, die ihre
    `fill*Content` nie substituiert) als exakte Menge fest. Nicht Teil von F7;
    er darf weder wachsen noch unbemerkt verschwinden.

Fallstrick für Folge-Widgets: `GetPropInt` (`design.go:1005-1017`) dekodiert
nur `float64` und `string` — die Formen, die aus einer JSON-Design-Datei
kommen. Ein `int`-Literal in einer Test-Property-Map wird **stillschweigend
verworfen** und der Default greift. In Tests deshalb immer `float64(20)`
schreiben, nie `20`.

**Golden-File-Eintrag. (erledigt)**
`golden_test.go`: `-update`-Flag `:31`, `goldenDesigns` `:34` (enthält
`progress`), `goldenDisplays` `:73` (beide Profile), fixe Uhr `goldenNow`
`:50` (`2026-07-20 12:00 CEST`), gesetzt in `newGoldenPreviewService` `:129`.
**Wichtig, weicht von der Vorlage ab:** die bestehenden Golden-Designs
enthielten **null**
`widget_*`-Elemente (verifiziert per grep) — genau weil jedes Widget
zeitabhängig ist. F7 ist damit das erste Widget im Golden-Harness und hat die
Determinismus-Lücke selbst geschlossen:

- Design `server/internal/services/testdata/designs/progress.json` mit
  vier `widget_progress`-Elementen (je ein `period`), `fontFamily`
  `testfont.ttf` gepinnt (Pflicht), `timezone: "Europe/Berlin"` explizit
  gesetzt.
- `"progress"` steht in `goldenDesigns` (`:34`).
- `newGoldenPreviewService` (`:122`) setzt `svc.now` auf `goldenNow` (`:129`).
  Ohne das wäre die Golden-Datei nach einer Minute rot. Bei diesem Zeitpunkt
  zeigt das `year`-Element `[##########----------] 54%` — derselbe String, den
  `TestProgressCanvasPanelParity` pinnt.
- Golden-PNGs für **beide** Displays sind eingecheckt
  (`progress__waveshare_7in3_e.png`, `progress__waveshare_7in5_v2.png`),
  im selben Commit wie der Renderer-Code (Konvention `golden_test.go:26-30`).
- Die zehn bestehenden Golden-Dateien sind **byteidentisch** geblieben.

## Non-Goals

- **Kein** grafisch gezeichneter Fortschrittsbalken (kein neuer
  `drawElement`-Zweig, keine Rects/Gradienten).
- **Keine** Auflösung des Font-Size-Duplikats `preview.go` ↔ `widgets.js`
  (nur konsistent befüllen + Paritätstest). Die Tabellen bleiben doppelt; nur
  die Go-Seite wurde mechanisch von `switch` auf `map` umgestellt, damit der
  Paritätstest sie lesen kann.
  **Bewusst überschriebenes Non-Goal:** der `widget_weather`-Nachtrag in die
  Go-Tabelle war ursprünglich ausgeschlossen, ist aber erfolgt — Begründung
  siehe „Bekanntes Duplikat" oben (ohne ihn bräuchte der Paritätstest eine
  Ausnahmeliste, die genau die Drift verdeckt, gegen die er existiert).
  Rendering nachweislich unverändert.
- **Keine** globale Timezone-Einstellung in `settings.go` / `.env.example`.
  Nur die widget-lokale `timezone`-Property.
- **Keine** Umstellung anderer `fill*Content`-Funktionen auf den neuen
  Clock-Seam.
- **Keine** neuen Dependencies, kein Build-Step, kein Netzwerkzugriff.
- **Keine** Client-Änderungen (`client/`).
- **Kein** `docs/adding-a-widget.md` in diesem Task — das schreibt der
  docs-writer **nach** dem Merge aus dem gemergten Code.
- **Keine** Lokalisierung/i18n-Infrastruktur; deutsche Strings hartkodiert wie
  bei `formatGermanDate` (`preview.go:543`).

Diff-Budget: **≤ 400 Zeilen** (ohne Golden-PNGs und ohne `progress.json`).

## Verifikation

**L1 — statisch**
```
cd server && gofmt -l . && go vet ./... && go test ./...
```
Erwartung: `gofmt -l` leer, `go vet` still, alle Tests grün inkl. der neuen
und `TestGoldenRender`.

**L2 — Render-Verifikation** (erst nach dem Golden-Design sinnvoll)
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
| Clock-Seam in `NewPreviewService` vergessen → `now == nil` | Nil-Panic im Render-Pfad, Panel bleibt schwarz | Defensiver Lookup-Helper `s.nowOrDefault()`; Test mit `PreviewService{}`-Zerowert |
| Font-Size-Paritätstest parst JS per Regex | Bricht bei Umformatierung von `widgets.js` | `t.Skip` bei nicht gefundener Datei/Block; Test ist Frühwarnung, kein Gate-Blocker |
| Unicode-Blockzeichen schleichen sich ein | Tofu/Leerraum auf dem Panel, auf dem Canvas sieht es korrekt aus | AC12 als harter Test |
| DST-Rechnung über `24*time.Hour` | `ratio > 1` oder `101%` an zwei Tagen im Jahr | AC7 + Clamping `ratio ∈ [0,1)` |
| Beispielwerte im Spec statt aus der Regel abgeleitet | Falsche Zahlen wandern in `docs/adding-a-widget.md` und von dort in fünf Folge-Widgets | Jedes Beispiel in diesem Spec ist nachgerechnet und durch einen Tabellentest in `widget_progress_test.go` gepinnt |
| Container-UTC vs. Pi-Local | Jahreswechsel/Tageswechsel bis zu 2 h versetzt | Dokumentiertes Verhalten (Serverzeit) + `timezone`-Property als Ausweg; im Widget-Panel als Feld sichtbar |
| Registrierungspunkt vergessen | Widget fehlt in der Palette / ohne Properties / falsche Schriftgröße auf dem Panel — und der docs-writer schreibt den Fehler fest | AC3 als mechanischer Test |

**Rollback:** `git revert` entfernt Widget, Golden-Design und Golden-PNGs
zusammen; bestehende Golden-Dateien und alle anderen Widgets sind unberührt,
weil kein bestehender Code-Pfad verändert wird (außer der mechanischen
`switch`→`map`-Umstellung der Font-Size-Tabelle).

## Offener Rest

**Keiner.** Alle ursprünglich offenen Punkte sind geschlossen:

1. Registrierungspunkte 1–6 (Frontend) — erledigt, siehe Tabelle oben.
2. Golden-Design `testdata/designs/progress.json` + `"progress"` in
   `goldenDesigns` + fixe Uhr in `newGoldenPreviewService` + beide PNGs
   (AC10) — erledigt.
3. `switch`→`map`-Umbau der Go-Font-Size-Tabelle und
   `TestWidgetDefaultFontSizesMatchFrontend` (AC13) — erledigt.
4. Mechanischer Registrierungstest über alle acht Punkte (AC3) — erledigt.
5. `TestProgressCanvasPanelParity` (AC2) — erledigt.

Bekannte, **bewusst nicht** in F7 gefixte Altlasten: die toten Placeholder von
`widget_calendar`/`widget_news` (durch `TestDeadPlaceholderRegistry` als exakte
Menge eingefroren) und das Font-Size-Duplikat selbst.

## Nach dem Merge

Der **docs-writer** leitet `docs/adding-a-widget.md` aus der gemergten
F7-Implementierung ab: die acht Registrierungspunkte mit den dann gültigen
Zeilenankern, das Passthrough-Muster für Daten-Widgets, die
Clock-Seam-Konvention für zeitabhängige Widgets und die Golden-File-Checkliste.
Die fünf Folge-Widgets dieser Runde folgen dieser Doku, nicht diesem Spec.
