# EPD ICS Calendar – Development Progress

_Last updated: 2025-12-17_

## 1. Project Overview

Target: Single Go daemon for Raspberry Pi that:

- Drives Waveshare 12.48" tri-color e-paper (B) panel (1304x984).
- Subscribes to one or more ICS (iCalendar) URLs (no OAuth / Google APIs).
- Expands events with robust timezone and recurrence handling.
- Renders a simple multi-day calendar view.
- Exposes a local Web UI for configuration and manual refresh.
- Runs as a systemd service with configuration and caches on disk.

Planned repository layout (relative to project root):

- [`cmd/epdcal/main.go`](cmd/epdcal/main.go) – CLI entrypoint and daemon startup.
- [`internal/config/config.go`](internal/config/config.go) – Config struct, load/save, defaults, validation.
- [`internal/web/web.go`](internal/web/web.go) – HTTP server, HTML UI, JSON API, basic auth.
- [`internal/ics/fetch.go`](internal/ics/fetch.go) – ICS download, HTTP caching (ETag / Last-Modified).
- [`internal/ics/parse.go`](internal/ics/parse.go) – ICS parsing into internal event model (including VTIMEZONE).
- [`internal/ics/expand.go`](internal/ics/expand.go) – Recurrence expansion, EXDATE, RECURRENCE-ID handling.
- [`internal/model/model.go`](internal/model/model.go) – Core domain structs (events, occurrences, config snapshot).
- [`internal/render/render.go`](internal/render/render.go) – Rendering calendar view into `image.NRGBA`.
- [`internal/convert/pack.go`](internal/convert/pack.go) – Convert NRGBA into 1bpp black/red packed planes.
- [`internal/epd/epd.go`](internal/epd/epd.go) – High-level display API and "render-only"/"once" modes.
- [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go) – cgo bindings to Waveshare C driver.
- [`waveshare/`](waveshare/) – Vendored C driver sources/headers for 12.48" B panel.
- [`systemd/epdcal.service`](systemd/epdcal.service) – systemd unit file.
- [`README.md`](README.md) – Main documentation.
- [`progress.md`](progress.md) – This progress and design tracking document.
- [`Makefile`](Makefile) – Build/test/install automation.

## 2. Major Workstreams & Status

**Legend**

- [ ] Not started
- [-] In progress
- [x] Done

### 2.1 Project Bootstrapping

- [x] Initialize Go modules and base `cmd/epdcal` skeleton.
  - [`go.mod`](go.mod) with `module epdcal` and `go 1.25.5`.
  - [`cmd/epdcal/main.go`](cmd/epdcal/main.go) created as main entrypoint.
- [x] Add minimal logging and signal handling (graceful shutdown).
  - [`internal/log/log.go`](internal/log/log.go) implements leveled logger (DEBUG/INFO/ERROR) to stderr.
  - `main()` uses context + SIGINT/SIGTERM handling and logs start/exit.
- [x] Define global configuration & state wiring.
  - [`internal/config/config.go`](internal/config/config.go) defines `Config` / `ICSConfig` / `BasicAuthConfig`.
  - `cmd/epdcal/main.go` loads config via `config.Load`, applies `--listen` override, logs effective config.

### 2.2 Configuration & Persistence

- [x] Implement YAML config loader/saver in [`internal/config/config.go`](internal/config/config.go).
  - Uses `gopkg.in/yaml.v3` for marshaling.
  - `Load(path)`:
    - If file does not exist:
      - Creates parent directory if needed (0700).
      - Writes `DefaultConfig()` with `Save` (0600 perms).
      - Returns the default config.
    - If file exists:
      - Reads YAML, unmarshals into `Config`, then `Normalize()` to fill defaults.
  - `Save(path, cfg)`:
    - Ensures parent directory exists (0700).
    - Normalizes config.
    - Marshals to YAML.
    - Writes atomically via temp file + `os.Rename`.
    - Ensures final file permissions are 0600.
  - `(*Config).Save(path)` convenience method wraps `Save(path, c)`.
- [x] Ensure first-run behavior:
  - When the config file is missing, `Load(path)`:
    - Instantiates `DefaultConfig()`.
    - Persists it immediately via `Save(path, cfg)` with 0600 permission.
    - Returns the config so the daemon and Web UI can start using defaults.
- [x] Design config schema:
  - Schema modeled in `Config`:
    - `Listen`, `Timezone`, `RefreshMinutes`, `HorizonDays`, `ShowAllDay`, `HighlightRed`, `ICS`, `BasicAuth`.
  - `Normalize()` ensures backwards compatibility / sane defaults even if some fields are missing in YAML.
- [ ] Implement runtime cache directory under `/var/lib/epdcal/`:
  - Per-ICS HTTP cache metadata (ETag, Last-Modified).
  - Last rendered image buffers and/or PNG preview.

### 2.3 ICS Fetching & HTTP Caching

- [ ] Implement ICS fetcher in [`internal/ics/fetch.go`](internal/ics/fetch.go):
  - Periodic fetch based on refresh interval.
  - On-demand fetch via Web UI (`/api/refresh`).
  - HTTP conditional requests:
    - Store ETag and Last-Modified per URL.
    - Set `If-None-Match` / `If-Modified-Since` when available.
    - Treat 304 as "no-change" and reuse cached ICS body.
- [ ] Add redaction utilities so logs never print full ICS URLs.
- [ ] Error-handling:
  - Errors must not crash daemon.
  - On fetch failure, keep previous successful ICS data and last rendered image.

### 2.4 ICS Parsing, Timezones, & Model

- [ ] Select and vendor an ICS parsing library in [`internal/ics/parse.go`](internal/ics/parse.go) (or thin wrapper).
- [ ] Parse:
  - VCALENDAR/VEVENT/VTIMEZONE.
  - DTSTART/DTEND (DATE and DATE-TIME).
  - RRULE, EXDATE, RDATE, RECURRENCE-ID, UID.
- [ ] Build an internal event model in [`internal/model/model.go`](internal/model/model.go):
  - Calendar ID / source URL tag.
  - UID and recurrence instance key.
  - All-day vs timed events.
  - Original timezone.

### 2.5 Recurrence Expansion & Exceptions

- [ ] Implement recurrence expansion in [`internal/ics/expand.go`](internal/ics/expand.go) using a library such as `rrule-go` or a minimal custom implementation:
  - RRULE basic support: FREQ=DAILY/WEEKLY/MONTHLY/YEARLY.
  - Common modifiers: BYDAY, BYMONTHDAY, INTERVAL, COUNT, UNTIL.
- [ ] Handle exceptions and overrides:
  - `EXDATE` removal of generated occurrences.
  - `RECURRENCE-ID` override of specific instances:
    - Collect override VEVENTs keyed by (UID, recurrence-id timestamp).
    - Replace corresponding base occurrence with override details.
- [ ] All-day event semantics:
  - DATE values mapped to local date range `[00:00, next day 00:00)` in display timezone.
- [ ] Timezone normalization:
  - Use `config.Timezone` as canonical display zone.
  - Interpret:
    - `DTSTART;TZID=...` using ICS VTIMEZONE when possible; otherwise IANA mapping or system tz.
    - `DTSTART:...Z` as UTC.
    - Floating times (no TZ) as local timezone.
  - Convert all occurrences into display timezone before grouping per day.
- [ ] Expansion range strategy:
  - Expand only in `[now - backfill, now + horizon]` (e.g., backfill 1 day, horizon N days).
  - Hard cap occurrences per event (e.g., 5000) with logging.

### 2.6 Deduplication & Merge

- [ ] Merge events from multiple ICS URLs:
  - Dedup key: (calendarID/url, UID, recurrence-instance key).
  - Ensure no duplicates when same ICS is accidentally added twice.

### 2.7 Unit Tests for ICS/TZ Logic

- [ ] Add test fixtures under `internal/ics/testdata/`:
  - Simple single event.
  - Weekly recurring event with EXDATE.
  - Recurring event with a single overridden instance via RECURRENCE-ID.
  - Mix of:
    - TZID-based event.
    - UTC event.
    - All-day event.
- [ ] Implement table-driven tests in:
  - [`internal/ics/parse_test.go`](internal/ics/parse_test.go) – parsing correctness.
  - [`internal/ics/expand_test.go`](internal/ics/expand_test.go) – expanded occurrence timestamps.
- [ ] Validate, per configured display timezone:
  - Correct local start/end times.
  - Correct application of EXDATE and overrides.
  - No duplicates for removed/overridden instances.

### 2.8 Rendering Pipeline (Image)

- [ ] Design basic layout in [`internal/render/render.go`](internal/render/render.go):
  - Current date and day-of-week.
  - Upcoming events for next N days (grouped by date).
  - Distinct all-day section (optional).
- [ ] Implement text rendering:
  - Use `image.NRGBA` canvas sized 1304x984.
  - Use `x/image/font/opentype` with bundled fonts for Korean/Latin support.
- [ ] Apply color rules:
  - Red for:
    - Events matching configured highlight keywords.
    - Weekends and optionally holidays (future extension).
- [ ] Provide debug `--dump` mode:
  - Emit `black.bin`, `red.bin`, and `preview.png` into working or cache directory.

### 2.9 Packed Plane Conversion

- [ ] Implement 1bpp packing in [`internal/convert/pack.go`](internal/convert/pack.go):
  - Dimensions: 1304x984, stride 163 bytes (1304 / 8).
  - Packed MSB-first:
    - `byteIndex = y*163 + (x >> 3)`.
    - `mask = 0x80 >> (x & 7)`.
  - Initialize both planes with `0xFF` (white).
  - For a black pixel: clear bit in BlackImage plane.
  - For a red pixel: clear bit in RedImage plane (C code will invert bits on transmit).
- [ ] Ensure single-pass conversion from `*image.NRGBA`:
  - Avoid `At()` per pixel; access backing slice directly.

### 2.10 EPD Hardware Integration

- [ ] Vendor Waveshare C driver under [`waveshare/`](waveshare/).
- [ ] Add cgo bindings in [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go) to:
  - `EPD_12in48B_Init`.
  - `EPD_12in48B_Clear`.
  - `EPD_12in48B_Display`.
  - `EPD_12in48B_TurnOnDisplay`.
  - `EPD_12in48B_Sleep`.
- [ ] Implement high-level wrapper in [`internal/epd/epd.go`](internal/epd/epd.go):
  - Initialize once and reuse where possible.
  - Optionally clear before first display.
  - Support "render-only" mode (no hardware calls).
  - Respect `--once` and `--render-only` flags.
- [ ] Add optional `--dump` logic around display pipeline.

### 2.11 Web UI & API

- [ ] Implement HTTP server in [`internal/web/web.go`](internal/web/web.go):
  - Bind address from config / `--listen` (default `127.0.0.1:8080`).
  - Optional basic-auth; protect all endpoints except `/health`.
- [ ] Endpoints:
  - `GET /` – settings + status HTML UI.
  - `GET /api/config` – return current config as JSON.
  - `POST /api/config` – update config (with validation and persistence using `config.Save`).
  - `POST /api/refresh` – trigger fetch + render + display.
  - `POST /api/render` – trigger fetch + render (no display).
  - `GET /preview.png` – serve last rendered preview.
  - `GET /health` – simple OK response.
- [ ] UI Features:
  - Manage ICS URLs (add/remove).
  - Set refresh interval, timezone, horizon days.
  - Toggle all-day section, define red highlight keywords.
  - Show last refresh time, next scheduled refresh time, last error summary.

### 2.12 Scheduler & Daemon Behavior

- [ ] Implement scheduler loop in [`cmd/epdcal/main.go`](cmd/epdcal/main.go):
  - Periodic refresh based on config.
  - Respect manual triggers and recompute "next refresh".
  - Clean shutdown on SIGTERM / SIGINT.
- [ ] Ensure that:
  - Fetch/render errors do not kill the process.
  - Last successful image is preserved and continues to be used.

### 2.13 systemd & Deployment

- [ ] Create systemd unit file at [`systemd/epdcal.service`](systemd/epdcal.service):
  - Run as dedicated user (recommended).
  - Configure working directory, environment, and restart policy.
  - Ensure access to `/etc/epdcal/config.yaml` and `/var/lib/epdcal/`.
- [ ] Document installation steps in [`README.md`](README.md):
  - Build for Raspberry Pi (GOOS=linux, GOARCH=arm/arm64).
  - Deploy binary, config, and systemd unit.
  - Enable and start service.

## 3. Known Technical Priorities

1. **Correctness of TZ and recurrence logic** (ICS handling).
2. **Deterministic rendering** across runs and across devices.
3. **Robustness**: daemon must survive network and ICS parse errors.
4. **Performance**:
   - Limit expansion to required time window.
   - Avoid per-pixel `At()` in rendering and packing.
5. **Security & Privacy**:
   - No OAuth, no external account linkage.
   - Never log full ICS URLs or event sensitive details when not required.
   - Optional basic auth for Web UI; bound to `127.0.0.1` by default.

## 4. Next Immediate Steps

Planned next sequence (post-2.2 config persistence):

1. Add basic HTTP server stub in [`internal/web/web.go`](internal/web/web.go) with `/health` and `/` placeholder page, using `Config.Listen` and optional BasicAuth (stub).
2. Integrate ICS fetching and caching skeleton in [`internal/ics/fetch.go`](internal/ics/fetch.go) with logging-only usage and placeholder in-memory cache.
3. Add ICS parsing and minimal one-off event expansion path in [`internal/ics/parse.go`](internal/ics/parse.go) and [`internal/ics/expand.go`](internal/ics/expand.go), plus first unit tests + testdata fixtures.
4. Design and implement minimal text-only rendering in [`internal/render/render.go`](internal/render/render.go) and NRGBA->packed planes in [`internal/convert/pack.go`](internal/convert/pack.go).
5. Wire up EPD integration in [`internal/epd/epd.go`](internal/epd/epd.go) and [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go) with `--render-only` support for development without hardware.
6. Flesh out Web UI endpoints (`/api/config`, `/api/render`, `/api/refresh`, `/preview.png`) and hook them into the core pipeline.
7. Implement runtime cache directory usage under `/var/lib/epdcal/` for ICS HTTP metadata and rendered previews.

This document should be updated as tasks are completed or requirements evolve.