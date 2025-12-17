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
  - Custom RFC3339Nano timestamp; stdlib logger flags disabled to avoid duplicate timestamps.
  - `main()` uses context + SIGINT/SIGTERM handling and logs start/exit.
- [x] Define global configuration & state wiring.
  - [`internal/config/config.go`](internal/config/config.go) defines `Config` / `ICSConfig` / `BasicAuthConfig`.
  - `cmd/epdcal/main.go` loads config via `config.Load`, applies `--listen` override, logs effective config.
  - CLI flags (현재 구현됨):
    - `--config`
    - `--listen`
    - `--once`
    - `--render-only` (예약)
    - `--dump` (예약)
    - `--debug` (개발용 로컬 경로 사용)

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

- [x] Implement ICS fetcher in [`internal/ics/fetch.go`](internal/ics/fetch.go).
  - `Fetcher` with `FetchAll` / `FetchOne`:
    - Uses `If-None-Match` / `If-Modified-Since` with ETag / Last-Modified.
    - Caches responses under `/var/lib/epdcal/ics-cache` (또는 상대 경로 fallback) as:
      - `meta.json` (URL, ETag, Last-Modified, UpdatedAt)
      - `body.ics` (ICS 본문)
    - On 304 or network/HTTP errors:
      - If cached body exists, logs error and falls back to cache.
      - If no cache, 에러 반환.
- [x] Add redaction utilities so logs never print full ICS URLs.
  - `redactURL()` masks paths/query, logging only scheme+host and `/(redacted)`.
  - `cmd/epdcal/main.go` 에서는 ID 기반으로만 URL을 간접적으로 표현(`ics://source(id)`).
- [x] Error-handling:
  - Errors from individual sources logged via `log.Error` and aggregated.
  - Network/HTTP errors do not crash the daemon; last good cached ICS is reused where possible.

### 2.4 ICS Parsing, Timezones, & Model

- [x] Select and vendor an ICS parsing library in [`internal/ics/parse.go`](internal/ics/parse.go).
  - Uses `github.com/arran4/golang-ical`.
- [x] Parse:
  - VEVENT 파싱 구현:
    - `UID`, `SEQUENCE`, `SUMMARY`, `DESCRIPTION`, `LOCATION`.
    - `DTSTART` / `DTEND` via `ve.GetStartAt()` / `ve.GetEndAt()` (VTIMEZONE/TZID 해석은 라이브러리 의존).
    - All-day 판별: `VALUE=DATE` 또는 값에 `T` 없음.
    - `TZID` 파라미터에서 `StartTZ` / `EndTZ` 추출.
    - `RRULE` 문자열을 그대로 `RawRRule` 필드에 저장.
    - `EXDATE` (`EXDATE` 프로퍼티들) 를 `parseICSTime` 으로 `[]time.Time` 에 저장.
    - `RECURRENCE-ID` 를 `Recurrence`/`IsOverride` 로 저장.
- [x] Build an internal event model in [`internal/model/model.go`](internal/model/model.go):
  - `Event`: 원본 이벤트 메타(필요 시 확장 용도).
  - `Occurrence`: 확장된 단일 occurrence:
    - `SourceID`, `UID`, `InstanceKey`, `Summary`, `Location`, `AllDay`, `Start`, `End` (display timezone).

### 2.5 Recurrence Expansion & Exceptions

- [x] Implement recurrence expansion in [`internal/ics/expand.go`](internal/ics/expand.go) using `github.com/teambition/rrule-go`:
  - `ExpandConfig`:
    - `DisplayLocation` (표시용 타임존, `nil` 시 `time.Local`)
    - `RangeStart`, `RangeEnd` – 확장 윈도우
    - `MaxOccurrencesPerEvent` – 기본 5000, 무한 루프 방지
  - `ExpandOccurrences([]ParsedEvent, ExpandConfig) (ExpandResult, error)`:
    - UID 별로 base event / override event 분리
    - 비반복 이벤트:
      - `[Start, End]` 와 `[RangeStart, RangeEnd]` 겹칠 때만 occurrence 생성
      - 동일 start 의 override(RECURRENCE-ID) 가 있으면 교체
    - 반복 이벤트:
      - `StrToRRule(ev.RawRRule)` → `RRule`
      - `Dtstart(ev.Start)`
      - `Set` 에 `RRule` 및 `EXDATE` 적용
      - `Set.Between(rangeStart, rangeEnd, true)` 로 발생 시각 계산
      - 발생 개수가 cap 초과 시 잘라내고 UID 를 `TruncatedEvents` 에 기록
      - All-day:
        - `[date 00:00, 다음날 00:00)` 로 처리
      - 일반:
        - 원래 duration (`ev.End - ev.Start`) 유지
      - 각 발생시각마다 override(RECURRENCE-ID) 를 검사하여 대체
      - `makeOccurrence` 로 display timezone 기준 `model.Occurrence` 생성
- [ ] All-day event semantics (정교한 TZ/P3D 처리 등)는 추후 렌더링/테스트 단계에서 추가 검증 예정.
- [ ] Timezone normalization:
  - 현재는 occurrence 생성 시 display timezone(`DisplayLocation`)으로 변환만 수행.
  - 세부 DST/복잡한 VTIMEZONE case 는 테스트/보완 필요.
- [x] Expansion range strategy:
  - `[RangeStart, RangeEnd]` 윈도우에 대해 RRULE `Between` 사용.
  - per-event cap(기본 5000) 적용.

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

- [-] Implement scheduler loop in [`cmd/epdcal/main.go`](cmd/epdcal/main.go):
  - [x] Periodic refresh based on config:
    - Uses `RefreshMinutes` (fallback 15분) 로 ticker 구동.
  - [ ] Respect manual triggers and recompute "next refresh" (Web UI 연동 이후).
  - [x] Clean shutdown on SIGTERM / SIGINT using context + signal.
  - [x] `--once`:
    - 한 번 `runRefreshCycle` 실행 후 종료.
  - [x] `--debug`:
    - 기본 `--config` 가 `/etc/epdcal/config.yaml` 인 경우, 디버그 모드에서는 `./config.yaml` 을 사용.
    - 캐시는 `/var/lib/epdcal/ics-cache` 대신 `./cache/ics-cache` 를 사용.
    - 개발/디버그 환경에서 root 권한 없이 ICS Fetch/Parse 테스트 가능.
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

Planned next sequence (post-ICS Fetch/Parse/Expand wiring):

1. Add ICS parsing / expansion unit tests and fixtures in `internal/ics/parse_test.go`, `internal/ics/expand_test.go` + `internal/ics/testdata/`.
2. Add basic HTTP server stub in [`internal/web/web.go`](internal/web/web.go) with `/health` and `/` placeholder page, using `Config.Listen` and optional BasicAuth (stub).
3. Design and implement minimal text-only rendering in [`internal/render/render.go`](internal/render/render.go) and NRGBA->packed planes in [`internal/convert/pack.go`](internal/convert/pack.go).
4. Wire up EPD integration in [`internal/epd/epd.go`](internal/epd/epd.go) and [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go) with `--render-only` support for development without hardware.
5. Flesh out Web UI endpoints (`/api/config`, `/api/render`, `/api/refresh`, `/preview.png`) and hook them into the core pipeline.
6. Implement runtime cache directory usage under `/var/lib/epdcal/` for ICS HTTP metadata and rendered previews.

This document should be updated as tasks are completed or requirements evolve.