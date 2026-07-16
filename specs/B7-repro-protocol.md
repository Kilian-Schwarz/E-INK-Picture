# B7 — AC0 Reproduction Protocol (blocking gate, DONE 2026-07-16)

Live reproduction of the "designer Preview button shows nothing" bug, per specs/B7-designer-preview.md AC0.
Read-only run (no project code touched, no branch, no commit). This document is the L2 evidence for B7
(JS has no unit-test build step, so the browser/curl protocol is the record).

## 1. Endpoint status-code matrix (curl) — this is the AC3 regression-test contract

| Request | Code | Content-Type | Note |
|---|---|---|---|
| `POST /api/preview_live` — valid session + **same-origin** Origin | **200** | `image/png` | valid PNG 800×480 |
| `POST /api/preview_live` — valid session + **foreign** Origin | **403** | `application/json` | `{"message":"cross-origin request rejected"}` — NOT 401 |
| `POST /api/preview_live` — **no** session | **401** | `application/json` | `{"message":"authentication required"}` |
| `POST /api/preview_live` — **invalid/expired** session | **401** | `application/json` | same |
| `GET /preview` — valid session | **200** | `image/png` | valid PNG |
| `GET /preview` — no session, no token | **401** | `application/json` | — |
| `GET /preview` — `X-Client-Token` header | **200** | `image/png` | client route accepts token |

## 2. AC0 matrix (auth × viewport × CDN)

Viewport caveat (honest): the Chrome window could not be shrunk below ~1710 CSS px live (`resize_window`
+ synthetic zoom keys had no effect on the CSS viewport). Only **Desktop** was achievable in-browser;
Tablet/Mobile media-query cells were confirmed via the app's own code + CSS instead.

| Auth | Viewport | CDN | Modal? | Image? | Console | Mechanism |
|---|---|---|---|---|---|---|
| no-password | Desktop | reachable | Yes | blob PNG (empty default design) | clean, 0 errors | AC5 normal case PASS |
| no-password | Desktop | **blocked** | **No — click inert** | none | `ReferenceError: fabric is not defined at Object.init (canvas-manager.js:15:27) at designer.js:46:19` | **[1]** |
| logged-in fresh | Desktop | reachable | OK | — | clean | normal path |
| logged-in **EXPIRED** | Desktop | reachable | **No — redirect to /login** | — | (redirect) | **[2]** |
| any | Tablet/Mobile | any | NOT RUN (viewport) | — | — | burger relocation confirmed by code+CSS |

Mechanism [1] method: fetched `/designer`, stripped only the fabric CDN `<script>`, loaded in a same-origin
`srcdoc` iframe (fresh realm, real static scripts, real load order, fabric genuinely absent) → exact
hypothesized stack trace; a direct `.click()` on `#preview-btn` left `#preview-modal` at `display:none`
(inert) because `Toolbar.init` (designer.js:52) never ran after `CanvasManager.init` threw at designer.js:46.

Burger-hiding: `ResponsiveLayout.MENU_SELECTORS_TABLET`/`_MOBILE` include `#preview-btn`; `responsive.css:19-58`
keeps `#topbar-menu` `display:none` except under `@media (max-width:1569.98px)` + `body.topbar-menu-open`.
→ button hidden until burger opened. By design, NOT a bug.

## 3. Verdict — mechanisms that actually FIRE

- **[1] CDN offline → fabric undefined → init throws before Toolbar.init → click inert: CONFIRMED (definitive).** Core offline/LAN-only Pi bug.
- **[2] Expired session → 401 → auth.js `/login` redirect: CONFIRMED** (live + curl).
- **[3] Cross-origin 403: server-side CONFIRMED via curl** (403, not 401). Browser can't forge a foreign `Origin`, so this only manifests behind a real reverse proxy. Per code a 403 is not caught by auth.js (it only handles 401) → toolbar.js `throw` → `GET /preview?name=` fallback → stale saved design labeled "unavailable" (not literally "nothing").
- **[4] PreviewLive 500 → fallback: code-confirmed** (`preview.go:78`), not independently triggered.
- **[5] `#preview-image` has no `onerror` → blank image on failing fallback: CONFIRMED live** (`onerror===null`; designer.html:290). This is the AC1 gap.
- **Burger-hiding: CONFIRMED, by design.**

## 4. Fix requirements (input for the frontend-designer implementer)

- **AC1 (core, cause-independent):** add `onerror` to `#preview-image` → surface a named error via existing `setPreviewStatus()` → `#preview-status` ("Preview konnte nicht geladen werden"). Must NOT fire on the successful object-URL, must NOT disturb object-URL revoke or session logic.
- **[1] visible init error:** detect `typeof fabric === 'undefined'` early → named message ("Designer nicht initialisiert — offline/CDN") instead of a dead click.
- **[3] 403 handling:** `toolbar.js` catch (≈215–230) must distinguish exactly status 403 → cross-origin-specific message; keep the 503 "renderer busy" branch untouched; do not funnel 403 into the generic saved-design "unavailable" fallback.
- **[4]/[5]:** any fallback response that is non-2xx or non-`image/*` → route to the AC1 visible error, not a blank `<img>`.
- **[2]:** current `/login` redirect is correct E5.1 behavior — document only, no change.

## 5. B7b scope flag

Mechanism [1] is the confirmed core cause. The real remedy — bundle `fabric.js` + Google Fonts locally and
serve via `go:embed` from `server/static/` — is a larger change and MUST be a **separate spec (B7b /
offline-designer-assets)**, per AC4. It is NOT implemented inside B7. B7's minimum for [1] is the early
visible "fabric undefined" error so an offline user learns the cause instead of a dead click.
Note: this only bites a truly air-gapped LAN (both Pi and the viewing browser offline); if the phone/laptop
has internet, fabric still loads from CDN.
