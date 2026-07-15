# E4a — Remove dead widget code + interfaces (safe cleanup, no behavior change)

## Context
The E4 scoping recon (2026-07-15) found the panel's live data widgets all work
end-to-end via `server/internal/services/preview.go` (`fill*Content`). Alongside them
sits a parallel, partly-dead second implementation. This task removes ONLY the provably
dead code so the codebase is honest before v1.0 — **zero behavior change** on the panel
and in the designer editor.

## In scope (remove — confirmed unreferenced)
1. **`server/internal/services/conditions.go`** — `ConditionService`/`NewConditionService`
   and `EvaluateElementConditions`/`EvaluateDesignRules`. Never instantiated (no call site
   in `main.go` or any handler; the render loop only checks `elem.Visible`). Remove the
   file and any now-dead helper it solely used. (The frontend `static/js/conditions.js`
   is a SEPARATE concern and is OUT of scope — do not touch it unless it is also provably
   unreferenced, which recon did not establish; leave it.)
2. **`server/internal/services/widgets/renderer.go`** — the `WidgetRenderer` interface and
   the `RenderContext`/`WidgetData` structs, plus every widget's stub `Render(...) error`
   method (returns `nil`, never called). Remove the interface and all its stub methods.
3. **Orphaned widget structs** in the `widgets/` package whose constructors are never
   called from non-test code: `NewWeatherWidget`/`WeatherWidget` (`widgets/weather.go`),
   `NewForecastWidget`/`ForecastWidget` (`widgets/forecast.go`),
   `NewClockWidget` (`widgets/clock.go`), `NewTimerWidget` (`widgets/timer.go`). Also the
   `widgets.WeatherFetcher` interface + `widgets.WeatherResult` type (0 implementers).
   VERIFY each has no non-test caller before deleting; delete the tests that only exercise
   the removed code.

## Explicitly OUT of scope (do NOT change here)
- The FOUR wired widgets used by the designer editor preview API (`/api/widgets/*`):
  `CalendarWidget`, `NewsWidget`, `CustomWidget`, `SystemWidget` in `widgets/` and their
  `GetContent`/`GetLayouts`/`Placeholders` — these ARE called by `handlers/widgets.go`.
  Keep them fully working.
- The panel renderer `preview.go` `fill*Content` path — unchanged.
- The editor-vs-panel duplication / divergence, the missing negative cache on the
  editor-preview path, the system-widget-reads-wrong-host issue — these are real but are a
  SEPARATE, larger consolidation effort (post-v1.0 candidate), NOT this cleanup.

## Acceptance criteria
- **AC1** — every symbol removed is proven to have no non-test caller (show the grep). No
  compile reference dangles.
- **AC2** — `gofmt -l` empty, `go vet ./...` clean, `go build ./...` clean, `go test ./...
  -count=1` green including the golden suite and `-race` on services/handlers. The
  designer editor's `/api/widgets/{calendar,news,custom,system}` endpoints still return
  their content (a handler test or a manual `GetContent` test still passes).
- **AC3** — ZERO behavior change: no golden PNG changes, no API response changes for the
  four kept widgets, no route removed that the frontend calls. Confirm `git diff` touches
  only dead files/symbols + their dedicated tests.
- **AC4** — net line reduction reported; the removed files/symbols listed explicitly.

## Verification
- L1: the full static + test matrix above (this task is fully L1-verifiable on the Mac — no
  hardware needed).
- L5: independent reviewer confirms each removed symbol truly had no live caller (adversarial
  grep, including reflection/string references and the frontend's `/api/*` calls) and that
  the four editor widgets + panel path are untouched.

## Non-goals
- No consolidation/refactor of the duplicated widget logic (separate effort).
- No frontend changes.
