# B1: Wetter Standort-/PLZ-Suche mit Autocomplete im Designer

## Ziel
Im Eigenschaften-Panel der Widgets `weather` UND `forecast` gibt es ein Feld
„Ort oder PLZ", das beim Tippen (debounced) `/location_search` abfragt und eine
Vorschlagsliste zeigt, die Mehrdeutigkeiten auflöst (Hannover DE vs. Hanover US,
bis auf PLZ-/Adressebene wo Nominatim sie liefert). Die Auswahl schreibt
`latitude`, `longitude` und einen lesbaren `locationName` in die Widget-Props und
rendert die Vorschau neu — danach ist der bisherige Zustand „nur zwei nackte
Latitude/Longitude-Zahlenfelder" nicht mehr wahr.

Umfang > ~400 Zeilen Diff → in **B1a (Backend)** und **B1b (Frontend)** zerlegt.
Empfohlene Reihenfolge: **B1a zuerst** — B1b konsumiert die angereicherten Felder
(Region/PLZ/Land) für die Disambiguierung in der Dropdown-Sublabel-Zeile.

---

## Kontext

### Verifizierter Ist-Zustand (Read-only, Zeilen geprüft)

**Backend existiert, Response-Shape zu dünn:**
- `SearchLocation` — `server/internal/services/weather.go:443-497`: fragt
  `https://nominatim.openstreetmap.org/search?format=json&q=…` (Zeile 450),
  cappt clientseitig auf 10 (Zeile 480), setzt einen User-Agent (Zeile 456).
- `LocationResult` — `weather.go:49-54`: liefert **ausschließlich**
  `display_name`, `lat`, `lon` (snake_case JSON-Tags) → keine Region/PLZ/Land →
  Disambiguierung im UI unmöglich → **Enrichment nötig**.
- **User-Agent ist `"Mozilla/5.0"` (weather.go:456) — ein generischer
  Browser-String. Das VERLETZT die Nominatim Usage Policy** („Provide a valid
  HTTP Referer or User-Agent identifying the application; stock User-Agents such
  as generic browser strings are unacceptable") → Ban-Risiko. Muss durch einen
  identifizierenden UA ersetzt werden (App-Name + Kontakt/URL). Der Task sagte
  „User-Agent already present — confirm": vorhanden ja, **konform nein**.
- Handler `LocationSearch` — `server/internal/handlers/weather.go:26-38`:
  `GET /location_search?q=`, gibt bei leerem `q` / Fehler `[]` zurück (fail-open).
- Route — `server/main.go:236` (Hinweis: PROGRESS/§28 nennt `:217`; aktueller
  Stand ist **236**, Route unverändert vorhanden).

**Kein serverseitiger Cache / kein serverseitiges Rate-Limit:** `SearchLocation`
geht bei jedem Aufruf direkt an Nominatim. Es gibt KEINE 1-req/s-Grenze
serverseitig — schnelles Tippen kann die Policy verletzen, selbst wenn ein Client
brav debounced. Muster für einen testbaren, uhr-injizierbaren Cache/Limiter
existiert bereits: `failureCache` in `server/internal/services/negcache.go`
(injizierbare Uhr `setNow` :106, `reset` :113, bounded map + Eviction-Idee).
Positiv-Cache-Muster mit Eviction: `WeatherService` Cache + `evictOldestCache`
(`weather.go:186-203`), `maxWeatherCacheEntries` (:83).

**Frontend-Lücke:** Das Designer-Panel bietet für weather/forecast nur lat/lon:
- `getWidgetPropertyDefs` — `server/static/js/properties-panel.js:771-831`:
  `widget_weather` (:777-781) = `latitude`/`longitude`/`fontSize`,
  `widget_forecast` (:782-787) = `latitude`/`longitude`/`days`/`fontSize`.
- Render + Prop-Write-Pfad: `renderWidgetProperties` (:539-684); der generische
  `.widget-prop`-`change`-Listener (:662-683) schreibt in
  `obj.elementData.properties[key]` und ruft `WidgetPreview.updatePreview(obj)` /
  `updatePreviewDebounced` + `HistoryManager.saveState()`. **Dieser Pfad ist die
  einzige Prop-Schreibquelle — die Auswahl-Logik soll ihn wiederverwenden, nicht
  forken.**

**Referenz-UI (Muster, das B1b nachbaut) — im Setup-Wizard, NICHT anfassen:**
- `server/static/js/setup-wizard.js`: `_renderStep2` (:296-369) Such-Input +
  Ergebnisliste; `_searchLocation` (:371-424) mit Politeness-Guard
  (`_lastSearchAt`/`_searchBusy`, „≤ 1/s und nie zwei parallel" :375-378),
  Fehler-/Leer-Zustände (:392-419); `_trimCoord` (:428-431) trimmt Koordinaten
  auf 4 Nachkommastellen als String.
- CSS-Muster: `server/static/css/setup-wizard.css`
  (`.setup-location-results`, `.setup-location-result`, `.setup-search-row`).
- **Der Wizard liest `r.display_name`, `r.lat`, `r.lon` (setup-wizard.js:401-407).
  Diese drei Felder DÜRFEN durch das Enrichment NICHT entfernt/umbenannt werden
  — sonst bricht der Wizard.** (Backward-Compat = harte Invariante.)

**Props-/Rendering-Verdrahtung (bestätigt, damit B1b die richtigen Keys schreibt):**
- Server-Renderer liest `GetPropString(props,"latitude","52.52")` /
  `"longitude"` (`server/internal/services/preview.go:550-551`, :599-600).
- `GetPropString` (`design.go:977-987`) akzeptiert **String UND float64** →
  lat/lon dürfen als getrimmter String (wie Wizard) ODER als Zahl gespeichert
  werden; beides rendert serverseitig identisch.
- `locationName` existiert bereits im alten Modell
  (`server/internal/models/design.go:35`, Mapping `design.go:175-176` →
  `props["location"]`). B1b verwendet den Prop-Key **`locationName`** (konsistent
  mit Modellfeld); der Renderer nutzt ihn nicht für Wetterdaten (rein
  informativ/Label) — kein Renderpfad-Risiko.

**Auth / Type-ahead-Falle:**
- `/location_search` ist NICHT in `publicRoutes`
  (`server/internal/middleware/auth.go:36-44`) und kein `clientRoute` (:51-56) →
  **session-pflichtig** (deny-by-default).
- Globaler fetch-Interceptor `server/static/js/auth.js:12-24` macht aus jeder
  same-origin-401 (außer `/api/auth/*`) eine Voll-Seiten-Weiterleitung nach
  `/login` (`shouldRedirectToLogin` :26-39). → Läuft die Session mitten im
  Tippen ab, liefert `/location_search` 401 → Redirect nach `/login`. Das ist
  **gewolltes E5.1-Verhalten**; B1b darf es nicht unterdrücken, muss aber
  verhindern, dass ein hängender „Suche…"-Zustand als kaputte UI zurückbleibt.

### Muster, an die sich die Umsetzung halten MUSS
- Kein Frontend-Build-Step: `server/static/*` und `server/templates/*` sind
  handgepflegt, via `go:embed` ausgeliefert — direkt editieren (CLAUDE.md).
- Go: gofmt, error returns, kein panic, `log/slog`. Injizierbare Uhr/Transport
  für Tests wie in `negcache.go`/`weather_test.go` (`stubTransport`,
  `blockedTransport`, `failCache.reset()` in `t.Cleanup`).
- Kein Wechsel auf Open-Meteo-Geocoding (Kilian-Entscheidung §28) —
  Nominatim + anreichern.

---

# B1a — Backend: Enrichment + Cache + serverseitiges Rate-Limit + Tests

## Ziel
`/location_search` liefert pro Treffer genug Struktur zur Disambiguierung
(Name, Typ, PLZ, Ort, Region, Land) und geht dabei nachweislich policy-konform an
Nominatim (identifizierender User-Agent, ≤ 1 req/s serverseitig erzwungen,
Wiederholungen aus Cache) — danach ist „direkter, ungebremster,
unidentifizierter Nominatim-Zugriff mit dünner 3-Feld-Antwort" nicht mehr wahr.

## Akzeptanzkriterien (überprüfbar)

- **AC1a — Enrichment-Query.** Der ausgehende Nominatim-Request enthält
  `addressdetails=1`, `limit=10` und `accept-language=de` (deutsche Labels).
  Verifiziert per Recording-Transport, der die Request-URL assertiert.
- **AC2a — Angereicherte Antwort.** `LocationResult` (weather.go:49-54) wird um
  Felder erweitert (snake_case JSON-Tags), mindestens:
  `name`, `type`, `postcode`, `city`, `region`, `country` — zusätzlich zu den
  **unverändert erhaltenen** `display_name`, `lat`, `lon`.
  `city` = erstes vorhandenes von `city|town|village|municipality|hamlet` aus dem
  Nominatim-`address`-Objekt; `region` = erstes vorhandenes von `state|county`;
  `country` = `address.country`; `postcode` = `address.postcode` (kann leer sein);
  `type` = Nominatim-`type`/`addresstype`. Verifiziert: gegebene kanonische
  Nominatim-JSON (mit `addressdetails`) → alle Felder korrekt befüllt; PLZ-Query
  → `type`/`postcode` gesetzt.
- **AC3a — Backward-Compat Wizard.** `display_name`, `lat`, `lon` bleiben nach
  Name und Typ (String) identisch belegt; ein Test mit der Wizard-Erwartung
  (`r.display_name != "" && r.lat != "" && r.lon != ""`) bleibt grün.
- **AC4a — Identifizierender User-Agent (Policy).** Der ausgehende Request trägt
  einen App-identifizierenden UA (App-Name + Version + Kontakt/Repo-URL), **nicht**
  `Mozilla/5.0`. Verifiziert per Recording-Transport: UA ist non-empty, enthält
  den App-Identifier und ist ≠ dem alten generischen String.
- **AC5a — Serverseitiges Rate-Limit ≤ 1 req/s (Pflicht, testbar OHNE
  Wall-Clock-Sleep).** Ein Nominatim-Limiter erzwingt einen Mindestabstand von
  ≥ 1 s zwischen zwei ausgehenden Nominatim-Requests, unabhängig davon, wie
  schnell `SearchLocation` (mit je unterschiedlichen, nicht gecachten Queries)
  aufgerufen wird. Der Limiter nutzt eine **injizierbare Uhr** (Muster
  `failureCache.setNow`) und darf im Test nicht real schlafen; der Test treibt die
  Fake-Uhr und beweist: innerhalb desselben 1-s-Fensters geht **kein** zweiter
  Request an den Recording-Transport hinaus (Serialisierung/Spacing).
- **AC6a — Cache serviert Wiederholungen (Pflicht).** Zwei aufeinanderfolgende
  `SearchLocation("Hannover")` erzeugen **genau EINEN** ausgehenden
  Nominatim-Request (zweiter Treffer aus Cache). Cache-Key = normalisierte Query
  (trim, lower-case, Mehrfach-Whitespace kollabiert). Verifiziert per
  Transport-Call-Count == 1. Cache ist bounded (max. Einträge + Eviction wie
  `evictOldestCache`) und hat eine TTL (Empfehlung 6–24 h; Geodaten sind stabil).
- **AC7a — Ranking bleibt Nominatim-Importance-Reihenfolge.** Das Enrichment
  reordert NICHT; die von Nominatim gelieferte (importance-sortierte) Reihenfolge
  bleibt erhalten (nur Feld-Anreicherung, optional Dedupe exakt gleicher
  `display_name`). Verifiziert: Reihenfolge der Input-JSON == Reihenfolge der
  Ausgabe.
- **AC8a — Fail-open unverändert.** Transport-Fehler / non-200 / Parse-Fehler →
  weiterhin `[]` (kein 5xx, kein Panik); Handler-Verhalten
  (`handlers/weather.go:26-38`) bleibt. Optionaler kurzer Negativ-Cache
  (Muster `failCache`) zulässig, um Fehl-/Leerläufe nicht zu hämmern — TTL kurz
  halten (Transienten dürfen nicht lange maskiert werden).

## Non-Goals (B1a)
- Kein Wechsel auf Open-Meteo-Geocoding (§28).
- Kein eigener „Relevanz-Scorer" über Nominatim hinaus (Importance-Reihenfolge
  genügt; eigenes Re-Ranking birgt Regressionsrisiko).
- Keine Änderung am Setup-Wizard-Code/-UI (profitiert automatisch vom
  gemeinsamen `SearchLocation` — UA-Fix + Enrichment wirken auch dort, ohne die
  Wizard-JS anzufassen).
- Keine Persistenz des Location-Caches auf Platte (In-Memory genügt; anders als
  Wetter-Cache kein Restart-Überleben nötig).

## Verifikation (B1a)
- **L1 — Go Statik + Tests (Mac):**
  - `cd server && gofmt -l .` (leer), `go vet ./...`.
  - `cd server && go test ./... -race` inkl. neuer Tests in
    `server/internal/services/` (Muster `weather_test.go`: `stubTransport` als
    Recording-Transport, `failCache.reset()` / Uhr-Injektion in `t.Cleanup`):
    Enrichment-Feld-Mapping (AC2a), Wizard-Backward-Compat (AC3a),
    UA-Assertion (AC4a), Query-Param-Assertion (AC1a), **Rate-Limit-Spacing mit
    Fake-Uhr ohne Sleep (AC5a)**, **Cache-Call-Count == 1 (AC6a)**,
    Reihenfolge-Erhalt (AC7a), Fail-open (AC8a).
- **L2/L3:** nicht erforderlich (reine Backend-/Testschicht; Verhalten im UI wird
  in B1b belegt). Python: n/a.
- **Review (L5):** UA-String identifiziert die App (Security/Policy-Lens),
  Limiter ist race-clean und ohne Wall-Clock-Sleep testbar, Cache bounded,
  `display_name`/`lat`/`lon` unverändert.

## Risiken (B1a)
- **Nominatim-Ban:** primäres Risiko. Mitigation = identifizierender UA (AC4a) +
  serverseitiges 1-req/s-Limit (AC5a) + Cache (AC6a). Rollback: Limiter/Cache sind
  additive Schicht vor dem bestehenden Call; Revert stellt den alten (aber
  policy-verletzenden) Direktpfad wieder her — daher UA-Fix separat und zuerst
  committen.
- **Cache-Staleness:** ein umgezogener/umbenannter Ort bliebe bis TTL-Ablauf im
  Cache. Für Geocoding vernachlässigbar (Namen/Koordinaten stabil); TTL 6–24 h
  begrenzt es. Kein Persistieren → Restart leert ohnehin.
- **Feld-Heuristik `city`/`region`:** Nominatim-`address` ist uneinheitlich
  (town/village/municipality). Mitigation: dokumentierte Prioritätsreihenfolge
  (AC2a) + Test mit mehreren Beispieltypen (Stadt, Dorf, PLZ).

---

# B1b — Frontend: Autocomplete-UI im Designer-Panel + Mobile

## Ziel
Im weather-/forecast-Properties-Panel gibt es ein debounztes „Ort oder PLZ"-Feld
mit Vorschlags-Dropdown (Name + Region + Land, PLZ wo vorhanden); Auswahl per
Tastatur ODER Touch schreibt `latitude`/`longitude`/`locationName` in die
Widget-Props und rendert die Canvas-Vorschau neu — auch im Mobile-Viewport
(< 768 px). Danach ist „nur manuelle lat/lon-Zahlenfelder" nicht mehr wahr.

## Entscheidung: manuelle lat/lon-Felder — behalten als „Erweitert/Override"
Empfehlung (siehe offene Kilian-Frage unten): Die lat/lon-Zahlenfelder **bleiben**,
werden aber unter das Suchfeld als „Erweitert / manuelle Koordinaten" verschoben
(demoted). Begründung: (1) Fallback wenn Nominatim/Netz ausfällt oder ein Nutzer
exakte Koordinaten kennt; (2) die Auswahl schreibt in **denselben** bestehenden
`.widget-prop`-Change-Pfad (properties-panel.js:662-683) → eine Schreibquelle,
kein Fork; (3) sichtbare aufgelöste Koordinaten = Debug-/Vertrauensnutzen.

## Akzeptanzkriterien (überprüfbar)

- **AC1b — Suchfeld vorhanden (beide Widgets).** Bei ausgewähltem `widget_weather`
  UND `widget_forecast` zeigt das Panel oben ein Textfeld „Ort oder PLZ"
  (`type=text`, `inputmode=search`, `autocomplete=off`), gefolgt von den
  (demoteten) lat/lon-Feldern. Verifiziert: DOM enthält das Suchfeld für beide
  Typen.
- **AC2b — Debounce + Client-Politeness.** Tastatureingabe feuert `/location_search`
  frühestens nach ~350–500 ms Ruhe und **nie öfter als 1×/s**, nie zwei Requests
  parallel (Wiederverwendung des `_lastSearchAt`/`_searchBusy`-Musters aus
  setup-wizard.js:373-424). Mindest-Query-Länge ≥ 2 Zeichen. Verifiziert im
  L2-Protokoll: Network-Panel zeigt beim Durchtippen von „Hannover" keine
  Request-Rate > 1/s.
- **AC3b — Disambiguierte Vorschläge.** Tippen von „Hannover" zeigt mehrere
  unterscheidbare Einträge; jeder Eintrag zeigt eine Label-Zeile (Name/kurz) und
  eine Sublabel-Zeile aus `region, country` (+ PLZ wenn vorhanden), sodass
  Hannover (Niedersachsen, Deutschland) von Hanover (…, USA) unterscheidbar ist.
  Verifiziert im L2-Protokoll (Screenshot).
- **AC4b — PLZ-Suche.** Eingabe einer deutschen PLZ (z. B. „30159") zeigt den
  zugehörigen Ort als Treffer. Verifiziert im L2-Protokoll.
- **AC5b — Auswahl schreibt Props + rendert neu.** Auswahl eines Vorschlags
  (Klick/Touch ODER Enter) schreibt `latitude`, `longitude` (getrimmt auf 4
  Nachkommastellen, Muster `_trimCoord`) und `locationName` (lesbarer Kurzname,
  z. B. „Hannover, Niedersachsen") in `obj.elementData.properties`, aktualisiert
  die sichtbaren lat/lon-Felder, ruft `WidgetPreview.updatePreview(obj)` (oder
  `updatePreviewDebounced`) + `HistoryManager.saveState()` und schließt das
  Dropdown; das Widget rendert mit dem neuen Standort neu. Verifiziert im
  L2-Protokoll: nach Auswahl neue Koordinaten in den Feldern + neu gerenderte
  Vorschau.
- **AC6b — Tastatur + Touch.** ArrowUp/Down bewegen die Hervorhebung, Enter wählt,
  Escape schließt; Touch/Tap wählt. Dropdown nutzt ARIA-Combobox-Semantik
  (`role=combobox`/`listbox`/`option`, `aria-expanded`, `aria-activedescendant`).
- **AC7b — Zustände sauber.** Leere/zu kurze Query → kein/kein hängendes Dropdown;
  keine Treffer → sichtbarer Text „Keine Treffer" (deutsch); Netzwerkfehler →
  sichtbarer Text „Standortsuche gerade nicht verfügbar" statt Dauer-„Suche…".
  Verifiziert im L2-Protokoll (Nominatim-Endpoint im DevTools blockieren →
  Fehlertext statt Hänger).
- **AC8b — Mobile (< 768 px).** Im Mobile-Viewport ist das Suchfeld voll breit,
  das Dropdown sichtbar/scrollbar innerhalb des Panels (nicht abgeschnitten),
  Einträge sind mit Touch-Zielgröße bequem tippbar; Auswahl funktioniert wie
  AC5b. Verifiziert im L2-Protokoll bei Viewport-Breite < 768 px (Screenshot +
  erfolgreiche Auswahl).
- **AC9b — Type-ahead-Falle dokumentiert + kein Silent-Break.** Läuft die Session
  während des Tippens ab → `/location_search` 401 → globaler auth.js-Interceptor
  leitet nach `/login` um (bestehendes E5.1-Verhalten, NICHT unterdrücken). Falls
  fetch vor der Navigation wirft, wird der „Suche…"-Ladezustand geräumt (kein
  eingefrorenes Dropdown). Im L2-Protokoll dokumentiert (Session-Cookie
  invalidieren → Redirect statt kaputtem Dropdown).

## Non-Goals (B1b)
- Kein volles Adressformular (Straße/Hausnummer als separate Felder) — Auswahl
  eines Vorschlags genügt; „bis Adress-/PLZ-Ebene" meint die Vorschlagsgranularität,
  nicht eine Formulareingabe.
- Keine Änderung am Setup-Wizard-Suchfeld/-Flow (eigene, unabhängige UI).
- Kein Umbau des Prop-Schreibpfads / der History-/Preview-Mechanik — nur
  Wiederverwendung.
- Kein neues Backend (nutzt B1a-`/location_search`).

## Verifikation (B1b)
- **L1:** JS hat KEINEN Build-Step → kein JS-Unit-Gate. Go bleibt unberührt
  (`gofmt`/`go vet`/`go test ./...` grün als Regressionsschutz). Python: n/a.
- **L2 — Browser-Protokoll (Mac), `cd server && go run .`, eingeloggt:**
  - Desktop: „Hannover" → mehrere disambiguierte Vorschläge (AC3b); PLZ „30159"
    → passender Ort (AC4b); Auswahl setzt lat/lon + `locationName` + Re-Render
    (AC5b); Debounce/Rate im Network-Panel (AC2b); Tastatur+Touch (AC6b);
    Leer/Keine-Treffer/Netzfehler (AC7b, Endpoint blocken).
  - **Mobile-Viewport < 768 px** (DevTools Device-Emulation): AC8b (Feld+Dropdown
    nutzbar, Auswahl erfolgreich) — Screenshot.
  - **Abgelaufene Session** (`eink_session`-Cookie invalidieren) mitten im Tippen
    → `/login`-Redirect, kein hängendes Dropdown (AC9b).
- **Review (L5):** Auswahl nutzt den bestehenden `.widget-prop`-Schreibpfad (keine
  Fork-Logik), Dropdown-Zustände deterministisch geräumt, ARIA korrekt,
  Wizard-Suche unangetastet.
- **L3:** nicht erforderlich (rein browserseitig; keine Panel-/Hardware-Interaktion).

## Risiken (B1b)
- **Type-ahead-Falle (Auth):** 401 mitten im Tippen → Redirect. Mitigation:
  E5.1-Verhalten bewusst belassen + Ladezustand bei fetch-throw räumen (AC9b).
  Rollback: rein additive UI; Revert des properties-panel.js/CSS-Diffs entfernt
  die Suche, lat/lon-Felder bleiben funktional.
- **Dropdown-Clipping/Overflow im Panel (v. a. Mobile):** könnte abgeschnitten
  werden. Mitigation: Positionierung/`overflow`/`z-index` im Panel-Kontext testen
  (AC8b), CSS-Muster aus setup-wizard.css adaptieren.
- **Doppelte Prop-Schreibquelle:** wenn die Auswahl NICHT den bestehenden
  Change-Listener nutzt, driften manuelle Eingabe und Auswahl auseinander.
  Mitigation: Auswahl setzt `.value` der lat/lon-Felder und dispatcht `change`
  (ein Schreibpfad) oder schreibt `elementData` + ruft dieselben Update-Aufrufe;
  im Review geprüft.

---

## Offene Frage an Kilian (nicht blockierend, Default gesetzt)
**Manuelle lat/lon-Felder behalten oder verstecken?** Default in dieser Spec:
**behalten**, aber als „Erweitert / manuelle Koordinaten" unter das Suchfeld
demoten (Begründung oben: Fallback bei Netz-/Nominatim-Ausfall, ein einziger
Prop-Schreibpfad, sichtbare aufgelöste Koordinaten). Alternative auf Wunsch:
lat/lon read-only anzeigen (nur Anzeige der Auswahl) oder in ein
zusammenklappbares „Erweitert"-Panel legen wie
`setup-wizard.js` Step 3 (`setup-advanced-toggle`).
