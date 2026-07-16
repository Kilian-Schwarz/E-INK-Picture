# B2: Rundungs-/Render-Parität Designer-Canvas ↔ E-Ink-Panel

## Ziel
Ein abgerundetes Rechteck (Fill, Stroke, Border-Radius, inkl. skalierte und
große Radien) wird vom Go-Panel-Renderer geometrisch **gleich** dargestellt wie
im Fabric-Designer — Radius, Stroke-Position/-Breite und Corner-Geometrie
stimmen bei `render_quality=high` innerhalb definierter Toleranz überein. Vorher
zeichnet das Panel den Radius zu klein (halbiert bei Standardqualität, zusätzlich
halbiert bei skalierten Shapes) und den Stroke an falscher Position.

## Kontext

### Betroffene Dateien
- `server/internal/services/preview.go` — Panel-Renderer (Go-Raster)
- `server/static/js/canvas-manager.js` — Fabric `object:modified`-Handler
- `server/static/js/element-factory.js` — Shape-Erzeugung (`createShape`)
- `server/static/js/storage.js` — Canvas→Design-JSON Export
- `server/static/js/properties-panel.js` — Border-Radius/Stroke-UI
- `server/internal/services/testdata/designs/*.json` — Golden-Testdesigns
- `server/internal/services/testdata/golden/*.png` — Golden-Referenz-PNGs
- `server/internal/services/golden_test.go` — Golden-Harness (`goldenDesigns`, `-update`)

### Verifizierte Divergenzpunkte (alle gegen `main` @ `3ecf4ce` geprüft)

**D0 — NEU (nicht im Kartografen-Summary): Go skaliert `rx`/`strokeWidth` NICHT
mit dem Supersample-Faktor.** `preview.go:221-224` skaliert `x/y/w/h` jedes
Elements mit `scale = supersampleScale()` (2.0 high, 1.5 medium, 1.0 fast,
`preview.go:139-150`). `renderShapeElement` (`preview.go:931-969`) bekommt diese
bereits skalierten `x/y/w/h`, liest aber `rx := GetPropInt(props,"rx",0)`
(`:952`) und `sw := GetPropInt(props,"strokeWidth",0)` (`:949`) **roh, ohne
`* scale`**. Folge bei Default-Qualität high (scale 2.0): Box wird 2× gezeichnet,
Radius bleibt 1× → nach CatmullRom-Downscale ist der sichtbare Radius **halb so
groß** wie im Design. Betrifft **jede** abgerundete Shape, auch unskalierte.
Ironie: nur bei `fast` (scale 1.0) ist der Radius korrekt, dort sind die Ecken
aber alias-hart.

**D1 — `rx` unskaliert exportiert, `width`/`height` skaliert (Canvas-Drift).**
`storage.js:79-80` exportiert `width`/`height` via `getScaledWidth()/getScaledHeight()`
(enthält `scaleX`/`scaleY`), `storage.js:133-135` exportiert `strokeWidth`/`rx`/`ry`
**roh** (`obj.strokeWidth`/`obj.rx`/`obj.ry`). Ursache: `object:modified`
(`canvas-manager.js:117-208`) hat Branches für widget-group (`:123`), text
(`:161`), image (`:188`) — **keinen** für `shape`/`rect`. Ein per Resize-Handle
skaliertes Rechteck behält `scaleX/scaleY≠1`; Canvas zeigt Radius = `rx*scale`,
Export liefert `width=base*scale`, `rx=base` → Panel zeichnet Radius proportional
zu klein. Kompoundiert mit D0.

**D2 — Panel ignoriert `ry`.** `preview.go:952` liest nur `rx`;
`drawRoundedRectFilled` (`:1511`) zeichnet achsensymmetrisch. Fabric klemmt `rx`
und `ry` unabhängig (`rx→min(rx,w/2)`, `ry→min(ry,h/2)`) → bei breit-flachen
Boxen mit großem Radius zeichnet Fabric eine **Ellipse**, Go einen Kreis.

**D3 — Stroke „inside-the-box" statt zentriert.** `preview.go:954-968`: äußeres
gefülltes Rounded-Rect in Stroke-Farbe an `(x,y,w,h,rx)`, dann inneres an
`(x+sw, y+sw, w-2sw, h-2sw, rx-sw)` (clamp ≥0) in Fill-Farbe. Der Stroke liegt
**komplett innerhalb** `[x, x+w]`. Fabric zeichnet den Stroke **zentriert auf der
Pfadkante** (je `sw/2` innen/außen), Außenkante bei `x-sw/2`, äußerer
Eck-Radius ≈ `rx+sw/2`. → andere Position, andere Eck-Geometrie, andere
Fill-Fläche. **Zusatzbug:** bei `fill:"transparent"` + Stroke wird `hasFill=false`
(`:938`), der innere Fill-Zeichenschritt entfällt (`:963`) → das Panel malt ein
**vollflächiges** Stroke-farbenes Rechteck statt eines Rings.

**D4 — Radius-Clamp divergiert bei großen Radien.** `drawRoundedRectFilled`
klemmt **einen** Radius `r` auf `min(w/2, h/2)` (`preview.go:1521-1526`). Fabric
klemmt `rx` und `ry` getrennt. Bei `w=200,h=60,rx=100`: Fabric →
`rx=100, ry=30` (Ellipse), Go → `r=30` (Kreis). Wird mit D2 gemeinsam gelöst.

**D5 — Corner-AA nur via Supersampling.** Der Eck-Test ist ein hartes binäres
Distanzkriterium `dx*dx+dy*dy <= rr*rr` (`preview.go:1538-1568`), kein
Per-Pixel-AA. Effektives Kanten-AA entsteht nur durch Supersample 2.0×/1.5× +
CatmullRom-Downscale (`preview.go:300-312`). Bei `fast` (scale 1.0) sind Ecken
**echt aliased**. Bestätigt.

**D6 — Koordinaten-Mapping ist 1:1** (Canvas 800×480 = Panel 800×480).
`einkOffsetX=200`/`einkOffsetY=160` (`preview.go:35-36`) sind **toter Code**
(grep: keine Verwendung außer der Deklaration). Offset-Hypothese **widerlegt** —
keine Änderung nötig.

### Bestehende Muster, an die sich der Implementierer halten muss
- Supersample-Skalierung vor dem Zeichnen anwenden (wie `fontSize` in
  `preview.go:260`: `int(float64(GetPropInt(...)) * scale)`).
- Scale-Absorption im `object:modified`-Handler exakt nach dem Vorbild der
  text/widget/image-Branches (`canvas-manager.js:122-201`): `width/height`
  (und hier zusätzlich `rx/ry`) mit `scaleX/scaleY` multiplizieren,
  `scaleX/scaleY = 1` zurücksetzen, `setCoords()`.
- Golden-Workflow: neue Testdesigns unter `testdata/designs/`, in `goldenDesigns`
  (`golden_test.go:33`) eintragen, Golden-PNGs **nur bewusst** via
  `go test ./internal/services -run TestGoldenRender -update` erzeugen und im
  selben Commit wie die Renderer-Änderung committen (Konvention `golden_test.go:27-29`).
- Kein Build-Step / kein Linter für das Frontend (Vanilla JS, go:embed).

## Die konkrete Fix-Entscheidung

### Panel (Go, `preview.go`) — der Haupt-Fix (D0, D2, D3, D4)

1. **Supersample-Skalierung an `rx/ry/sw` durchreichen (D0).** `scale float64`
   an `drawElement` und `renderShapeElement` durchreichen (analog zum bereits
   vorskalierten `fontSize`). In `renderShapeElement`:
   ```
   rx := int(float64(GetPropInt(props, "rx", 0)) * scale)
   ry := int(float64(GetPropInt(props, "ry", GetPropInt(props, "rx", 0))) * scale)
   sw := int(float64(GetPropInt(props, "strokeWidth", 0)) * scale)
   ```
   `ry` fällt auf `rx` zurück, wenn nicht gesetzt. Plumbing-Detail frei
   (Parameter durchreichen ODER `s.supersampleScale()` in `renderShapeElement`
   erneut aufrufen — Parameter bevorzugt, kein erneuter Settings-Read mitten im
   Render).

2. **Elliptische Radien + unabhängiger Clamp (D2, D4).**
   `drawRoundedRectFilled(img, x, y, w, h, rx, ry int, c)` — Signatur um `ry`
   erweitert. Clamp getrennt: `if rx > w/2 { rx = w/2 }`, `if ry > h/2 { ry = h/2 }`.
   Eck-Test elliptisch: `(dx/rx)² + (dy/ry)² <= 1` (float), Strips entsprechend
   `x+rx … x+w-rx` bzw. `y+ry … y+h-ry`. `rx<=0 && ry<=0` → gefüllter Rect
   (bestehender Fast-Path `:1516-1520`).

3. **Zentrierter Stroke als Ring-Band (D3).** Neue Routine
   `drawRoundedRectStroke(img, x, y, w, h, rx, ry, sw int, c)` malt **nur** das
   Band, das die Pfadkante um `sw/2` innen/außen überdeckt (äußere Rounded-Rect
   `(x-sw/2, y-sw/2, w+sw, h+sw, rx+sw/2, ry+sw/2)` minus innere Rounded-Rect
   `(x+sw/2, y+sw/2, w-sw, h-sw, rx-sw/2, ry-sw/2)`, innere Radien clamp ≥0).
   `renderShapeElement`-Ablauf:
   - `if hasFill`: `drawRoundedRectFilled(x, y, w, h, rx, ry, fillColor)` (voller Pfad)
   - `if hasStroke`: `drawRoundedRectStroke(x, y, w, h, rx, ry, sw, strokeColor)`
     **nach** dem Fill (überpinselt äußere `sw/2` des Fills, fügt `sw/2` außen an)
   Damit: transparenter Fill + Stroke → korrekter Ring (Interior unberührt);
   opaker Fill + Stroke → zentrierter Stroke wie Fabric. Der alte
   `innerRx=rx-sw`-Pfad (`:959-964`) entfällt.

4. **`einkOffsetX/Y` optional entfernen (D6)** — toter Code; nur wenn es keine
   ungeprüfte Verwendung gibt (grep-verifiziert leer). Rein kosmetisch, kein AC.

### Canvas (JS) — Scale-Absorption für Shapes (D1)

5. **`canvas-manager.js` `object:modified`: Shape-Branch** nach dem image-Branch
   (~`:201`), Vorbild text/widget:
   ```js
   if (type === 'shape' && obj.type === 'rect') {
       var sSx = obj.scaleX || 1, sSy = obj.scaleY || 1;
       if (sSx !== 1 || sSy !== 1) {
           obj.set({
               width:  Math.round(obj.width  * sSx),
               height: Math.round(obj.height * sSy),
               rx: Math.round((obj.rx || 0) * sSx),  // rx folgt horizontaler Skalierung
               ry: Math.round((obj.ry || 0) * sSy),  // ry folgt vertikaler Skalierung
               scaleX: 1, scaleY: 1,
           });
           obj.setCoords();
       }
   }
   ```
   Danach liegen `width/height` und `rx/ry` im **selben** Koordinatenraum;
   `storage.js:79-80/133-135` exportiert konsistente Werte ohne eigene Änderung
   (`getScaledWidth()==width`, weil scale=1). Nicht-uniforme Skalierung erzeugt
   korrekt `rx≠ry` (elliptische Ecken) — dafür ist der Go-Fix #2 ausgelegt.

6. **`element-factory.js` `createShape`: `strokeUniform: true`** setzen
   (nach `:99`). Damit skaliert Fabric die Stroke-Breite **nicht** mit dem
   Objekt → exportierter `strokeWidth` bleibt scale-unabhängig und passt zum
   Single-Scalar-Modell des Panels. Entfernt die Stroke-unter-Skalierung-Ambiguität
   an der Wurzel (kein `strokeWidth`-Baking im `object:modified`-Handler nötig).

### Entscheidungen (explizit)
- **`ry`:** Go **unterstützt `rx≠ry`** (elliptische Ecken, unabhängiger Clamp).
  Grund: die Canvas-Scale-Absorption erzeugt bei nicht-uniformer Skalierung
  legitim `rx≠ry` — nur so bleibt das faithful. Die UI behält ein einzelnes
  „Border Radius"-Feld (`properties-panel.js:531-535` setzt weiter `rx==ry`);
  das ist kein Widerspruch, weil die Divergenz erst durch Skalierung entsteht.
- **Stroke:** Panel **matcht Fabrics zentrierte Stroke-Geometrie** (Ring-Band),
  kein „accepted difference". Das ist die Bedingung dafür, dass „sieht aus wie
  der Designer" für gerahmte Boxen wahr wird.
- **AA/Fast:** Parität ist bei `render_quality=high` (Default) definiert. Bei
  `fast` bleiben Ecken alias-hart (D5) — **Non-Goal**, kein Per-Pixel-AA.

## Akzeptanzkriterien

**AC1 (D0/D1 — Radius-Skalierung, deterministisch, browserfrei):** Für ein
Design mit Shape `(x,y,w,h,rx=R)` bei `render_quality=high` gemessen am finalen
800×480-PNG: der Fill-Beginn auf der Oberkante liegt bei `x+R (±2px)`, auf der
linken Kante bei `y+R (±2px)` — verifiziert durch Kanten-Scan. Gilt für
`R ∈ {8, 16, 24, 40}` und für Shapes mit vorab absorbierter nicht-uniformer
Skalierung. Vor dem Fix schlägt dies fehl (gemessener Radius ≈ `R/2`).

**AC2 (D2/D4 — Ellipse bei großem Radius):** Shape `w=200, h=60, rx=100`
rendert mit horizontalem Eck-Radius `100→min(100,100)=100 (±2px)` und vertikalem
`ry-Clamp = min(100,30)=30 (±2px)` (Pill/Ellipse), **nicht** als Kreis r=30.
Gemessen an der Corner-Kontur des finalen PNG.

**AC3 (D3 — Stroke zentriert):** Shape mit opakem Fill + `strokeWidth=sw`:
die Außenkante des Strokes liegt bei `x-sw/2 (±1px im 800×480-Raum, high)`, die
Fill/Stroke-Grenze bei `x+sw/2 (±1px)`. Ein Shape mit `fill:"transparent"` +
Stroke rendert als **Ring** (Interior = Hintergrund, nicht Stroke-Farbe) —
verifiziert durch ein Hintergrund-farbenes Zentrum-Pixel.

**AC4 (Golden byte-match):** Neues Testdesign `testdata/designs/rounding.json`
(Matrix: `rx ∈ {0,8,16,24,40}`, ein Pill `rx>h/2`, ein `rx≠ry`-Fall, dünner +
dicker Stroke, ein transparenter-Fill-Ring), eingetragen in `goldenDesigns`
(`golden_test.go:33`). `TestGoldenRender` und `TestPaletteExactness` grün für
beide Displays; Golden-PNGs committed. Palette bleibt exakt (Farben aus
`{#000,#FFF,#F00,#FF0,#0F0,#00F}` wählen, damit Quantisierung ein No-op ist und
die Geometrie sauber vergleichbar bleibt).

**AC5 (bestehende Golden regeneriert + inspiziert):** `basic.json` enthält
bereits abgerundete/gerahmte Shapes (`s_red rx=12`, `s_yellow rx=12`+stroke,
`s_blue rx=20`, `s_panel rx=16`+stroke). Deren Golden-PNGs
(`basic__waveshare_7in5_v2.png`, `basic__waveshare_7in3_e.png`) **ändern sich**
durch den Fix und müssen via `-update` neu erzeugt, **visuell inspiziert**
(Radius größer/korrekt, Stroke zentriert) und im selben Commit committed werden.

**AC6 (L2 Canvas↔Panel-Diff, Reviewer sieht beide Bilder):** Referenzbild
`testdata/parity/rounding__canvas_ref.png` wird **einmalig** aus dem echten
Fabric-Designer erzeugt (Prozedur unten) und committed. Ein Go-Test rendert
`rounding.json` **raw** (`Render(ctx, design, /*raw=*/true)`, keine
Quantisierung/Dithering) bei `high` und vergleicht gegen das Referenzbild:
- Fraktion der Pixel, deren Per-Kanal-Differenz > 24/255 ist: **< 3.0 %** der
  Gesamtpixel (der Rest ist reiner Kanten-AA-Fransenraum Vektor-vs-Raster).
- In jedem 40×40-Eckfenster jeder Shape: < 20 % abweichende Pixel (die Ecken
  liegen übereinander).
Der Reviewer legt Panel-Render und `..._canvas_ref.png` nebeneinander (bzw. als
Overlay) und bestätigt visuell: gleicher Radius, gleiche Stroke-Lage, keine
sichtbare Verschiebung. Schwellen sind Regressions-Guardrails; das visuelle
Urteil ist das eigentliche Gate.

## Non-Goals
- **KEINE** Widget-Content-Vereinheitlichung (das ist B4: verticalAlign,
  fontFamily, Fake-Daten, calendar title etc.).
- **KEIN** Font-/Text-Rendering-Fix (fontFamily-Drift ist eigener Folgetask).
- **KEIN** Per-Pixel-Corner-AA bei `render_quality=fast` (D5 dort akzeptiert).
- **KEIN** neuer UI-Regler für getrenntes `rx`/`ry` — das „Border Radius"-Feld
  bleibt ein Wert; `rx≠ry` entsteht nur durch Skalierung.
- **KEINE** Änderung am Koordinaten-Mapping (D6 widerlegt); `einkOffset`-Löschung
  optional/kosmetisch, nicht Teil der ACs.
- **KEINE** neuen Shape-Typen (Kreis, Linie, Polygon) — nur `rect`.

## Verifikation

**L1 (statisch):**
```
cd server && gofmt -l . && go vet ./... && go test ./...
```
Muss AC1–AC5 abdecken (neuer Mess-Test `TestShapeRoundingGeometry` +
`TestGoldenRender` inkl. `rounding` + regenerierte `basic`-Golden). JS:
kein Linter im Repo — manueller Review von `canvas-manager.js`/`element-factory.js`
+ Design im Browser laden, `object:modified` auslösen (Resize), keine
Konsolen-Fehler, `canvasToDesignJSON()` liefert `scaleX/scaleY=1` und
konsistente `rx/width`.

**L2 (Render-Verifikation, Canvas↔Panel-Diff):** AC6.
Referenzbild-Prozedur (einmalig, dokumentiert, im Commit-Body verlinken):
1. Designer öffnen, `rounding.json` laden.
2. Grid ausblenden (Grid-Toggle) — nur Shapes sollen im Export sein.
3. Browser-Konsole:
   `CanvasManager.getCanvas().toDataURL({format:'png', multiplier:1})`
   (liefert 800×480 1:1, da Canvas = Displaygröße).
4. Data-URL als PNG speichern → `testdata/parity/rounding__canvas_ref.png`, committen.
Go-seitiger Diff-Test rendert `rounding.json` raw und prüft die Schwellen aus AC6.

**L3 (Hardware-in-the-Loop, an hardware-validator auf dem Pi):** Design
`rounding.json` als aktives Design setzen, Panel-Foto aufnehmen, gegen einen
Designer-Screenshot desselben Designs legen. Bestätigen: Radius und Stroke-Lage
sichtbar identisch. Deferred bis Pi verfügbar (siehe PROGRESS: L3-Warteschlange).

## Risiken
- **Golden-Bruch (erwartet):** `basic__*.png` **und** alle Konsumenten der alten
  Stroke-/Radius-Geometrie ändern sich. Rollback = `-update` rückgängig + Go-Diff
  revert. Mitigation: AC5 erzwingt bewusste Regeneration + Inspektion; niemals
  `-update` blind zum Rotgrün-Machen (Konvention `golden_test.go:27-29`).
- **Supersample-Interaktion:** `rx/sw` jetzt `*scale` → Eck-Arc-Loop läuft über
  2×-Radius, dann Downscale. Erwartet glattere Ecken (Nebeneffekt, gewollt).
  Regressionsschutz: `TestPaletteExactness` (Palette bleibt exakt) +
  `TestRenderDeterminism` (byte-stabil) müssen grün bleiben.
- **Fast-Quality:** Ecken bleiben hart (D5, Non-Goal). Falls Kilian dort auch
  Glätte will → separater Task (Per-Pixel-Coverage-AA in `drawRoundedRect*`).
- **Nicht-uniforme Skalierung erzeugt `rx≠ry`:** neu für den Panel-Pfad; der
  elliptische Eck-Test muss `rx=0 XOR ry=0` und `rx/ry`-Clamp auf 0 sauber
  behandeln (sonst Division durch 0). Test-Fall in `rounding.json` einplanen.
- **`strokeUniform:true` ändert Canvas-Verhalten** bestehender skalierter Shapes
  minimal (Stroke skaliert nicht mehr mit) — nach dem ersten `object:modified`
  ist scale=1, dann irrelevant. Für frisch geladene Designs (scale=1) folgenlos.
- **Nicht kaputt machen:** unskalierte, nicht-abgerundete Shapes (`rx=0, sw=0`)
  müssen byte-identisch bleiben (Fast-Path `preview.go:1516-1520` erhalten);
  `basic.json` `s_black`/`s_green` (rx=0, kein Stroke) sind der Wächter.

## Umfang / Split-Empfehlung
Als **ein** Task machbar, aber intern sequenzieren — bei Zeitdruck an der
Stroke-Grenze splitbar:
- **Phase 1 (größter, universeller Win):** Go D0 (`rx/sw *scale`) + D2/D4
  (elliptisch, Clamp) + Canvas D1 (Scale-Absorption) + `strokeUniform`. Damit ist
  die **Radius-Geometrie** paritätisch. Golden `basic` regenerieren.
- **Phase 2 (riskanter, unabhängiger):** Go D3 (zentrierter Stroke-Ring +
  Transparent-Fill-Ring). Ändert erneut Golden.
Phase 1 und Phase 2 könnten zwei Branches/Commits sein (Radius- vs.
Stroke-Parität), teilen sich aber `rounding.json` + den L2-Diff-Test.
