# epdcal – ICS 기반 E-Paper 캘린더 (Raspberry Pi / Waveshare 12.48" B)

Raspberry Pi (Raspbian)에서 구동되는 Go 기반 데몬으로, Waveshare 12.48" tri-color e-paper (B) 패널(1304x984)에 **ICS(iCalendar) 구독**으로부터 가져온 캘린더를 렌더링하여 표시합니다.  
Google API / OAuth / Python / PIL 은 사용하지 않습니다.

- ICS URL 여러 개 구독
- 견고한 타임존 / 반복 일정(RRULE, EXDATE, RECURRENCE-ID 등) 처리
- 로컬 Web UI 로 설정 / 미리보기 / 수동 갱신(예정)
- 1bpp black/red plane 으로 패널 구동 (Waveshare C 드라이버 + cgo)
- systemd 서비스로 항상 실행
- 개발용 **`--debug` 모드**에서 로컬 `./config.yaml` / `./cache` 사용 가능

---

## 1. 기능 개요

### 1.1 ICS 구독 (OAuth 없음)

- 하나 이상의 ICS URL 을 설정 파일 또는 Web UI 에서 등록 (Web UI 는 추후 구현)
- 주기적으로(기본 15분) 및 CLI 의 `--once` 옵션으로 on-demand 1회 갱신
- HTTP 캐시 지원:
  - `ETag` / `Last-Modified` 저장
  - `If-None-Match` / `If-Modified-Since` 헤더 전송
  - 304 응답 시 이전 본문 재사용
- 네트워크/서버 오류 발생 시:
  - 데몬은 크래시 없이 계속 동작
  - 캐시된 ICS 본문이 있다면 그대로 사용

구현 파일:

- [`internal/ics/fetch.go`](internal/ics/fetch.go:1) – ICS 다운로드 및 HTTP 캐싱
- [`internal/ics/parse.go`](internal/ics/parse.go:1) – ICS 파싱 (VEVENT/VTIMEZONE 등)

### 1.2 타임존 및 iCalendar 처리 (진행 중)

목표: **정확성을 우선**으로, 일반적인 사용 사례에서 안정적으로 동작하는 ICS 처리.

현재 상태:

- [`internal/ics/parse.go`](internal/ics/parse.go:1)에서 VEVENT 파싱 구현:
  - `UID`, `SEQUENCE`, `SUMMARY`, `DESCRIPTION`, `LOCATION`
  - `DTSTART` / `DTEND` → `ve.GetStartAt()`, `ve.GetEndAt()` 사용
  - all-day 판별: `VALUE=DATE` 또는 값에 `T` 없음
  - `TZID` 파라미터 추출: `StartTZ`, `EndTZ`
  - `RRULE` 원문 문자열을 `RawRRule` 로 저장
  - `EXDATE` / `RECURRENCE-ID` 를 `[]time.Time` / `*time.Time` 로 파싱
- 추후 [`internal/ics/expand.go`](internal/ics/expand.go:1)에서 RRULE / EXDATE / RECURRENCE-ID 기반 recurrence 확장을 구현 예정

### 1.3 Web UI (예정)

내장 HTTP 서버를 통해 간단한 관리 UI 를 제공할 예정입니다. (아직 미구현)

- 계획:
  - `GET /` – HTML 설정/상태 페이지
  - `GET /api/config` – JSON 설정 조회
  - `POST /api/config` – JSON 설정 갱신 (`config.Save` 사용)
  - `POST /api/refresh` – fetch + render + display
  - `POST /api/render` – fetch + render (display 생략)
  - `GET /preview.png` – 마지막 렌더링된 미리보기 PNG
  - `GET /health` – 헬스 체크(인증 없이 접근 가능)
- 보안:
  - 기본은 loopback(127.0.0.1)에만 바인딩
  - 설정에서 Basic Auth 활성화 가능
  - Basic Auth 활성화 시 `/health` 를 제외한 모든 endpoint 보호

구현 파일(예정):

- [`internal/web/web.go`](internal/web/web.go:1)

### 1.4 설정 & 런타임 캐시

- 설정 파일:
  - 운영 기본: `/etc/epdcal/config.yaml`
  - CLI `--config` 로 경로 변경 가능
  - 첫 실행시:
    - 파일이 없으면 [`internal/config/config.go`](internal/config/config.go:1)의 `Load` 가 `DefaultConfig()` 로 파일 생성
    - 권한 `0600` 설정
- 런타임 캐시:
  - 운영 기본: `/var/lib/epdcal/ics-cache`
  - ICS URL 해시별 디렉터리:
    - `meta.json`: URL, ETag, Last-Modified, UpdatedAt
    - `body.ics`: ICS 본문
- 개발/디버그 환경:
  - `--debug` 플래그 사용 시:
    - 기본 config 경로를 `./config.yaml` 로 변경
    - 캐시 경로를 `./cache/ics-cache` 로 변경 (root 권한 불필요)

구현 파일:

- [`internal/config/config.go`](internal/config/config.go:1)

---

## 2. 아키텍처 및 코드 구조

### 2.1 전체 구조

- [`cmd/epdcal/main.go`](cmd/epdcal/main.go:1) – CLI 엔트리포인트, 플래그 파싱, 스케줄러 루프
- [`internal/config/config.go`](internal/config/config.go:1) – YAML 설정 로드/저장, 기본 값, Normalize
- [`internal/web/web.go`](internal/web/web.go:1) – (예정) HTTP 서버, Web UI, JSON API
- [`internal/ics/fetch.go`](internal/ics/fetch.go:1) – ICS 다운로드, HTTP 캐싱
- [`internal/ics/parse.go`](internal/ics/parse.go:1) – ICS 파싱 (VEVENT)
- [`internal/ics/expand.go`](internal/ics/expand.go:1) – (예정) recurrence 확장
- [`internal/model/model.go`](internal/model/model.go:1) – (예정) 이벤트/occurrence 도메인 모델
- [`internal/render/render.go`](internal/render/render.go:1) – (예정) `image.NRGBA` 기반 렌더링
- [`internal/convert/pack.go`](internal/convert/pack.go:1) – (예정) NRGBA → 1bpp plane 변환
- [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go:1) – (예정) Waveshare C 드라이버 cgo 바인딩
- [`internal/epd/epd.go`](internal/epd/epd.go:1) – (예정) 고수준 EPD 제어
- [`internal/log/log.go`](internal/log/log.go:1) – 공통 로거
- [`Makefile`](Makefile:1) – 빌드/테스트/설치 자동화
- [`systemd/epdcal.service`](systemd/epdcal.service:1) – (예정) systemd 유닛

### 2.2 로깅

[`internal/log/log.go`](internal/log/log.go:1):

- 출력 대상: `os.Stderr` (systemd/journald 에 의해 수집)
- 포맷:
  - `2025-12-17T14:51:30.123456789Z [INFO] message key=value ...`
- 구현:
  - `log.Info(msg string, kv ...any)`
  - `log.Error(msg string, err error, kv ...any)`
  - `log.Debug(msg string, kv ...any)`
- 주의:
  - stdlib logger flags 를 `0` 으로 설정하여 **중복 타임스탬프** 발생을 방지

---

## 3. 설치 및 빌드

### 3.1 요구 사항

- Go 1.25.5 (또는 호환) – [`go.mod`](go.mod:1)에 명시
- Raspberry Pi (Raspbian)
- Waveshare 12.48" B e-paper + C 드라이버 (EPD 연동 단계에서 필요)
- C 컴파일러 (예: `gcc`)

### 3.2 의존성 설치

루트에서 한 번만:

```bash
go get gopkg.in/yaml.v3
go get github.com/arran4/golang-ical
go mod tidy
```

### 3.3 빌드 (개발/운영 공통)

```bash
make build
# 또는
go build -o epdcal ./cmd/epdcal
```

### 3.4 크로스 컴파일 (x86_64 → Raspberry Pi)

```bash
GOOS=linux GOARCH=arm GOARM=7 go build -o epdcal ./cmd/epdcal   # 32bit
GOOS=linux GOARCH=arm64       go build -o epdcal ./cmd/epdcal   # 64bit
```

---

## 4. 설정 방법

### 4.1 운영 환경 (기본)

- 기본 설정 파일: `/etc/epdcal/config.yaml`
- 없으면 첫 실행 시 자동 생성 (`DefaultConfig()` 기반, 권한 0600)

기본 예시:

```yaml
listen: "127.0.0.1:8080"
timezone: "Asia/Seoul"
refresh_minutes: 15
horizon_days: 7
show_all_day: true
highlight_red:
  - "휴일"
  - "휴가"
  - "중요"
ics:
  - url: "https://calendar.google.com/calendar/ical/....../basic.ics"
    id: "gcal"
    name: "Google Calendar"
basic_auth: null
```

- `ics` 필드는 `url` / `id` / `name` 를 가진 구조체 리스트여야 한다.

### 4.2 개발/디버그 환경 (`--debug`)

디버그용 시스템에서는 `/etc`, `/var/lib` 에 쓸 수 없을 수 있으므로:

- `--debug` 사용 시:
  - 기본 config 경로 `/etc/epdcal/config.yaml` → `./config.yaml` 로 변경
  - 캐시 경로 `/var/lib/epdcal/ics-cache` → `./cache/ics-cache` 로 변경
- 권장 실행:

```bash
./epdcal --debug --once
```

- 루트에 `config.yaml` 작성:
  - 위 예시와 같은 형식
- `./cache/ics-cache` 아래에 URL 해시 기반 subdir/cached 파일이 생성됨

---

## 5. 동작 방식 (ICS Fetch/Parse + 스케줄러)

엔트리포인트: [`cmd/epdcal/main.go`](cmd/epdcal/main.go:1)

### 5.1 CLI 플래그

- `--config /path/to/config.yaml`
  - 설정 파일 경로 (기본 `/etc/epdcal/config.yaml`)
- `--listen 127.0.0.1:8080`
  - Web UI / API 리슨 주소 (현재는 Web UI 미구현, 향후 사용; 설정 값 override)
- `--once`
  - 한 번 `fetch + parse` 실행 후 종료
- `--render-only`
  - (예약) EPD 하드웨어 호출 없이 렌더링/pack/dump 만 수행
- `--dump`
  - (예약) `black.bin`, `red.bin`, `preview.png` 디버그 출력
- `--debug`
  - 디버그 모드:
    - 기본 config 경로가 `/etc/epdcal/config.yaml` 라면 이를 `./config.yaml` 로 변경
    - ICS 캐시 디렉터리를 `/var/lib/epdcal/ics-cache` 대신 `./cache/ics-cache` 로 사용

### 5.2 스케줄러 및 `--once`

[`cmd/epdcal/main.go`](cmd/epdcal/main.go:1)의 주요 흐름:

1. 설정 로드:
   ```go
   conf, err := config.Load(flags.configPath)
   ```
   - 파일 없으면 기본값으로 생성 후 로드

2. `--once`:
   ```go
   if flags.once {
       runRefreshCycle(ctx, conf, flags.debug)
       return
   }
   ```
   - 한 번의 Refresh 사이클(`runRefreshCycle`) 실행 후 종료

3. 주기 실행:
   - `interval := time.Duration(conf.RefreshMinutes) * time.Minute`
     - 0 이하이면 15분으로 강제
   - 초기 한 번 `runRefreshCycle` 실행
   - `time.NewTicker(interval)` 으로 매 interval 마다 실행

4. `runRefreshCycle`:
   - `conf.ICS` 를 [`ics.Source`](internal/ics/fetch.go:1) 리스트로 변환
   - `context.WithTimeout(parentCtx, 60*time.Second)` 으로 타임아웃
   - `cacheDir` 선택:
     - `debug == false` → `/var/lib/epdcal/ics-cache`
     - `debug == true` → `./cache/ics-cache`
   - `ics.NewFetcher(cacheDir)` 생성
   - `FetchAll` → `ParseICS` 호출
   - 각 소스에 대해:
     - fetch 결과/캐시 여부/이벤트 개수 로그
   - 전체 파싱 이벤트 개수/소요 시간 로그

---

## 6. ICS 타임존 / 반복 처리 상세 (계획)

(요약만 유지, 상세는 초기 설계대로 진행)

- Time normalization:
  - display timezone (`Config.Timezone`) 기준으로 모든 occurrence 변환
- Recurrence:
  - `RRULE` / `EXDATE` / `RECURRENCE-ID` 처리
  - 기간 `[now - backfill, now + horizon]` 내에서만 확장
- All-day:
  - DATE 값인 일정은 `[해당 날짜 00:00, 다음날 00:00)` 로 처리

---

## 7. 개발/테스트 팁

### 7.1 ICS 파이프라인 테스트 (`--debug + --once`)

```bash
go get gopkg.in/yaml.v3
go get github.com/arran4/golang-ical
go mod tidy

go build -o epdcal ./cmd/epdcal

# 루트에 config.yaml 작성 후:
./epdcal --debug --once
```

- 로그에서:
  - `refresh cycle start`
  - `ics fetch start`
  - `ics fetch success` 또는 오류 + 캐시 사용
  - `ics parse completed (event_count=N)`
  - `refresh cycle completed (parsed_event_total=N)`
  - `once mode completed; exiting`
- `./cache/ics-cache` 에 ICS 캐시 파일 생성 확인 가능

---

## 8. Troubleshooting (일부 업데이트)

1. **로그에 날짜가 두 번 찍힘**
   - 해결됨:
     - [`internal/log/log.go`](internal/log/log.go:1) 에서 stdlib logger flags 제거
     - 이제 라인당 한 번만 RFC3339Nano 타임스탬프 출력

2. **`cannot unmarshal ...` 오류 (Google ICS URL 사용 시)**
   - 원인:
     - `config.yaml` 의 `ics` 필드를 문자열 리스트로만 작성한 경우
   - 해결:
     - `ics` 항목을 `url`/`id`/`name` 필드가 있는 구조체 리스트로 작성해야 함:
       ```yaml
       ics:
         - url: "https://calendar.google.com/calendar/ical/....../basic.ics"
           id: "gcal"
           name: "Google Calendar"
       ```

3. **/var/lib/epdcal 에 쓸 권한이 없는 디버그 환경**
   - `--debug` 플래그 사용:
     - config: `./config.yaml`
     - cache: `./cache/ics-cache`

---

## 9. 개발 진행 상황

세부 TODO 및 진행 상황은 [`progress.md`](progress.md:1)에 최신 상태로 정리되어 있습니다.  
이 README 는 전체 기능/요구사항 및 현재 구현 상태를 개략적으로 설명하며, 작업 우선순위/남은 일감은 progress 문서를 참고하십시오.