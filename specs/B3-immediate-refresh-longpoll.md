# B3: Refresh Display triggers the panel in ~2s via long-polling

## Ziel
After the user clicks "Refresh Display" (`POST /api/trigger_refresh`), the E-Ink client begins its panel refresh within ~2s instead of the current worst-case ~30s / average ~15s — achieved by turning `GET /api/refresh_status` into a long-poll that the server answers immediately when a trigger fires, without changing any autonomous refresh cadence or the content-skip semantics.

## Kontext

### Grounded root cause (verified, do not re-derive)
The delay is pure pull-poll latency, not server-side processing:
- The trigger is instant server-side. `POST /api/trigger_refresh` → `SettingsHandler.TriggerRefresh` (`server/internal/handlers/settings.go:142-153`) → `SettingsService.TriggerRefresh` (`server/internal/services/settings.go:290-313`) writes `settings.LastRefreshTrigger = time.Now().UTC()` to `settings.json` under `s.mu.Lock()` and returns immediately.
- `GetRefreshStatus` (`server/internal/services/settings.go:340-402`) already returns `should_refresh=true`, `reason="manual"` (`models.RefreshReasonManual`, `server/internal/models/settings.go:32`) as soon as `LastRefreshTrigger` is newer than `LastClientRefresh`.
- The ONLY latency is the client's poll gap. The client sleeps `POLL_INTERVAL` seconds (`client/config.py:7`, default 30) one second at a time, then runs exactly one `process_refresh_cycle()` (`client/client.py:508-517`). Worst case = a full 30s until the next poll observes the trigger.

### Chosen fix: long-polling on the existing poll path
Server holds the `GET /api/refresh_status` request open and returns the moment a trigger fires; otherwise it returns `should_refresh=false` after a bounded hold and the client immediately re-polls. No new endpoint, no push path.

### Affected files (implementer must edit these, following the noted patterns)
- `server/internal/services/settings.go` — add a broadcast notify signal to `SettingsService`; add a blocking wait method; `TriggerRefresh` must fire the signal AFTER the timestamp is durably written.
- `server/internal/handlers/settings.go` — `RefreshStatus` handler switches to the blocking wait, passing `r.Context()` and the server hold cap.
- `server/internal/services/settings_test.go` — new unit tests (existing patterns: `t.TempDir()`, `NewSettingsService(dir, models.DisplayWaveshare75V2)`, injectable `now` field at `settings.go:24`).
- `client/client.py` — replace the fixed pre-poll sleep loop (`508-517`) / `get_refresh_status()` timeout (`308-318`) with a blocking long-poll plus bounded reconnect backoff.
- `client/config.py` — add the client long-poll read timeout knob.
- `client/test_client.py` — new tests (existing pattern: `unittest`, `unittest.mock.patch`, mocked `requests`).
- `.env.example` — document the new client env var next to `EINK_POLL_INTERVAL` (line 57).

### Existing patterns the implementer MUST follow
- Go concurrency: standard "closed-channel broadcast" — a `chan struct{}` field guarded by its OWN small mutex (NOT `s.mu`); waiters grab the current channel reference, then `select`; `TriggerRefresh` does `close(ch); ch = make(chan struct{})` to wake all waiters.
- Test-time clock injection already exists (`now func() time.Time`, `settings.go:24,40`); mirror it for the hold duration by making the wait method take `maxWait time.Duration` as a parameter (handler passes the const, tests pass a small value) — no real-time sleeps in unit tests.
- `models.RefreshStatus` is the unchanged response shape; the blocking method returns the same struct.
- The B6 content-skip predicate `_should_skip_panel_write` (`client/client.py:339-359`) and specifically its guard `if reason != "interval": return False` (`client/client.py:351`) is verified-complete and MUST NOT be edited; `reason` must keep flowing from the status response through `process_refresh_cycle` → `handle_refresh`.

## Akzeptanzkriterien

### Server — notify + blocking wait
1. `SettingsService` gains a broadcast notify signal (a `chan struct{}` field with a dedicated `sync.Mutex`, initialized in `NewSettingsService`). It is NOT `s.mu`.
2. A new method exists with signature equivalent to `WaitForRefresh(ctx context.Context, maxWait time.Duration) (*models.RefreshStatus, error)` that:
   a. First evaluates `GetRefreshStatus()`; if `ShouldRefresh` is already true, returns immediately (covers interval refreshes and a trigger that fired before parking).
   b. Otherwise parks in a single `select` on `{notify channel, time.After(maxWait), ctx.Done()}`.
   c. On the notify channel firing, re-evaluates `GetRefreshStatus()` and returns the fresh result.
   d. On `maxWait` elapsing OR `ctx.Done()`, returns a fresh `GetRefreshStatus()` result (normally `should_refresh=false`); a canceled context must not be reported as a 500.
3. The channel reference is captured BEFORE the step-2a check (lost-wakeup-safe): a trigger firing between the check and the park still wakes the waiter.
4. While parked, the method holds NEITHER `s.mu` NOR the notify mutex (the notify mutex is held only to read/swap the channel reference, never across the `select`).
5. `TriggerRefresh` fires the notify broadcast ONLY AFTER `os.WriteFile` of the new `LastRefreshTrigger` succeeds (a woken waiter's `GetRefreshStatus()` must read the persisted trigger). Broadcast = `close(current); current = make(chan struct{})` under the notify mutex.
6. Unit test: a parked `WaitForRefresh` (started in a goroutine, `maxWait` well above the assertion window, e.g. 5s) is woken by a concurrent `TriggerRefresh()` and returns `ShouldRefresh=true`, `Reason="manual"` in **< 250 ms** (assert on wall-clock delta, not the full `maxWait`).
7. Unit test: with no trigger and no elapsed interval, `WaitForRefresh(ctx, maxWait)` with a small `maxWait` (e.g. 50 ms) returns `ShouldRefresh=false` after ≥ `maxWait` and well under `maxWait+250ms`.
8. Unit test: a canceled `ctx` (context canceled before/while parked) causes `WaitForRefresh` to return promptly without panicking and without reporting a server error.
9. Concurrency isolation: a test parks N (≥ 3) `WaitForRefresh` calls and asserts `PreviewService.ActiveRenders()` stays 0 and a concurrent `PreviewService.Render(...)` completes normally — long-polls do not consume `renderSem` capacity. (Structural backstop: `SettingsHandler`/`SettingsService` hold no reference to `PreviewService`, so the path cannot touch `renderSem` — assert this holds in review.)

### Server — handler + timeout coordination
10. `SettingsHandler.RefreshStatus` calls the blocking wait with `r.Context()` and a package-level const `serverHoldTimeout` set to **25s** (strictly below `http.Server.WriteTimeout = 30s`, `server/main.go:318`). The const carries a comment tying it to `WriteTimeout` so a future change is caught.
11. The hold never exceeds `serverHoldTimeout`; on timeout the handler returns HTTP 200 with `should_refresh=false` (never a 5xx, never a truncated response from the `WriteTimeout` killing the connection).
12. The dead push handler (`DisplayHandler.RefreshDisplay`, `server/internal/handlers/display.go`; route `POST /refresh-display`, `server/main.go:224`; gated on empty `EINK_CLIENT_URL`) is left untouched — the fix is on the poll path only.

### Client — blocking long-poll + reconnect
13. `client/config.py` gains `LONGPOLL_TIMEOUT = int(os.getenv("EINK_LONGPOLL_TIMEOUT", "30"))` — the read timeout for the status poll, chosen strictly greater than the server hold (25s) so the server responds before the client times out.
14. `get_refresh_status()` (`client/client.py:308-318`) issues the status GET with a read timeout of `config.LONGPOLL_TIMEOUT` (recommended `timeout=(5, config.LONGPOLL_TIMEOUT)` — 5s connect, long read) instead of the current fixed `timeout=5`.
15. The main poll loop no longer pre-sleeps `POLL_INTERVAL` seconds before polling: the old `for _ in range(poll_interval): time.sleep(1)` block (`client/client.py:509-512`) is removed. On a successful poll response the client processes the result and immediately re-polls (the server hold provides the pacing).
16. On a poll timeout OR network error (`requests.Timeout`, `requests.ConnectionError`, or any request exception), the client waits a bounded reconnect backoff of `POLL_INTERVAL` seconds (still the env-tunable `EINK_POLL_INTERVAL`) before re-polling — it MUST NOT busy-loop. `POLL_INTERVAL` is retained as the reconnect/backoff cadence, not a happy-path delay.
17. `reason` threading is unchanged: `process_refresh_cycle` (`client/client.py:428-447`) still passes `status.get("reason")` into `handle_refresh`; the B6 guard at `client/client.py:351` is byte-for-byte unchanged (the diff touches no line inside `_should_skip_panel_write`).
18. The initial-display retry path (`_initial_display_done is False` branch, `client/client.py:438-442`) still runs on every cycle until the first successful write, unchanged in behavior.
19. Client unit test (mocked `requests`): a status response with `should_refresh=true, reason="manual"` triggers exactly one `handle_refresh` with `reason="manual"` and does NOT skip (manual is never content-skipped).
20. Client unit test (mocked `requests`): a `requests.ConnectionError`/`Timeout` from the poll makes the loop back off (assert `time.sleep` called with the backoff, or that the poll is not re-issued in a tight loop) rather than spinning.

### Documentation
21. `.env.example` documents `EINK_LONGPOLL_TIMEOUT=30` with a one-line comment, placed in the client section near `EINK_POLL_INTERVAL` (line 57). No real secrets/values beyond the default.

### End-to-end latency (deferred to L3, Pi offline)
22. On the Pi: button-click (`POST /api/trigger_refresh`) → client observes the trigger and `epd.init()` / panel refresh begins in **< 2s**, measured from server trigger-log timestamp to client "Initializing display..." log timestamp (`client/client.py:255`). MARKED DEFERRED — requires the Pi back online; record the measured delta in the PR when run.

## Non-Goals
- No SSE and no WebSocket — overkill for a single Pi client; long-poll on the existing endpoint is sufficient.
- Do NOT revive or wire the push path (`/refresh-display` → client HTTP listener). `EINK_CLIENT_URL` stays default-empty; no client-side HTTP server is added.
- No change to autonomous cadence: interval refreshes still fire at most once per `RefreshInterval` (the `now.Sub(clientTime) > interval` gate in `GetRefreshStatus`, `settings.go:378`, is untouched). A tighter effective poll cadence (≤25s vs 30s) MUST NOT increase how often interval/`reason="interval"` refreshes actually hit the panel.
- No change to the B6 content-skip predicate or its `reason != "interval"` guard (`client/client.py:351`). "Immediate" shortens latency for USER (`reason="manual"`) triggers only.
- No change to `http.Server` timeouts in `server/main.go` (the hold is capped below the existing `WriteTimeout` instead of raising it).
- No change to the auth/guard middleware or the `X-Client-Token` flow.

## Verifikation

### L1 — local static + unit (required, run before commit)
- `cd server && gofmt -l .` → expect empty output.
- `cd server && go vet ./...` → clean.
- `cd server && go test ./...` → all pass, including the new AC6–AC9 tests in `settings_test.go`.
- `cd client && python3 -m py_compile client.py config.py` → no errors.
- `cd client && python3 -m unittest test_client -v` → all pass, including new AC19–AC20 tests; existing E5.2/E5.4 tests still green (proves B6 guard + recovery untouched).

### L3 — on-Pi hardware integration (DEFERRED — Pi offline)
- Deploy, click "Refresh Display" in the UI, correlate server trigger timestamp with the client "Initializing display..." log line; assert < 2s (AC22). Record the measured delta in the PR when the Pi is back online.

## Risiken
- **Hold exceeds `WriteTimeout` → truncated/killed response.** Mitigated by `serverHoldTimeout = 25s` < `WriteTimeout = 30s` (AC10) with a linking comment. Rollback: revert the handler to the immediate `GetRefreshStatus()` call — the service method and notify signal are inert if unused.
- **Lost wakeup (trigger between check and park) → user waits the full 25s.** Mitigated by capturing the channel reference before the initial status check (AC3) and by write-then-notify ordering (AC5); AC6 proves the wake path. Not silent data loss — worst case degrades to a 25s bound, still better than today's 30s.
- **Long-polls starving goroutines / blocking the renderer.** A parked long-poll is only a cheap `net/http` goroutine and never touches `renderSem` (AC9, structural backstop). Rollback is the handler revert above.
- **Client busy-loop on a down server (no more 30s pre-sleep).** Mitigated by the bounded reconnect backoff (AC16, AC20). Rollback: restore the pre-poll sleep loop and the `timeout=5` on the status GET.
- **Client read timeout below the server hold → reconnect churn.** Not a correctness bug (next poll re-parks, no missed trigger) but wasteful; default `EINK_LONGPOLL_TIMEOUT=30 > 25` avoids it (AC13); documented in `.env.example` (AC21).
- **Rollback (whole feature):** the change is confined to the two server files, two client files, `config.py`, and `.env.example`; reverting the commit restores the 30s-poll behavior with no schema/state migration (`settings.json` shape is unchanged; the notify signal is in-memory only).
