# B7: Designer-"Preview"-Button zeigt nichts / kein Bild

## Ziel
Ein Klick auf den Designer-"Preview"-Button führt in JEDER unterstützten Kombination
(Auth-Zustand × Viewport × CDN erreichbar/blockiert) zu einem sichtbaren Ergebnis —
entweder ein gerendertes Vorschaubild ODER eine sichtbare, benannte Fehlermeldung im
Modal; ein still leeres/blindes Bild oder ein wirkungsloser Klick sind danach unmöglich.

## Kontext

### Verifizierter Ist-Zustand (Read-only-Analyse, Zeilen geprüft)
Die Verdrahtung ist intakt — der Bug liegt NICHT in fehlender Registrierung:
- `server/static/js/toolbar.js:22` liest `#preview-btn`, `:34` bindet den Click auf
  `Toolbar.preview()`, `:35` ruft `initPreviewModal()`.
- `server/static/js/designer.js:41` ist der EINZIGE `DOMContentLoaded`-Handler des Designers
  (async). Reihenfolge: `:46` `CanvasManager.init()` läuft VOR `:52` `Toolbar.init()` — beide
  synchron, vor dem ersten `await`.
- `server/static/js/canvas-manager.js:15` `new fabric.Canvas('design-canvas', …)` — wirft
  `ReferenceError`, wenn das globale `fabric` fehlt.
- Route: `server/main.go:220` `GET /preview` → `PreviewHandler.Preview`,
  `:221` `POST /api/preview_live` → `PreviewHandler.PreviewLive`
  (`server/internal/handlers/preview.go:30-61` bzw. `:64-85`).

### Die 401-Hypothese des Managers ist WIDERLEGT
- `server/static/js/auth.js:12-24` installiert einen globalen `fetch`-Interceptor, der jede
  same-origin, nicht-`/api/auth/*` **401**-Antwort in eine Voll-Seiten-Weiterleitung nach
  `/login` verwandelt (`:16-17`, Filter in `shouldRedirectToLogin` `:26-39`) — also KEIN
  stilles kaputtes Bild.
- Der Fallback `GET /preview` ist ein `clientRoute`
  (`server/internal/middleware/auth.go:51-56`), der eine gültige Session akzeptiert und
  daher nicht zuverlässig 401 liefert.

### Gerankte reale Ursachen-Kandidaten — der Implementierer MUSS live reproduzieren, welche tatsächlich feuern (nicht annehmen)
1. **[höchster Wert] Init-Exception VOR `Toolbar.init()`.** `fabric.js` wird von einem
   EXTERNEN CDN geladen (`server/templates/designer.html:450`, `cdnjs.cloudflare.com`). Auf
   einem offline/LAN-only Pi (dem Kern-Deployment des Projekts!) lädt fabric nicht →
   `canvas-manager.js:15` wirft in `CanvasManager.init()` (`designer.js:46`) INNERHALB des
   einen async-`DOMContentLoaded`-Handlers, BEVOR `Toolbar.init()` (`designer.js:52`) läuft →
   der Preview-Click-Listener (`toolbar.js:34`) wird nie gebunden → Klick tut buchstäblich
   nichts (kein Modal). (`responsive-layout.js:210-212` hat einen EIGENEN
   `DOMContentLoaded` und läuft trotzdem an — die Burger-Umlagerung passiert also evtl.,
   der Preview-Listener aber nie.)
2. **Session zwischen Seitenaufbau und Klick abgelaufen** → `POST /api/preview_live` 401 →
   `auth.js` leitet nach `/login` um (`middleware/auth.go:112-118` liefert 401 JSON für
   die Nicht-Page-Route). Nutzer erlebt "nichts passiert", landet aber real auf Login.
3. **Reverse-Proxy/Cloud, `Origin` ≠ `Host`** → CSRF-Guard liefert **403**
   (`middleware/auth.go:123-125`). 403 wird von `auth.js` NICHT abgefangen → `toolbar.js:203`
   `throw` → `isInitial`-Fallback `GET /preview?name=` (`toolbar.js:221-223`, ein GET, nicht
   mutierend, kein CSRF-Check) → zeigt das GESPEICHERTE (evtl. veraltete) Design, nicht
   "nichts".
4. **`PreviewLive` 500 für ein bestimmtes Design/Widget/Font** (`preview.go:78`) → Fallback;
   wenn nie gespeichert (`#design-select` leer) → nacktes `GET /preview` rendert das AKTIVE
   Design oder liefert 404 (`preview.go:54`).
5. **`img#preview-image` (`designer.html:290`) hat KEINEN `onerror`-Handler** → wenn eine
   Fallback-URL einen JSON-Fehlerkörper in das `<img>` liefert (z. B. `GET /preview` → 404
   `{"message":"No design"}`), ist das Modal offen (`display:flex`, `toolbar.js:174`), das
   Bild aber leer — exakt die Variante "Modal öffnet, kein Bild".

Zusätzlich, nicht-CDN: Auf **mobile (<768px)** und **tablet (768–1569.98px)** wird
`#preview-btn` in das Burger-Overflow-Menü umgelagert
(`responsive-layout.js:17` MENU_SELECTORS_MOBILE, `:23` MENU_SELECTORS_TABLET;
Grenzen `:34-35`). Ein Nutzer, der den Burger nie öffnet, sieht "Preview tut nichts", weil
der Button schlicht ausgeblendet ist.

### Muster, denen die Umsetzung folgen MUSS
- Kein Build-Step im Frontend: `server/static/*` und `server/templates/*` sind
  handgepflegt, via `go:embed` ausgeliefert — direkt editieren (CLAUDE.md).
- Bestehende Status-/Fehler-UX im Preview-Modal weiterverwenden: `setPreviewStatus(text)`
  (`toolbar.js:263-268`) schreibt in `#preview-status` (`designer.html:289`) und toggelt
  dessen Sichtbarkeit. Ein sichtbarer Fehlerzustand SOLL über diesen bereits vorhandenen
  Mechanismus laufen, keine neue Notification-Schiene erfinden.
- Session-/CSRF-/Redirect-Semantik von E5.1 bleibt unverändert (`auth.js`, `middleware/auth.go`).

## Akzeptanzkriterien

- **AC0 — Reproduktion ZUERST (kein Fix ohne reproduzierten Bug).** Vor jeder Code-Änderung
  wird die Vollmatrix live durchgespielt und protokolliert:
  Auth-Zustände {Passwort NICHT gesetzt · eingeloggt frische Session · eingeloggt
  abgelaufene Session} × Viewports {Desktop ≥1570px · Tablet 768–1569px · Mobile <768px} ×
  {CDN erreichbar · CDN blockiert/offline}. Für JEDE Zelle wird die Browser-Konsole erfasst
  (insbesondere auf `fabric === undefined` / `ReferenceError` in `CanvasManager.init`) sowie
  Screenshot des Ergebnisses. Ergebnis-Artefakt: eine ausgefüllte Matrix, die dokumentiert,
  **welche(r) der Mechanismen [1]–[5] und/oder die Burger-Ausblendung real feuern** — inkl.
  der genauen Symptomatik pro Zelle. Der Fix in AC2 adressiert genau die so bestätigten
  Ursachen, nicht die Vermutung.
- **AC1 — Fehler ist niemals unsichtbar (Kern-Fix, unabhängig von der bestätigten Ursache).**
  `img#preview-image` erhält einen `onerror`-Handler; schlägt das Laden des `<img>` fehl
  (kaputtes/JSON-in-`<img>`, Fallback-404 etc.), zeigt das Modal einen sichtbaren, benannten
  Fehlerzustand über `#preview-status` (z. B. „Preview konnte nicht geladen werden") statt
  eines still leeren Bildes. Der `onerror` invalidiert nicht die Session-Logik und feuert
  nicht für die erfolgreiche Object-URL. Verifizierbar: In der reproduzierten Fehlerzelle ist
  nach dem Fix Text in `#preview-status` sichtbar (`display != none`, non-empty), das Modal
  bleibt bedienbar/schließbar.
- **AC2 — Bestätigte Nicht-CDN-Ursache wird behoben.** Jede in AC0 bestätigte Ursache aus
  {[2] Session-Redirect-Wahrnehmung, [3] CSRF-403, [4]/[5] Render-Fehler/Fallback-Blank,
  Burger-Ausblendung} wird so behandelt, dass der Nutzer ein eindeutiges, benanntes Ergebnis
  erhält:
  - Bei [3] 403: Der `catch`-Zweig (`toolbar.js:215-230`) unterscheidet den 403-Fall und
    zeigt eine passende Meldung (z. B. „Vorschau abgelehnt (cross-origin)"), statt den
    403 stumm in den Saved-Design-Fallback zu leiten und als „unavailable" zu labeln.
  - Bei [4]/[5]: Ein Fallback-Response, der KEIN Bild ist (Statuscode ≠ 2xx oder
    Content-Type ≠ `image/*`), führt zum sichtbaren Fehlerzustand aus AC1, nicht zu einem
    leeren `<img>`.
  - Bei Burger-Ausblendung (mobile/tablet): keine Code-Änderung nötig, aber im
    Repro-Protokoll (AC0) als „by design, Button im Burger" dokumentiert, damit dies nicht
    fälschlich als Bug behandelt wird.
  (Falls AC0 zeigt, dass NUR Mechanismus [1] feuert, ist AC2 durch die Scope-Entscheidung in
  AC4 abgedeckt und dieser Punkt reduziert sich auf AC1.)
- **AC3 — Regressionstest definiert (Test-Engineer besitzt die Umsetzung).** Ein Test deckt
  den Preview-Pfad inklusive Auth ab und MUSS mindestens assertieren:
  - `POST /api/preview_live` mit gültiger Session + same-origin `Origin` → 200,
    `Content-Type: image/png`, PNG dekodierbar (analog `handlers/preview_test.go:45-93`).
  - `POST /api/preview_live` mit gültiger Session + fremdem `Origin` → **403**, NICHT 401
    (durch den Guard; Muster `middleware/auth_test.go:232` `TestGuardCSRFOriginCheck`).
  - Abgelaufene/fehlende Session auf `POST /api/preview_live` → **401** (damit der
    Client-Redirect greift; Muster `middleware/auth_test.go:139`
    `TestGuardSessionExpiryOnNextRequest`).
  - `GET /preview` (Fallback-Route) mit gültiger Session → 200 PNG; ohne Session/Token → 401.
  - Der clientseitige `onerror`-Pfad (AC1) ist im Repro-Protokoll (L2) belegt, da JS keinen
    Unit-Build-Step hat; Go-seitig genügt die Status-Mapping-Abdeckung oben.
- **AC4 — Scope-Flag zu Mechanismus [1] (CDN offline).** Bestätigt AC0, dass [1] die reale
  Ursache ist (fabric.js vom CDN auf dem Offline-Pi nicht ladbar), gilt: Der eigentliche Fix
  — fabric.js (und die Google-Fonts-Einbindung `designer.html:12`) lokal bündeln und via
  `go:embed` aus `server/static/` ausliefern — ist eine GRÖSSERE Änderung, die zugleich die
  bekannte Altlast „Offline-Pi = gar kein Designer" (PROGRESS-Erkenntnis: Designer lädt
  Fabric 5.3.1 + Google Fonts per CDN) auflöst. Diese Bündelung wird NICHT in B7
  hineingeschmuggelt, sondern als eigener Spec **B7b / offline-designer-assets** empfohlen.
  B7 liefert für [1] als Minimum: eine sichtbare, benannte Fehlermeldung, dass der Designer
  ohne Netz/CDN nicht initialisiert ist (statt eines wirkungslosen Klicks) — konkret ein
  früh sichtbarer Hinweis, wenn `typeof fabric === 'undefined'` beim Init erkannt wird, sodass
  der Nutzer die Ursache erfährt. B7 = Preview robust + Fehler sichtbar + bestätigte
  Nicht-CDN-Ursache; das CDN-Bündeln ist B7b.
- **AC5 — Keine Regression, keine neuen Konsolenfehler.** Im „CDN erreichbar / eingeloggt /
  Desktop"-Normalfall ist das Verhalten unverändert (Panel/Original-Tabs, Object-URL-Bild,
  Revoke beim Schließen). Null neu eingeführte JS-Konsolenfehler in allen Matrixzellen.
  `go vet`/`gofmt`/`go test ./...` bleiben grün.

## Non-Goals
- Keine Neufassung der gesamten Preview-/Render-Pipeline (`services/PreviewService`,
  `handlers/preview.go` Render-Logik bleiben unberührt außer nötigem Status-Mapping).
- Keine Änderung am Panel-Renderer / an der Quantisierung/Kalibrierung.
- Keine Änderung am E5.1-Auth-Modell (Sessions, CSRF-Origin-Regel, `/login`-Redirect,
  `clientRoutes`/`pageRoutes`).
- Kein CDN-Bündeln von fabric.js / Google Fonts in B7 (→ eigener Spec B7b, siehe AC4).
- Kein Redesign des Preview-Modals oder des responsiven Burger-Menüs.

## Verifikation

### Gate L1 — Statik + Tests (lokal, Mac)
- `cd server && gofmt -l .` (leer) für jede Go-seitige Änderung (Status-Mapping).
- `cd server && go vet ./...`
- `cd server && go test ./...` — inkl. des neuen Regressionstests aus AC3.
- JS hat KEINEN Build-Step → keine JS-Unit-Gate; die JS-Änderungen (`toolbar.js`,
  ggf. `designer.js`/`designer.html`) werden über L2 belegt.

### Gate L2 — Reproduktions-/Verifikations-Protokoll (Browser, läuft auf dem Mac)
- Die vollständige AC0-Matrix (Auth × Viewport × CDN) als Vorher-Nachher: pro Zelle
  Screenshot + Konsolen-Log. „CDN blockiert" wird per DevTools-Request-Blocking bzw.
  Offline-Profil auf `cdnjs.cloudflare.com` (und `fonts.googleapis.com`) hergestellt.
  „Abgelaufene Session" wird durch Löschen/Invalidieren des `eink_session`-Cookies erzeugt.
- Beleg für AC1: In mindestens einer bestätigten Fehlerzelle zeigt das Modal nach dem Fix
  sichtbaren Fehlertext statt leeren Bilds.
- Beleg für AC5: Normalfall Desktop/eingeloggt/CDN-erreichbar rendert Panel + Original
  unverändert, Konsole fehlerfrei.
- Konkrete Befehle zum Hochfahren: `cd server && go run .` (Dev-Server), Browser gegen die
  lokale URL; die Matrix wird headless/DevTools erfasst.

### Gate L5 — Review
- Review bestätigt: (a) AC0-Matrix ist ausgefüllt und die adressierte Ursache deckt sich mit
  dem, was real gefeuert hat; (b) `onerror` feuert nicht im Erfolgsfall und bricht die
  Object-URL-Revoke-Logik nicht; (c) 403 vs 401 werden clientseitig korrekt unterschieden;
  (d) das E5.1-Verhalten ist unangetastet; (e) falls [1] bestätigt, ist B7b als separater
  Spec vermerkt und NICHT mit-implementiert.

### Gate L3 — nicht erforderlich
Der Bug und der Fix sind browserseitig und werden auf dem Mac verifiziert; keine Panel-/
Hardware-Interaktion. (B7b, falls angelegt, hätte einen eigenen Offline-Pi-Nachweis.)

## Risiken
- **`onerror`-Schleife / Doppel-Feuern:** Ein `onerror`, der selbst wieder eine (fehlerhafte)
  `src` setzt, könnte loopen. Rollback/Mitigation: `onerror` setzt nur Status-Text, ändert
  KEINE `src`; einmaliges Feuern pro Fehl-Load. Prüfen, dass der Erfolgsfall (Object-URL)
  `onerror` nicht auslöst.
- **Fehlklassifikation 403 vs. „unavailable":** Wird der 403-Zweig zu breit gefasst, könnte
  ein legitimer 503-„Renderer busy"-Fall (`toolbar.js:225-230`) falsch gelabelt werden.
  Mitigation: Nur exakt Status 403 auf die CSRF-Meldung mappen, restliches Verhalten
  unverändert lassen.
- **Scope-Creep in B7b:** Versuchung, fabric lokal zu bündeln „weil man gerade dran ist".
  Mitigation/Rollback: AC4 verbietet das explizit; falls [1] bestätigt, entsteht ein
  separater Spec, B7 bleibt auf sichtbaren Fehler + Nicht-CDN-Ursache begrenzt.
- **Regression im Normalfall:** Änderungen am `catch`-Zweig könnten den etablierten
  Saved-Design-Fallback (`toolbar.js:217-224`) brechen. Mitigation: L2 belegt den
  unveränderten Normalfall (AC5); Rollback = Revert des `toolbar.js`-Diffs, Go-Seite
  (nur additive Status-Assertions/Tests) bleibt.
- **Falsche Ursachen-Annahme:** Ohne AC0 könnte am falschen Mechanismus „gefixt" werden.
  Mitigation: AC0 ist blockierend („Reproduktion ZUERST"); der Fix folgt dem Protokoll.
