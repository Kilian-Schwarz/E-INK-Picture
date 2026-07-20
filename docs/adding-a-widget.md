# Ein neues Widget hinzufügen

Abgeleitet aus der gemergten F7-Implementierung (`widget_progress`). Referenz
für alle weiteren Widgets: Luftqualität, Strommix, ÖPNV, Foto-Slideshow.
Feiertage (F4) sind inzwischen ebenfalls gemergt und die Referenz für ein
Widget **ohne** Netzpfad.

Referenzdateien, die beim Bauen offen sein sollten:

- `server/internal/services/widget_progress.go` — Referenz-Implementierung
- `server/internal/services/preview.go` — `WidgetTextContent`, Font-Size-Tabelle, Clock-Seam
- `server/internal/services/widgets/layouts.go` — Layouts und Placeholders
- `server/internal/services/widget_registration_test.go` — mechanischer Vollständigkeitstest
- `server/internal/services/widget_progress_test.go` — Test-Muster
- `server/internal/services/golden_test.go` — Golden-Harness

---

## 1. Die acht Registrierungspunkte

Zuerst die Voraussetzung, die keiner der acht Punkte ist: die
Content-Funktion selbst und ihr `case` in der Dispatch.

**Punkt 0 — `server/internal/services/widget_<name>.go`**

Neue Datei mit `func (s *PreviewService) fill<Name>Content(props map[string]any) string`.
Dazu ein `case "widget_<name>": return s.fill<Name>Content(props), true` in
`WidgetTextContent` (`preview.go`, aktuell `:389-416`).

Der Test leitet die Typenliste per AST aus genau diesem `switch` ab
(`dispatchWidgetFills`). Sobald der `case` steht, prüft
`TestWidgetRegistrationCompleteness` das Widget automatisch gegen alle acht
Punkte — niemand pflegt dafür eine Liste. Solange der `case` fehlt, ist das
Widget für die Tests unsichtbar und `/api/widget_content` antwortet mit
HTTP 400 (`handlers/widgets.go:44-47`).

Danach die acht Punkte in dieser Reihenfolge:

| Reihenfolge | Punkt (Testnummer) | Datei / Stelle | Eintrag | Bricht ohne Eintrag |
|---|---|---|---|---|
| 1 | 7 | `services/preview.go`, `widgetDefaultFontSizes` (aktuell `:357-369`) | `"widget_<name>": <px>` | Go fällt auf `widgetFallbackFontSize` (18), `widgets.js` auf `14` — Canvas und Panel rendern denselben Text in unterschiedlicher Größe. `TestWidgetDefaultFontSizesMatchFrontend` schlägt fehl. |
| 2 | 8 | `services/widgets/layouts.go`, `allLayouts` und `allPlaceholders` | Layout-Liste; Placeholder-Liste nur bei `customTemplate` | Ohne `allLayouts`-Eintrag liefert `GetLayouts` nur den generischen `default` (`layouts.go:11-17`) → Layout-Dropdown im Properties-Panel ist leer bzw. zeigt nur „Default". Ohne `allPlaceholders` bietet das Panel keine Placeholder-Chips an. |
| 3 | 1 | `static/js/element-factory.js`, `defaultSizes` (aktuell `:117-131`) | `widget_<name>: { w: …, h: … }` | Neu gezogenes Widget bekommt `{ w: 200, h: 100 }` (`:133`) — Text wird abgeschnitten oder umbricht. |
| 4 | 2 | `static/js/element-factory.js`, `getDefaultProperties` (aktuell `:198-302`) | Property-Defaults, spiegelbildlich zu den Go-Defaults | Widget startet mit `{}`; die erste Server-Antwort nutzt Go-Defaults, das Panel zeigt etwas anderes als der Nutzer eingestellt hat. |
| 5 | 3 | `static/js/properties-panel.js`, `getWidgetPropertyDefs` (aktuell `:1079-1157`) | Feld-Definitionen (`type: text/number/select/checkbox`, `default`, `min`, `max`) | Properties-Panel zeigt für das Widget keine Felder — es ist nur per JSON konfigurierbar. |
| 6 | 4 | `static/js/widgets.js`, `getPreviewContent` (aktuell `:67-90`) | `case 'widget_<name>':` im **Passthrough-Block** (`:83`) | Canvas zeigt nur das Typ-Label (`_widgetTypeLabel`, `:92`) statt des echten Inhalts. |
| 7 | 5a | `static/js/widgets.js`, `getDefaultLayout` (aktuell `:96-108`) | `widget_<name>: '<layout-id>'` | Canvas fragt mit `'default'` an; ein Widget mit Layouts rendert die falsche Variante. Nur nötig, wenn `fill*Content` `props["layout"]` liest. |
| 8 | 5b | `static/js/widgets.js`, `getPreviewFontSize` (aktuell `:214-235`) | `widget_<name>: <px>` — identisch zu Punkt 7 (Go) | Siehe Punkt 1: Größendrift zwischen Canvas und Panel. |
| 9 | 6 | `templates/designer.html`, Widget-Palette (aktuell `:80-132`) | `<div class="widget-item" data-type="widget_<name>">` mit `<span class="widget-icon">` und Label | Widget taucht in der Palette nicht auf und ist im Designer nicht erreichbar. |

Die Tabelle hat neun Zeilen für acht Punkte: **5a und 5b sind ein Punkt** (beide
in `widgets.js`), stehen aber als zwei Zeilen, weil sie zwei verschiedene
Funktionen betreffen und unabhängig voneinander vergessen werden können.

**Die Zeilenangaben sind Anker auf den heutigen Stand und driften.** Autorität
ist `TestWidgetRegistrationCompleteness` (`widget_registration_test.go`, aktuell
`:190`): der Test schneidet die JS-Dateien per Brace-Matching auf die
jeweilige Funktion zu und prüft je Punkt eine Zeichenkette. Wenn die Tabelle
und der Test sich widersprechen, gilt der Test. Findet der Test eine Datei
oder einen Block nicht, macht er `t.Skip` statt rot zu werden — ein
übersprungener Registrierungstest ist also selbst ein Befund.

Ein zweiter Anker in derselben Datei ist die Untergrenze in
`TestDispatchWidgetTypesFound` (`:182`): `if len(types) < 8`. Sie sichert den
AST-Parser gegen stilles Nichts-Finden ab. Beim **Hinzufügen** eines Widgets ist
sie irrelevant (aktuell 11 Typen). Sie bricht, wenn Widgets **entfernt** werden
oder wenn der Parser aufhört zu greifen — dann ist die Zahl anzupassen bzw. der
Parser zu reparieren, nicht die Grenze zu senken.

Punkt 5a gilt nur für Widgets, die `props["layout"]` lesen. Punkt 8 hängt an
**beiden** Properties, nicht 1:1 an einer: `allLayouts` wird über
`props["layout"]` erzwungen (`widget_registration_test.go:244-250`),
`allPlaceholders` unabhängig davon über `props["customTemplate"]` (`:256-258`).
Ein Widget kann also Punkt 8 zur Hälfte erfüllen müssen. Für `layout` prüft der
Test in **beide** Richtungen: ein Layout-Eintrag ohne Leser ist ebenso ein
Fehler wie ein Leser ohne Eintrag (`:251-253`).

Wer Placeholders in `allPlaceholders` einträgt, muss `customTemplate` auch
wirklich substituieren. `TestDeadPlaceholderRegistry` (`:279`) friert die
bestehende Altlast (`widget_calendar`, `widget_news`) als exakte Menge ein und
schlägt bei jedem neuen toten Eintrag fehl.

---

## 2. Single-Source-Regel

`WidgetTextContent` ist der **einzige** Ort, an dem Widget-Content entsteht.
Zwei Konsumenten lesen daraus:

- `drawElement` (`preview.go:437`) — der Panel-Renderer
- `WidgetHandler.Content` (`handlers/widgets.go:43`) — `POST /api/widget_content`

Der Designer-Canvas holt seinen Text über `POST /api/widget_content`
(`widgets.js:33`) und setzt ihn unverändert per `label.set('text', …)`. Das ist
der Passthrough-Block in `getPreviewContent` (`widgets.js:83-86`).

**Nie** Formatierung oder Rechenlogik in JS nachbauen. Kein
`build<Name>Content` in `widgets.js`. Die beiden Pfade dürfen nicht forken —
das hat einmal eine ganze Epic gekostet, weil Canvas und Panel unterschiedliche
Strings zeigten und der Fehler nur auf dem Papier reproduzierbar war.

Ausnahmen sind ausschließlich `widget_clock` und `widget_timer`: sie ticken pro
Sekunde und würden als Server-Render einfrieren. Ein neues Widget gehört nicht
in diese Gruppe.

Absicherung, nach dem Muster von `TestProgressCanvasPanelParity`
(`widget_registration_test.go:350`): der Test prüft, dass der Typ im
Passthrough-Zweig steht, dass der Zweig weiterhin `liveData.content` verbatim
zurückgibt, dass keine `build*`-Funktion existiert, dass keine
widget-spezifischen Rechenprimitive in `widgets.js` auftauchen — und dass der
Typ in `widgets.js` **exakt so oft** vorkommt wie er registrierte Rollen hat
(bei `widget_progress`: dreimal). Jedes weitere Vorkommen ist per Definition ein
zweiter Codepfad.

Weitere Konsequenz aus der Single-Source-Regel: **kein eigener Grafikpfad.**
Ein gezeichneter Balken bräuchte einen eigenen `drawElement`-Zweig, den der
Canvas nicht übernehmen kann. Deshalb ist der Progress-Balken ASCII
(`[`, `#`, `-`, `]`).

### Zeichenvorrat: die Grenze ist U+0100, nicht ASCII

Die Fallback-Schrift ist `goregular` (`preview.go:1453`, Cache-Key
`__goregular__`); sie greift, sobald `fontFamily` nicht auflösbar ist. Ihr
Glyphenvorrat ist die eigentliche Schranke — und die verläuft **nicht** bei
ASCII:

- **Erlaubt: alles unter U+0100 (Latin-1).** `ä ö ü ß Ä Ö Ü ° § « »` sind
  non-ASCII und werden gerendert. Präzedenzfall ist `germanMonths` mit `März`
  (`locale.go:22`), das über `preview.go:558` seit jeher auf beiden Panels
  steht. Ein deutschsprachiges Widget darf und soll Umlaute schreiben.
- **Verboten: alles ab U+0100.** Verifiziert fehlend sind die Blockzeichen
  U+2588/U+2591 (`widget_progress.go:15`) — das Panel rendert Leerraum oder
  Tofu, während der Browser-Canvas korrekt aussieht. Für den Rest gibt es
  schlicht keinen verifizierten Präzedenzfall, und ein Fehlversuch ist auf dem
  Panel still.

Explizit verboten, weil man sie beim Formatieren von `Datum — Name` versehentlich
tippt (oder der Editor sie einsetzt):

| Falsch | Codepoint | Richtig |
|---|---|---|
| `–` En-Dash | U+2013 | `-` |
| `—` Em-Dash | U+2014 | `-` oder ` - ` |
| `…` Ellipse | U+2026 | `...` |
| `„ " ' '` typografische Anführungszeichen | U+201E/U+201C/U+2018/U+2019 | `"` `'` |
| `→` Pfeil | U+2192 | `->` |
| `×` Malzeichen | U+00D7 | erlaubt (< U+0100), aber `x` ist eindeutiger |

Achtung, kein Freibrief: `widget_hass` führt heute `—` (U+2014) und `…`
(U+2026) in `locale.go:88,107`. Diese Strings sind **nie gegen das Panel
verifiziert** worden — sie sind Altbestand, kein Präzedenzfall. Nicht kopieren.

---

## 3. Clock-once-Konvention

`PreviewService` hat einen injizierbaren Uhr-Seam (`preview.go:92-108`):

```go
now func() time.Time          // Feld, gesetzt in NewPreviewService
func (s *PreviewService) nowOrDefault() time.Time
```

Regeln:

- Die Uhr **nie** direkt über `s.now` lesen, immer über `nowOrDefault()`. Ein
  `PreviewService`-Zerowert (z. B. in Tests) hat `now == nil` und würde im
  Renderpfad paniken.
- `nowOrDefault()` **einmal** in eine lokale Variable lesen und alles daraus
  ableiten (`widget_progress.go:52`). Zwei Aufrufe können über eine Sekunden-,
  Stunden- oder Periodengrenze hinweg auseinanderfallen; das Ergebnis ist ein
  in sich widersprüchlicher String, der sich nicht reproduzieren lässt.
- Zeitgrenzen mit `time.Date(…, loc)` konstruieren und voneinander
  subtrahieren, **nie** über `24*time.Hour`-Arithmetik. Nur so stimmen
  DST-Tage (23 h / 25 h) und Schaltjahre.

Timezone-Semantik (Präzedenzfall `widget_clock`, übernommen von
`widget_progress`): optionale Property `timezone` (IANA-Name), leerer Wert →
`time.Now().Location()` (Serverzeit, im Container typischerweise UTC),
ungültiger Wert → stiller Fallback auf Serverzeit. Alle Periodengrenzen werden
in **derselben** Location gebaut.

Bestehende `fill*Content`-Funktionen außer `fillProgressContent` rufen weiterhin
`time.Now()` direkt. Neue zeitabhängige Widgets nutzen den Seam.

---

## 4. Die `GetPropInt`-Falle

`GetPropInt` (`services/design.go:1005-1017`) dekodiert **nur** `float64` und
`string` — die Formen, die aus einer JSON-Design-Datei kommen. Ein `int` fällt
in den `default`-Zweig und wird stillschweigend verworfen; der Fallback greift.
Dasselbe gilt für `GetPropFloat` (`:990`) und `GetPropString` (`:977`).

In Tests deshalb immer:

```go
props := map[string]any{"barWidth": float64(20)}   // richtig
props := map[string]any{"barWidth": 20}            // FALSCH — Default greift
```

Ein Test mit blankem `int` ist grün, weil der Default zufällig zum erwarteten
Wert passt, und behauptet damit nichts. Das war ein echter Bug in den F7-Tests.

Konsequenz für Punkt 2 (`getDefaultProperties`): numerische Defaults bleiben
Zahlen, keine Strings, keine Booleans.

---

## 5. Testanforderungen

Datei: `server/internal/services/widget_<name>_test.go`.

**`widget_registration_test.go` ist für das eigene Widget nicht anzufassen.**
Die Registrierungstests iterieren über die aus dem `switch` abgeleitete
Typenliste (`for _, widgetType := range dispatchWidgetTypes(t)`, `:207`) und
decken jedes neue Widget ab, sobald sein `case` in `WidgetTextContent` steht —
ohne Eintrag in irgendeiner Testliste. Einzige Ausnahme ist der Parity-Test
(unten): der ist pro Widget handgeschrieben, weil die erwartete Vorkommenszahl
in `widgets.js` widget-spezifisch ist. Wer eine Zahl in
`widget_registration_test.go` anpassen zu müssen glaubt, hat vermutlich ein
anderes Problem — siehe die Untergrenze `len(types) < 8` in Abschnitt 1.

**Unit-Tests.** Tabellentest über die Widget-Logik, mit fixierter Uhr:

```go
func new<Name>Service(frozen time.Time) *PreviewService {
    s := &PreviewService{}
    s.now = func() time.Time { return frozen }
    return s
}
```

(Muster: `widget_progress_test.go:14-18`.) Abzudecken: Defaults, `nil`-Props,
ungültige Werte, Clamping, Grenzfälle der Domäne, Zerowert-Service ohne Panic.

**Glyphen-Test — für jedes Widget, nicht nur für Balken.** Die Prüfung ist die
Grenze aus Abschnitt 2: **jede Rune < U+0100**. `TestProgressASCIIOnly`
(`widget_progress_test.go:213`) prüft bytweise `content[i] >= 0x80` und ist
damit **strenger als nötig** — das ging nur durch, weil AC12 ausschließlich die
reinen Balken-Layouts abdeckt. Ein deutschsprachiges Widget kann das nicht
kopieren, es würde an jedem Umlaut scheitern. Stattdessen über Runen prüfen:

```go
for _, r := range content {
    if r >= 0x100 {
        t.Errorf("rune %U outside Latin-1 in %q", r, content)
    }
}
```

Diese Form fängt En-Dash, Em-Dash, Ellipse und typografische Anführungszeichen
und lässt `März` durch. Enthält das Widget zusätzlich ein reines
ASCII-Konstrukt (Balken, Tabelle, Spaltenraster), dafür separat auf `>= 0x80`
prüfen.

Zonen-abhängige Tests laden über einen Helper, der bei fehlender tzdata
`t.Skip` macht (`mustLoadLocation`, `widget_progress_test.go:20-27`) — sonst
ist der Test auf minimalen Hosts rot statt ehrlich übersprungen.

**Parity-Test.** `WidgetTextContent(typ, props)` byteidentisch zum direkten
`fill*Content(props)`, plus die statischen JS-Prüfungen aus Abschnitt 2.
Muster: `TestProgressCanvasPanelParity`.

**Golden-Design.** Nur sinnvoll, wenn die Zeitquelle des Widgets injizierbar
ist — sonst wird `TestGoldenRender` innerhalb einer Stunde rot und blockiert
alle. Der Clock-Seam ist dafür Voraussetzung, nicht Kür. Netzgestützte Widgets
gehören ohne einen ebenso injizierten Datenseam **nicht** in den Golden-Harness.

Schritte:

1. `server/internal/services/testdata/designs/<name>.json` anlegen. Jedes
   Textelement pinnt `fontFamily: "testfont.ttf"` (Pflicht, siehe
   `setupGoldenServicesVariant`, `golden_test.go:83-88`); `timezone` explizit
   setzen.
2. Design-Namen in `goldenDesigns` (`golden_test.go:34`) eintragen.
3. Bei IANA-Timezone-Property zusätzlich in `goldenTZDesigns`
   (`golden_test.go:56`) eintragen — sonst schlägt der Test auf Hosts ohne
   tzdata mit einem irreführenden Pixel-Diff fehl statt zu skippen.
4. **Design-Namen in `ditherDesigns` (`golden_test.go:344`) eintragen.** Das ist
   eine **zweite, eigene Liste** neben `goldenDesigns` — ein Name in
   `goldenDesigns` landet dort nicht automatisch. Wird der Schritt vergessen,
   ist die Folge **still**: die Palette-Assertion läuft für dieses Design
   einfach nie, alle Tests bleiben grün, und ein Widget mit Fremdfarbe im
   Output fällt erst auf dem echten Panel auf. Der einzige Schritt hier ohne
   rotes Warnsignal.

   `assertPaletteExactness` (`:261`) arbeitet generisch über
   `models.GetDisplayConfig(displayType).Colors` und prüft: Palettenlänge und
   -reihenfolge exakt wie im Treiber, keine Fremdfarbe im Output, mindestens
   zwei Palettenfarben genutzt. `goldenDisplays` (`:73`) enthält beide Profile —
   `DisplayWaveshare75V2` (S/W) und `DisplayWaveshare73E` (6 Farben); dort ist
   nichts zu registrieren, aber beide müssen grün sein.
5. Die Uhr ist bereits gepinnt: `newGoldenPreviewService` setzt `svc.now` auf
   `goldenNow` (`golden_test.go:129`), ein `FixedZone`-Instant
   `2026-07-20 12:00 +02:00`. Nichts zu tun, nur zur Kenntnis.
6. Golden-PNGs für **beide** Displays erzeugen und im **selben** Commit wie der
   Renderer-Code einchecken (Konvention `golden_test.go:26-30`):

   ```
   cd server && go test ./internal/services -run TestGoldenRender -update
   ```

   Danach prüfen, dass im Diff **nur** die neuen PNGs erscheinen. `-update`
   niemals laufen lassen, um einen roten Test zu beruhigen.

7. **Die neuen PNGs mit den Augen anschauen — Pflicht, nicht optional.**

   `-update` friert ein, was gerendert wurde, ohne zu bewerten, **ob** das
   Gerenderte vollständig ist. Der F4-Implementierer hatte sein erstes
   Textelement zu niedrig dimensioniert; die Ausgabe war mitten in einer Zeile
   abgeschnitten, und `-update` hätte genau diesen Zustand als „korrekte"
   Referenz festgeschrieben. Danach ist der Test dauerhaft grün und pinnt einen
   Darstellungsfehler.

   **Kein automatischer Check fängt das.** Der Golden-Vergleich prüft
   Pixelgleichheit gegen die selbst erzeugte Datei, die Palettenprüfung nur die
   Farbtabelle — beide sind mit einem abgeschnittenen Text genauso zufrieden wie
   mit einem vollständigen. Nur der Blick auf das PNG entscheidet das.

   Konkret prüfen: keine Zeile abgeschnitten (oben/unten/rechts), kein
   ungewollter Umbruch, kein Tofu bei `ä`/`ö`/`ü`/`ß`, Elementhöhe trägt alle
   Zeilen des größten Layouts (z. B. `count = 3`), und der Text steht innerhalb
   seines Rahmens statt an ihm zu kleben.

**`-count=1` ist Pflicht.**

```
cd server && gofmt -l . && go vet ./... && go test -count=1 ./...
```

Go cached Testergebnisse pro Paket. Da die Registrierungstests **Dateien
außerhalb des Pakets** lesen (`static/js/*.js`, `templates/designer.html`),
merkt der Cache eine reine Frontend-Änderung nicht: ein grüner Lauf aus dem
Cache beweist nichts.

---

## 6. Netzgestützte Widgets

Für Luftqualität, Strommix und ÖPNV gilt: die Infrastruktur existiert, sie
wird **wiederverwendet**, nicht neu gebaut.

> **Feiertage gehören ausdrücklich nicht in diese Liste.** F4 hat sich bewusst
> gegen eine API und für lokale Berechnung entschieden (Osterformel +
> Regeltabelle, `widget_holidays.go`): die Domäne ist reine Arithmetik über
> `(Jahr, Bundesland)`, ein Netzaufruf würde ihr nur Latenz, einen Ausfallmodus
> und Nicht-Determinismus hinzufügen. Kein Cache, kein `failCache`, kein
> `safefetch.go`. Prüfe bei jedem neuen Widget zuerst, ob es überhaupt eine
> externe Quelle **braucht** — Abschnitt 6 gilt nur, wenn ja.

**Fetch + persistenter Cache — `services/weather.go`.**
`WeatherService.FetchForLocation` (`:248-383`) ist das Referenzmuster:

1. Positiv-Cache prüfen, frischer Eintrag gewinnt sofort (TTL 30 min, `:255`).
2. Negativ-Cache prüfen, Treffer → direkt in den Stale-oder-Fehler-Pfad
   (`:262-264`).
3. Fetch mit `s.client` (`http.Client{Timeout: 10 * time.Second}`).
4. Jeder Fehlerzweig (Transport, Non-200, Body-Read, Parse) geht über
   `failFetch` (`:393`) → `markFailure` + `returnCachedOrError` (`:399`), d. h.
   veraltete Daten sind besser als leerer Content.
5. Erfolg: Cache schreiben, `evictOldestCache` (Bound: `maxWeatherCacheEntries
   = 50`), `persistCacheLocked` nach `data/cache/weather.json`, dann
   `failCache.markSuccess(negKey)`.

Der persistente Cache (`loadPersistedCache` `:148`, `persistCacheLocked` `:174`)
überlebt Neustarts — auf dem Pi ist das der Unterschied zwischen „Panel zeigt
nach dem Boot Daten" und „Panel zeigt No data, bis der erste Fetch durch ist".

**Negativ-Cache — `services/negcache.go`.** Paketweite Instanz `failCache`
(`:50`), TTL 2 min (`negativeCacheTTL`, `:22`). API:
`blocked(key)`, `blockedFallback(key)`, `markFailure(key)`,
`markFailureValue(key, fallback)`, `markSuccess(key)`. Key-Konvention:
`"url:<url>"` für Quellen über `defaultHTTPClient`, `"weather:<lat,lon>"` für
open-meteo. Neue Quellen wählen ein eigenes, stabiles Präfix.

Wichtig: gecacht wird nur der **Fehlerzustand**, nie Content. Ein Cache-Treffer
muss byteidentisch dieselbe Ausgabe liefern wie der Live-Fehlerfall — bei
variierendem Fehlerwert dafür `markFailureValue` nutzen (Muster:
`fetchCustomAPI`, `helpers.go:104-130`). Tests setzen `failCache.reset()` in
`t.Cleanup`.

Einfache Quellen ohne eigenen Service nutzen `defaultHTTPClient`
(`helpers.go:16`) plus `readLimitedBody(r, 1<<20)` (`:19`) — Body-Limit ist
Pflicht, kein `io.ReadAll` ohne Cap. Muster: `fetchRSSFeed` (`:45-94`).

**SSRF-Härtung — `services/safefetch.go`.** Für jedes Widget, dessen Ziel-URL
aus Konfiguration oder Nutzereingabe stammt.
`safeFetchAllowlisted(ctx, c, targetURL, bearer, limit)` (`:127`) mit einem
`allowlistClient` aus `newAllowlistClient(host, port)` (`:66`) liefert:
Scheme-Beschränkung auf http/https, Ziel-`host:port` gegen den
Allowlist-Eintrag mit Default-Port-Normalisierung (`hostPort`, `:106`),
`pinnedDialContext` (`:93`) als zweite, unabhängige Kontrolle gegen DNS
Rebinding, deaktivierte Redirects, Timeout und `io.LimitReader`-Cap. Alle
Fehler sind das generische `errFetchUnavailable` — kein Host, keine IP, kein
Token leakt in die UI.

Fixe Provider-Endpunkte (open-meteo-Muster) dürfen `defaultHTTPClient`
verwenden. Sobald der Nutzer die URL bestimmt, ist `safefetch.go` die richtige
Wahl. Secrets bleiben serverseitig in `data/`, nie als Widget-Property im
Design-JSON (Präzedenzfall `widget_hass`, `element-factory.js:271-274`).

Für Render-Determinismus in Tests: die Netzpfade werden im Testcode über
`defaultHTTPClient.Transport` bzw. `WeatherService.client` ausgetauscht
(`offline_render_test.go:131-133`). Neue Fetch-Sites, die daran vorbeigehen,
machen den Offline-Test wirkungslos.

---

## 7. Monochrom-Lesbarkeit

`epd7in5_V2` (`DisplayWaveshare75V2`) ist reines Schwarz/Weiß. `epd7in3e`
(`DisplayWaveshare73E`) hat sechs Farben.

Ein Widget, das Information über Farbe kodiert — Luftqualitätsstufe, Strommix,
Verspätungsstatus — verliert diese Information auf dem S/W-Panel vollständig.
Jede farbkodierte Aussage braucht deshalb eine zweite, farbunabhängige
Repräsentation: Text („Gut" / „Mäßig" / „Schlecht"), Balkenlänge,
ASCII-Muster oder eine Zahl.

Prüfung: Golden-PNG für `waveshare_7in5_v2` öffnen und lesen, ohne die
Farbversion daneben zu legen. Was dort nicht mehr unterscheidbar ist, ist auf
dem Panel verloren.

---

## 8. tzdata — ehrlicher Stand

Verifizierte Fakten:

- `server/Dockerfile:31-32` kopiert `zoneinfo.zip` in das Scratch-Image und
  setzt `ENV ZONEINFO=/usr/local/go/lib/time/zoneinfo.zip`.
- Raspberry Pi OS hat `/usr/share/zoneinfo`.
- `widget_clock` hat dieselbe Abhängigkeit seit jeher.

Es funktioniert also heute. Das Risiko liegt im Fehlerfall: schlägt
`time.LoadLocation` fehl, fällt das Widget **still** auf Serverzeit zurück
(`widget_progress.go:55-59`) — kein Fehler, kein Log, nur eine falsche Uhrzeit
auf dem Panel. Jedes Widget mit `timezone`-Property erbt dieses Verhalten.

Ein `import _ "time/tzdata"` würde die Datenbank in das Binary einbetten und
einen verlorenen `ZONEINFO`-ENV überleben (Kosten: ca. 450 KB Binärgröße).
**Das ist nicht umgesetzt** und ein offener Folge-Task, kein Teil dieses
Rezepts. Bis dahin: bei zonenabhängigen Widgets den Golden-Design-Namen in
`goldenTZDesigns` eintragen, damit ein Host ohne tzdata skippt statt einen
unerklärlichen Pixel-Diff zu melden.

---

## Checkliste

```
Implementierung
[ ] services/widget_<name>.go mit fill<Name>Content angelegt
[ ] case "widget_<name>" in WidgetTextContent (preview.go)
[ ] Uhr (falls zeitabhängig) einmal über nowOrDefault() in lokale Variable
[ ] Zeitgrenzen via time.Date(..., loc), kein 24*time.Hour
[ ] timezone-Property: leerer Wert = Serverzeit, ungültig = stiller Fallback
[ ] jede Rune der Ausgabe < U+0100 (Umlaute ja, En-/Em-Dash, Ellipse,
    typografische Anfuehrungszeichen, Blockzeichen nein)
[ ] kein eigener drawElement-Zweig

Registrierung (Reihenfolge)
[ ] 7  preview.go widgetDefaultFontSizes
[ ] 8  widgets/layouts.go allLayouts (nur wenn props["layout"] gelesen wird)
[ ] 8  widgets/layouts.go allPlaceholders (nur wenn props["customTemplate"]
       gelesen wird — unabhaengig von allLayouts)
[ ] 1  element-factory.js defaultSizes
[ ] 2  element-factory.js getDefaultProperties (Zahlen bleiben Zahlen)
[ ] 3  properties-panel.js getWidgetPropertyDefs
[ ] 4  widgets.js getPreviewContent -> Passthrough-Block
[ ] 5a widgets.js getDefaultLayout (nur wenn props["layout"] gelesen wird)
[ ] 5b widgets.js getPreviewFontSize (gleicher Wert wie Punkt 7)
[ ] 6  designer.html Widget-Palette

Netz (nur netzgestützte Widgets)
[ ] Positiv-Cache mit TTL, persistiert nach data/cache/
[ ] failCache: blocked/markFailure/markSuccess, eigenes Key-Präfix
[ ] Fehlerpfad liefert stale Daten statt leerem Content
[ ] Body-Limit gesetzt (readLimitedBody / io.LimitReader)
[ ] nutzerbestimmte URL -> safeFetchAllowlisted, nicht defaultHTTPClient
[ ] keine Secrets als Widget-Property im Design-JSON

Tests
[ ] Unit-Tabellentest mit fixierter Uhr (PreviewService{} + s.now)
[ ] Zerowert-Service paniked nicht
[ ] alle Property-Zahlen in Tests als float64(...), nie als int
[ ] Glyphen-Test: jede Rune < 0x100 (nicht >= 0x80 kopieren)
[ ] widget_registration_test.go NICHT angefasst (deckt neue Widgets selbst ab)
[ ] Parity-Test: WidgetTextContent == fill<Name>Content
[ ] Parity-Test: Typ kommt in widgets.js exakt N-mal vor (N = Rollen)
[ ] testdata/designs/<name>.json mit fontFamily "testfont.ttf"
[ ] Name in goldenDesigns
[ ] Name in goldenTZDesigns (falls timezone-Property)
[ ] Name in ditherDesigns — SEPARATE Liste, Vergessen faellt still aus
[ ] Golden-PNGs für BEIDE Displays im selben Commit
[ ] bestehende Golden-PNGs unverändert (git diff --stat prüfen)
[ ] neue PNGs ANGESCHAUT: nichts abgeschnitten/umgebrochen — -update friert
    auch eine truncated Ausgabe klaglos ein, kein Test merkt es

Lesbarkeit
[ ] farbkodierte Information hat eine S/W-taugliche Zweitform
[ ] Golden-PNG waveshare_7in5_v2 einzeln gelesen und verständlich

Verifikation
[ ] cd server && gofmt -l . && go vet ./... && go test -count=1 ./...
[ ] TestWidgetRegistrationCompleteness grün (nicht SKIP)
[ ] TestWidgetDefaultFontSizesMatchFrontend grün (nicht SKIP)
[ ] manuell: Widget aus Palette ziehen, Canvas-Text == GET /preview
```
