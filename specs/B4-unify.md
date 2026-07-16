# B4: Widget-Content-Vereinheitlichung (eine Content-Quelle: Go-Renderer)

## Ziel
Nach diesem Task ist der Text-`{content}` jedes Daten-Widgets (weather, forecast,
calendar, news, system, custom) auf **genau einer** Quelle definiert — den
`fill*Content`-Methoden in `server/internal/services/preview.go` —, sodass der
Canvas-Editor exakt denselben String anzeigt, den das Panel rendert; die
zweite (client- + `widgets/`-Package-)Implementierung existiert danach nicht
mehr. Vorher weichen Editor und Panel bei ≥ 9 Prop-Fällen ab (Kalender-`title`,
`layout`, `maxEvents`; News-`title`/`showDescription`/`layout`;
System-`showLabels`/`layout`; Custom `url`/`jsonPath`) und der Editor erfindet
Fake-Daten für weather/forecast/calendar/news/system/custom.

## Kontext

Diese Aufgabe setzt die Kilian-Entscheidung aus `PROGRESS.md` §28 um (Go-Renderer
= einzige Content-Quelle, Canvas rendert Server-`{content}` verbatim; clock/timer
bleiben client-live; Golden/Offline-Tests werden bewusst behandelt). Die
Architektur-Entscheidung selbst wird NICHT neu verhandelt. Sie ist zugleich die
in `specs/E4a-widget-deadcode-cleanup.md:34` und `specs/E5.5-offline-hardening.md:90`
ausdrücklich als „separate, größere Konsolidierung" vertagte Arbeit.

### Verifizierter Ist-Zustand — drei Render-Pfade (Zeilen geprüft)

**P = Panel/Go (AUTORITATIV, einziger Pfad zum E-Ink-Panel).**
`server/internal/services/preview.go`:
- `drawElement` (`:334-378`) dispatcht per `switch elemType`: ruft für jeden Typ
  die passende `fill*Content(props)`-Methode und übergibt das Ergebnis an
  `renderTextV`.
- Die `fill*Content`-Methoden sind BEREITS reine Content-String-Builder mit
  Signatur `func (s *PreviewService) fillXContent(props map[string]any) string`:
  `fillTextContent :441`, `fillClockContent :448`, `fillWeatherContent :516`,
  `fillForecastContent :565`, `fillCalendarContent :606`, `fillNewsContent :693`,
  `fillTimerContent :746`, `fillCustomContent :819`, `fillSystemContent :830`.
  **Sie nehmen KEINEN Zeichen-Kontext (kein image/font) entgegen** — die
  Trennung Content vs. Zeichnen existiert schon in `drawElement`. Ein „Extrahieren"
  im Sinne von Auseinanderziehen ist damit NICHT nötig; es fehlt nur ein
  gemeinsamer, exportierter Dispatch-Einstieg (siehe Ziel-Architektur B4a).
- Netz-Fetches der P-Fills laufen über die konsolidierte Policy:
  `defaultHTTPClient` (10 s Timeout, `helpers.go:16`), 1 MB Body-Limit
  (`readLimitedBody`, `helpers.go:19`), Negative-Cache `failCache`
  (`negcache.go`, `negativeCacheTTL = 2*time.Minute :22`), `webcal://`→`https://`
  Rewrite (`preview.go:617-619`). weather/forecast nutzen zusätzlich den
  WeatherService-Cache (`weather.go:221 FetchForLocation`).

**E = `/api/widgets/*` (Editor-Preview-API, DUPLIKAT von P, verschluckt Props).**
`server/internal/handlers/widgets.go` + `server/internal/services/widgets/*.go`:
- Routen `main.go:237-243`.
- `Weather :32` / `Forecast :51` liefern ROHDATEN (Wetter-JSON bzw. `{daily:[]}`),
  der Canvas formatiert client-seitig.
- `Calendar :85` ruft `widgets.CalendarWidget.GetContent` (`widgets/calendar.go:25`),
  hardcodet aber `maxEvents=10`, forwardet `title`/`layout` NIE, kein Negative-Cache,
  eigener `http.Client` (`calendar.go:20`); `layout` wird gar nicht unterstützt,
  dafür `showTime` (das P wiederum ignoriert).
- `News :113` ruft `widgets.NewsWidget.GetContent` (`widgets/news.go:39`), forwardet
  nur `feedUrl`+`maxItems`, verschluckt `title`/`showDescription`/`layout`.
- `System :139` ruft `widgets.SystemWidget.GetContent` (`widgets/system.go:18`),
  hardcodet `showLabels=true`, kein `layout`/`custom`.
- `Custom :165` ruft `widgets.CustomWidget.GetContent` (`widgets/custom.go:26`) —
  **toter Pfad**: der Canvas ruft ihn nie (siehe C).
- `Layouts :151` ruft `widgets.GetLayouts`/`widgets.Placeholders`
  (`widgets/layouts.go`) — **das ist KEIN Content-Duplikat**, sondern
  UI-Metadaten (Layout-Dropdown + Placeholder-Chips), die der Canvas braucht
  (`properties-panel.js:695 fetch('/api/widget_layouts/' + type)`). MUSS bleiben.

**C = Canvas (Editor-Preview, Fabric.js).** `server/static/js/widgets.js`:
- `fetchWidgetData(type, props)` (`:19`) baut per `switch` eine GET-URL:
  weather `:30`, forecast `:33`, calendar `:37` (`return null` wenn kein icalUrl),
  news `:41` (`return null` wenn kein feedUrl), system `:44`; clock `:47` liefert
  `{time}` lokal, timer `:49` liefert `{targetDate}` lokal; **custom fehlt im
  switch → `default: return null` :50-51**.
- `getPreviewContent(type, props, liveData)` (`:67`) formatiert:
  `buildWeatherContent :143` und `buildForecastContent :183` formatieren client-seitig
  UND ERFINDEN FAKE-DATEN offline (`:147 '22°C Sunny'`, `:190-193 '12-20°C'`);
  `buildCalendarContent :228` / `buildNewsContent :236` / `buildSystemContent :293`
  geben `liveData.content` durch, ERFINDEN aber bei `!data.content` Fake
  (`:230`, `:238`, `:295`); custom `:87` gibt hartes Fake `'API: 42'`;
  `buildClockContent :107` und `buildTimerContent :244` rechnen client-live.
- `_germanWeekdayShortByIndex`/`ByName` (`:12-16`) werden NUR von
  `buildForecastContent` benutzt.

### Prop-Matrix je Widget (aus dem Code, Quelle der Akzeptanzkriterien)

Settable in `properties-panel.js` `getWidgetPropertyDefs` (`:769-820`) +
`element-factory.js` `getDefaultProperties` (`:188-263`); „P liest" = welche
Keys `fill*Content` tatsächlich auswertet.

| Widget | Props aus Panel-UI | P (`fill*Content`) liest | Divergenz heute |
|---|---|---|---|
| weather | latitude, longitude, layout(+customTemplate), fontSize | latitude, longitude, layout(Fallback `style`→compact), customTemplate (`:516-543`) | Canvas erfindet `'22°C Sunny'` offline |
| forecast | latitude, longitude, days, layout(+customTemplate), fontSize | latitude, longitude, days(3), layout(vertical), customTemplate (`:565-604`) | Canvas erfindet `'12-20°C'` offline |
| calendar | icalUrl, maxEvents, showTime, daysAhead, title, layout(+customTemplate), fontSize | icalUrl, maxEvents(5), layout(list; compact cappt auf 3), title, daysAhead(7) (`:606-691`) | Canvas: title/layout/maxEvents wirkungslos, Fake bei leer; `showTime` liest P NICHT (Orphan, s.u.) |
| news | feedUrl, maxItems, showDescription, title, layout(+customTemplate), fontSize | feedUrl, maxItems(5), title, layout(headlines), showDescription (`:693-744`) | Canvas: title/showDescription/layout wirkungslos, Fake bei leer |
| system | showLabels, layout(+customTemplate), fontSize | layout(vertical/horizontal/custom), showLabels (`:830-842` via `fetchSystemInfo helpers.go:186`) | Canvas: showLabels(hardcoded true über E)/compact/custom wirkungslos, Fake bei leer |
| custom | url, jsonPath, prefix, suffix, fontSize | url, prefix, suffix, jsonPath (`:819-828` via `fetchCustomAPI helpers.go:99`) | Canvas fetcht NIE, zeigt Fake `'API: 42'` |

Gemeinsame Nicht-Content-Props (color, textAlign, verticalAlign, fontFamily,
fontSize) betreffen das ZEICHNEN, nicht den Content-String; sie sind NICHT Teil
dieser Aufgabe (siehe Non-Goals). `showTime` (calendar) ist ein Orphan: das Panel
wertet ihn heute schon nicht aus — nach der Vereinheitlichung ignoriert ihn auch
der Canvas (Parität durch beidseitiges Ignorieren; funktional tot, wird in B4
NICHT repariert).

### Auth-/Guard-Semantik (unverändert zu übernehmen)
`server/internal/middleware/auth.go`: neue API-Route ist nicht in `publicRoutes`
(`:36-44`) → sitzungspflichtig, sobald ein Passwort gesetzt ist (Editor läuft
authentifiziert). Ein POST ist `isMutating` (`:143-149`) → CSRF-Origin-Check
(`:123-125`), same-origin passiert — exakt wie `POST /api/preview_live`
(`main.go:221`). Kein neues Auth-Verhalten.

### Muster, denen die Umsetzung folgen MUSS
- Kein Frontend-Build-Step: `server/static/*` handgepflegt, via `go:embed`
  (CLAUDE.md) — `widgets.js` direkt editieren.
- Go: gofmt, error returns, `log/slog`, kein panic.
- Fetch-Determinismus in Tests über die vorhandenen Stubs
  (`offline_render_test.go`: `installRenderTransport`, `hostStubTransport`,
  `countingTransport`, `canonicalOpenMeteoJSON`).

## Ziel-Architektur

### Endpoint-Form: `POST /api/widget_content` (begründet)
Body: `{"type":"widget_calendar","properties":{ ... voller Prop-Map ... }}` →
`200 {"content":"..."}`.

Begründung (gegen die GET-Alternative abgewogen):
1. **Voller Prop-Map verbatim.** Die Ursache der B4-Drift ist, dass die
   GET-Handler in `widgets.go` pro Typ nur ausgewählte Query-Params auf `props`
   mappen und den Rest verschlucken. Ein JSON-Body übergibt GENAU die Map, die
   `fill*Content` konsumiert — kein Per-Typ-Query-Mapping, das die Drop-Klasse
   strukturell wiederbelebt.
2. **Präzedenz.** `POST /api/preview_live` (`handlers/preview.go:64`) sendet
   bereits ein strukturiertes JSON-Objekt an den Renderer; gleiche Guard-/CSRF-/
   same-origin-Eigenschaften. Der Canvas kennt das Muster.
3. **Minimaler Canvas-Diff.** `fetchWidgetData` hat `props` schon zur Hand; es
   ändert sich nur der Fetch (URL-Bau → JSON-Body).
4. **Typ-JSON-Treue.** Zahlen kommen als `float64`, Bools als `bool` an — genau
   wie `fill*Content` sie beim Design-Laden bekommt; `GetPropInt/Bool/String`
   (`design.go:977-1035`) decken diese Coercion bereits ab.

GET `?props=<urlencoded-json>` wäre möglich (kein CSRF, HTTP-Caching), verliert
aber gegen die Body-Treue und die Präzedenz; der Canvas hat mit `_dataCache`
(60 s TTL, `widgets.js:7`) ohnehin eine eigene Cache-Schicht. → **POST.**

Die Layout-Metadaten bleiben getrennt bei `GET /api/widget_layouts/{type}`
(`main.go:243`) — das ist keine Content-Quelle.

### File-by-file

**`server/internal/services/preview.go` (neu: gemeinsamer Dispatch)**
- Neue exportierte Methode als EINZIGER Dispatch-Einstieg:
  ```go
  // WidgetTextContent returns the text content for a text/widget element type
  // using the same fill*Content logic drawElement draws. ok is false for types
  // without server-side text content (image, shape, unknown).
  func (s *PreviewService) WidgetTextContent(elemType string, props map[string]any) (content string, ok bool)
  ```
  `switch`-Abdeckung 1:1 wie `drawElement`: `text/i-text/textbox`→`fillTextContent`;
  `widget_clock`→`fillClockContent`; `widget_weather`→`fillWeatherContent`;
  `widget_forecast`→`fillForecastContent`; `widget_calendar`→`fillCalendarContent`;
  `widget_news`→`fillNewsContent`; `widget_timer`→`fillTimerContent`;
  `widget_custom`→`fillCustomContent`; `widget_system`→`fillSystemContent`;
  `default`→`("", false)`.
- `drawElement` (`:334-378`) wird umgebaut, sodass es für die Text-/Widget-Fälle
  `content, _ := s.WidgetTextContent(elemType, props)` aufruft statt die
  `fill*Content` einzeln inline — damit teilen Panel UND Endpoint garantiert
  denselben Dispatch (kein Logik-Fork). `image`/`shape` bleiben unverändert.
  Diese Umstellung ist rein mechanisch und MUSS byte-identische Panel-Ausgabe
  liefern (siehe AC-Golden).

**`server/internal/handlers/widgets.go` (Umbau)**
- `WidgetHandler` hält künftig `*services.PreviewService` (für `WidgetTextContent`)
  statt `*services.WeatherService` + die vier `widgets.*Widget`-Felder.
  `NewWidgetHandler(preview *services.PreviewService)`.
- ENTFERNEN: `Weather`, `Forecast`, `Calendar`, `News`, `System`, `Custom`.
- NEU: `Content(w, r)` — dekodiert `{type, properties}`, ruft
  `h.preview.WidgetTextContent(type, properties)`, bei `ok==false` → 400
  „unsupported widget type", sonst `200 {"content": ...}` (via vorhandenes
  `jsonResponse`).
- BEHALTEN: `Layouts` — nutzt weiterhin `widgets.GetLayouts` +
  `widgets.Placeholders` (state-los).

**`server/main.go` (Routen)**
- ENTFERNEN `:237-242` (weather/forecast/calendar/news/system/custom).
- NEU: `mux.HandleFunc("POST /api/widget_content", widgetH.Content)`.
- BEHALTEN `:243` `GET /api/widget_layouts/{type}`.
- `:133` `widgetH := handlers.NewWidgetHandler(previewSvc)` (previewSvc existiert
  ab `:100`).

**`server/internal/services/widgets/` (Löschung — Content-Duplikat)**
- LÖSCHEN: `calendar.go`, `news.go`, `system.go`, `custom.go`, `helpers.go`
  (`helpers.go` `getString/getInt/getBool` werden nur von den vier genutzt).
- BEHALTEN: `layouts.go` (`GetLayouts`/`Placeholders` — vom Layout-Dropdown
  gebraucht; nutzt `helpers.go` NICHT, kompiliert eigenständig).

**`server/static/js/widgets.js` (Passthrough)**
- `fetchWidgetData`: für weather/forecast/calendar/news/system/custom EINEN
  gemeinsamen Zweig, der `POST /api/widget_content` mit
  `{type, properties: props}` sendet und die `{content}`-Antwort (unter dem
  bestehenden `_dataCache`-Mechanismus) zurückgibt. Die `return null`-Guards für
  fehlende `icalUrl`/`feedUrl` (`:36`, `:40`) ENTFERNEN, damit der Editor die
  Server-Strings („No calendar URL"/„No feed URL") identisch zum Panel zeigt.
  clock (`:47`) und timer (`:49`) bleiben lokal.
- `getPreviewContent`: die sechs Daten-Widgets geben
  `(liveData && liveData.content) ? liveData.content : <neutraler Platzhalter>`
  zurück — der Platzhalter ist NIE erfundene Daten (z. B. Widget-Typ-Label oder
  „…"), nur für den seltenen Fall, dass der POST an den eigenen Server scheitert
  (der Server liefert sonst immer einen Content-String, auch „No data").
  clock/timer rufen weiter `buildClockContent`/`buildTimerContent`.
- ENTFERNEN: `buildWeatherContent`, `buildForecastContent`, `buildCalendarContent`,
  `buildNewsContent`, `buildSystemContent`, die custom-Fake-Zeile (`:87`) und die
  nun toten `_germanWeekdayShortByIndex`/`ByName` (`:12-16`).
- BEHALTEN: `buildClockContent`, `buildTimerContent`, `applyTimePlaceholders`.
- Live-Preview-Last drosseln: die Property-Change→`updatePreview`-Kette in
  `properties-panel.js` (`:611`, `:662-681`) für Daten-Widgets debouncen
  (≈ 300–500 ms), damit `customTemplate`-Tippen (`input`-Event) nicht pro
  Tastendruck einen POST auslöst; `_dataCache` (60 s) bleibt.

**Test-Referenzen auf die gelöschten Routen (in B4c mitziehen)**
- `server/main_test.go:120` + `:147` (`GET /api/widgets/system`).
- `server/internal/middleware/auth_test.go:88` (`GET /api/widgets/system`).
- `scripts/test-e2e.sh:187-188` (`curl .../api/widgets/system`).
  Alle drei auf `POST /api/widget_content` (bzw. den Layout-Endpoint) umstellen
  oder entfernen.

## Akzeptanzkriterien (hart, überprüfbar)

- **AC1 — Eine Quelle, ein Dispatch.** `drawElement` und der Endpoint erhalten
  ihren Content-String über `PreviewService.WidgetTextContent`; es existiert
  keine zweite Content-Formatierung mehr für die sechs Daten-Widgets (grep:
  `buildWeatherContent|buildForecastContent|buildCalendarContent|buildNewsContent|buildSystemContent`
  liefert 0 Treffer in `server/static/js/`; `server/internal/services/widgets/`
  enthält nur noch `layouts.go`).
- **AC2 — Content-Gleichheit Canvas↔Panel (String-Level, tabellengetrieben).**
  Ein Go-Test (Handler-Ebene) POSTet je Config `{type, properties}` an
  `/api/widget_content` und assertiert, dass der zurückgegebene `content` EXAKT
  `previewSvc.WidgetTextContent(type, properties)` (Direktaufruf, gleiche
  Service-Instanz) entspricht — für alle Configs. Deterministisch via
  `installRenderTransport` + `hostStubTransport`/`canonicalOpenMeteoJSON`
  (system liest `/proc` → Gleichheit der zwei Aufrufwege prüfen, nicht gegen
  Fixstring). Die Config-Tabelle MUSS die Regressionsfälle aus AC3 enthalten.
- **AC3 — Regressionsfälle wirken im Editor identisch zum Panel** (belegt in AC2
  auf String-Ebene + in L2 im Browser):
  - calendar `title` erscheint im Canvas; `layout∈{list,agenda,compact}` und
    `maxEvents` wirken (heute wirkungslos, `widgets.js:232`/`widgets.go:99-104`).
  - news `title`, `showDescription`, `layout∈{headlines,summary,single}` wirken.
  - system `showLabels=false` und `layout∈{vertical,horizontal,custom}` wirken
    (statt hardcoded `true`/kein Layout).
  - custom `url`/`jsonPath` werden im Canvas TATSÄCHLICH gefetcht und formatiert
    (statt Fake `'API: 42'`); `prefix`/`suffix` wirken.
  - weather/forecast erfinden im Canvas KEINE Daten mehr: bei Fetch-Fehler zeigt
    der Canvas den Panel-String („No data"/„No forecast data"), nicht
    „22°C Sunny"/„12-20°C".
- **AC4 — Volle Prop-Parität je Widget.** Für jeden in der Prop-Matrix (Kontext)
  gelisteten, von P gelesenen Key gilt: gleicher Wert → gleicher `{content}` in
  Editor und Panel. `showTime` (calendar) ist als bekannter Orphan dokumentiert
  (beide Seiten ignorieren ihn; kein Fix in B4).
- **AC5 — Golden byte-identisch (kein Panel-Drift).** `TestGoldenRender` läuft
  OHNE `-update` grün. Begründung/Beleg: die Golden-Designs
  (`testdata/designs/{basic,gradient,rotation,calibration}.json`) enthalten
  NUR `text`/`shape`/`image`, KEIN Widget (grep-verifiziert) → die
  `drawElement`-Umstellung auf `WidgetTextContent` ändert keinen Pixel. Ein
  `-update`-Re-Baseline ist in B4 VERBOTEN; würde er nötig, wäre unbeabsichtigt
  `fill*Content`-Logik verändert worden (Panel ist autoritativ und bleibt
  unverändert).
- **AC6 — Offline-Härtung unverändert grün.** `offline_render_test.go` besteht
  UNVERÄNDERT: `TestOfflineRenderNegativeCache` (PNG render1==render2
  byte-identisch, Attempt-Zählung 4/4/8/12), `TestOfflineRenderStaleWeatherAfterRestart`
  (`fillWeatherContent == "17°C Teilweise bewölkt"`),
  `TestCustomAPINegativeCacheHitPreservesHTTPStatus`. Die Content-Fills und der
  Negative-Cache bleiben unangetastet.
- **AC7 — clock/timer unberührt (akzeptierte „Drift-Insel").** `buildClockContent`,
  `buildTimerContent`, `applyTimePlaceholders`, `startClockUpdates` (`widgets.js:382`)
  bleiben; clock/timer werden NICHT über `/api/widget_content` bezogen. Explizit
  dokumentiert als bewusste Ausnahme (Sekunden-/Minutentakt; ein statischer
  Server-Render würde einfrieren).
- **AC8 — SSRF: eine Fetch-Policy, keine neue Fläche.** Nach B4c existiert genau
  EIN Fetch-Pfad für calendar/news/custom (die P-Fills mit `defaultHTTPClient`
  10 s, 1 MB-Limit, `failCache`, `webcal`-Rewrite); die zweite Client-Familie in
  `widgets/*.go` ist gelöscht. Kein neuer SSRF-Code. Der EINZIGE neu
  canvas-erreichbare Fetch ist `custom` (heute Fake, nie gefetcht) und erbt
  `fetchCustomAPI` inkl. Negative-Cache; calendar/news/system fetchen schon heute
  aus dem Canvas (via `/api/widgets/*`) und sind daher nicht neu. Grep-Beleg:
  keine `http.Client`-Instanz mehr in `server/internal/services/widgets/`.
- **AC9 — Statik/Build/Tests grün.** `gofmt -l server/` leer, `go vet ./...`
  clean, `go build ./...` clean, `go test ./... -count=1` grün inkl. `-race` auf
  `services`/`handlers`; keine dangling references (grep auf entfernte Symbole).

## Non-Goals (ausdrücklich NICHT in B4)
- `verticalAlign`-Anker im Canvas (Canvas-LAYOUT-Fix, alle 8 Widgets, C ignoriert
  ihn heute) — **separater Folgetask B4-verticalAlign**.
- `fontFamily`-Drift (Canvas bietet Web-Fonts, die der Server nicht hat;
  `widgets.js:366`/`element-factory.js:149` „monospace" vs Panel Noto/DejaVu) —
  **separater Folgetask B4-fontFamily**.
- B2 Rundung/Stroke-Parität (Raster vs. Vektor) — **eigenes Epic B2**.
- Neues Home-Assistant-Widget — **B5**, baut AUF dieser Pipeline auf (dann
  `fillHassContent` + `WidgetTextContent`-Case + Canvas-Passthrough, kein neuer
  Endpoint).
- Kein Anfassen der Zeichen-/Quantisierungs-/Kalibrierungs-Logik, des
  WeatherService, der Auth-/CSRF-Regeln, des Layout-Endpoints
  `/api/widget_layouts/{type}`.
- `showTime` (calendar) wird NICHT funktional gemacht (Orphan, s. o.).

## Dekomposition (der Diff überschreitet ~400 Zeilen → drei geordnete Sub-Tasks)

Empfohlene, sicherste Reihenfolge: **B4a → B4b → B4c**. Jeder Schritt ist für
sich verifizierbar und revertierbar; alte und neue Pfade koexistieren, bis der
Canvas nachweislich nicht mehr vom alten Pfad abhängt.

### B4a — Server: gemeinsamer Dispatch + neuer Endpoint (additiv, 0 Verhaltensänderung)
**Scope:** `WidgetTextContent` in `preview.go` ergänzen; `drawElement` darauf
umstellen; `handlers/widgets.go` um `Content` erweitern; `NewWidgetHandler` auf
`*PreviewService` umstellen; Route `POST /api/widget_content` in `main.go`
ergänzen. Alte `/api/widgets/*`-Routen + `widgets/*.go` bleiben vorerst bestehen.
Content-Gleichheitstest (AC2) hinzufügen.
**Akzeptanz:** AC1 (Server-Teil: Dispatch existiert, `drawElement` nutzt ihn),
AC2, AC5 (Golden byte-identisch — beweist die `drawElement`-Umstellung ist
neutral), AC6, AC9. Alter E-Pfad weiter funktionsfähig (kein Canvas-Change).
**Warum zuerst:** rein additiv, reversibel, Panel- und Canvas-Verhalten
unverändert; liefert die Zielsenke, bevor irgendein Konsument migriert.

### B4b — Canvas: Passthrough + Formatter/Fake-Daten entfernen
**Scope:** `widgets.js` `fetchWidgetData`/`getPreviewContent` auf
`POST /api/widget_content` + verbatim-`{content}` umstellen; die fünf
`build*Content`-Formatter + custom-Fake + tote Weekday-Tabellen entfernen;
Guards für leere URLs entfernen; Debounce in `properties-panel.js`. clock/timer
unangetastet.
**Akzeptanz:** AC1 (Canvas-Teil: keine `build*Content` mehr), AC3, AC4, AC7,
belegt über L2 (Browser: Editor-Preview == Panel für die Regressions-Configs).
Der alte `/api/widgets/*`-Pfad ist danach vom Canvas UNGENUTZT, aber noch
vorhanden (Fallback bei Revert).
**Warum als zweites:** Verhaltensänderung nur im Editor; der Server-Zielpunkt aus
B4a steht schon; bei Problemen genügt ein `widgets.js`-Revert.

### B4c — Toten E-Pfad löschen + Test-Referenzen ziehen + Review
**Scope:** `widgets/{calendar,news,system,custom,helpers}.go` löschen (layouts.go
behalten); `Weather/Forecast/Calendar/News/System/Custom`-Handler + Felder aus
`widgets.go` entfernen; Routen `main.go:237-242` entfernen; `main_test.go:120/147`,
`auth_test.go:88`, `scripts/test-e2e.sh:187-188` umstellen/entfernen. Grep-Beweis
der Referenzfreiheit.
**Akzeptanz:** AC1 (vollständig), AC8, AC9; volle Suite grün inkl. Golden ohne
`-update`; L5-Review bestätigt Referenzfreiheit + unveränderten Panel-/
Layout-/Auth-Pfad.
**Warum zuletzt:** Löschen erst, wenn kein Konsument mehr am alten Pfad hängt.

## Verifikation

### Gate L1 — Statik + Tests (lokal, Mac)
- `cd server && gofmt -l .` (leer)
- `cd server && go vet ./...`
- `cd server && go build ./...`
- `cd server && go test ./... -count=1` inkl. `-race` auf `./internal/services`
  und `./internal/handlers`.
- Grep-Gates: keine `build*Content`-Formatter in `static/js/`; nur `layouts.go`
  in `services/widgets/`; keine `http.Client` in `services/widgets/`; keine
  Referenz auf entfernte Handler/Routen.

### Gate L2 — Golden-Nichtdrift + Content-Gleichheit + Browser
- **Golden (bewusst, kein Re-Baseline):** `go test ./internal/services -run
  TestGoldenRender` MUSS OHNE `-update` grün sein — der Beleg, dass die
  Vereinheitlichung keinen Panel-Pixel bewegt (Golden-Designs ohne Widgets). Nur
  falls in B5 o. Ä. später bewusst `fill*Content` geändert wird, gilt die
  Golden-Update-Konvention (`golden_test.go:23-30`); in B4 nicht.
- **Offline:** `go test ./internal/services -run 'Offline|CustomAPINegativeCache'`
  unverändert grün (AC6).
- **Content-Gleichheit (AC2):** neuer Handler-Test grün; deckt die
  Regressions-Configs (AC3) ab.
- **Browser (AC3/AC4/AC7):** `cd server && go run .`, Designer öffnen; je
  Regressions-Config ein Widget platzieren und Editor-Label gegen
  `GET /preview`-Panel-Render vergleichen (calendar title+agenda+maxEvents;
  news title+showDescription+summary; system showLabels=false+horizontal;
  custom url+jsonPath gegen einen lokalen JSON-Stub; weather/forecast offline →
  kein Fake). clock/timer ticken weiter (AC7).

### Gate L3 — nicht erforderlich
Server + Browser; keine Panel-/Hardware-Interaktion. (Der Content-String, den das
Panel zeichnet, ist über die Golden-Nichtdrift + Content-Gleichheit abgedeckt.)

### Gate L5 — Review
Bestätigt: (a) genau ein Content-Dispatch (`WidgetTextContent`), Panel und
Endpoint teilen ihn, kein Fork; (b) `widgets/` nur noch `layouts.go`, alle
entfernten Symbole referenzfrei (adversariales Grep inkl. Frontend `/api/*` +
Shell-Skripte + Tests); (c) Layout-Endpoint, WeatherService, Auth/CSRF, Panel-
Renderer unangetastet; (d) SSRF: eine Fetch-Policy, `custom` als einziger neu
canvas-erreichbarer Fetch dokumentiert; (e) clock/timer-Drift-Insel explizit
vermerkt.

## Risiken

- **Golden-Re-Baseline (Missverständnis-Risiko).** Die Manager-Notiz nennt
  „Golden neu baselinen"; die Code-Verifikation zeigt jedoch: Golden-Designs
  enthalten keine Widgets → B4 ändert keinen Panel-Pixel. Risiko = jemand führt
  reflexartig `-update` aus und übertüncht einen echten Regress. **Mitigation:**
  AC5 macht „Golden OHNE `-update` grün" zur Bedingung; `-update` in B4 verboten.
  Rollback: `git checkout` der Golden-PNGs.
- **Live-Preview-Latenz auf Pi Zero.** Jede Prop-Änderung an einem Daten-Widget
  löst einen POST → ggf. einen Live-Fetch aus. **Mitigation:** (1) `_dataCache`
  60 s bleibt; Cache-Key = `type + JSON(props)` erfasst nun den VOLLEN Prop-Map
  → korrekte Invalidierung; (2) Negative-Cache (`failCache`, 2 min) bündelt
  Fehlversuche; (3) Debounce (≈ 300–500 ms) auf die Change-Kette gegen
  Tastendruck-Sturm bei `customTemplate`; (4) `/api/widget_content` läuft NICHT
  durch `renderSem` (`preview.go:79`) — es rendert kein PNG, konkurriert also
  nicht mit dem Panel-Render. Rollback: `widgets.js`-Revert (B4b).
- **SSRF-Konsolidierung, nicht -Erweiterung.** `custom` wird erstmals aus dem
  Editor gefetcht. **Mitigation/Bewertung:** gleiche Policy wie das Panel schon
  nutzt (`fetchCustomAPI` + Negative-Cache); Fetch-Pfade sinken von 2 auf 1 →
  falls SSRF-Härtung gewünscht, existiert danach EIN Chokepoint statt zwei
  (echter Netto-Gewinn). Kein neuer SSRF-Code in B4.
- **Verschluckte Test-/Skript-Referenzen.** `/api/widgets/system` steckt in
  `main_test.go`, `auth_test.go`, `test-e2e.sh`. **Mitigation:** in B4c explizit
  gelistet und mitgezogen; L1 grep-Gate fängt Übriggebliebenes.
- **clock/timer-Drift-Insel.** Bewusst akzeptiert: Browser-Uhr vs. Server-Uhr/
  Timezone bleiben client-live. Kein Rollback nötig — dokumentierte Ausnahme
  (AC7); ein späterer Task könnte sie separat angehen, gehört aber NICHT zu B4.
- **JSON-Zahlen-Coercion.** Props kommen im Body als `float64`/`bool` an. Risiko:
  ein Fill erwartet einen anderen Typ. **Mitigation:** `GetPropInt/Bool/String`
  (`design.go:977-1035`) behandeln `float64`/`string`/`bool` bereits robust; AC2
  prüft die Round-Trip-Gleichheit gegen den Direktaufruf und deckt Coercion mit ab.
