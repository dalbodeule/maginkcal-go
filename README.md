# epdcal – ICS 기반 E-Paper 캘린더 (Raspberry Pi)

Raspberry Pi(ARM, Raspbian)에서 **Waveshare 12.48" tri‑color e-paper (B)** 패널(1304x984)을 구동하여, 하나 이상의 **ICS(iCalendar) 구독**으로부터 캘린더를 렌더링하고 표시하는 Go 애플리케이션입니다.  

- 여러 ICS URL 구독 (OAuth / Google API 사용 안 함)
- TZID/VTIMEZONE, RRULE, EXDATE, RECURRENCE-ID, all-day 이벤트 처리
- 로컬 Web UI로 설정 관리 및 Preview/Refresh
- Headless Chromium을 이용해 Web UI(`/calendar`)를 캡처하여 EPD로 전송

---

## 1. 전체 아키텍처

최종 목표 구조:

```text
+-------------------------------+
|          Web UI (Next.js)     |
|   - /calendar 뷰              |
|   - 설정 관리 (TODO)          |
+-------------------------------+
                ^
                | HTTP (localhost)
                v
+-------------------------------+
|       Go 데몬 epdcal          |
|  - HTTP 서버(/api/*, /health) |
|  - ICS Fetch/Parse/Expand     |
|  - Chromium 캡처              |
|  - PNG → packed plane 변환    |
|  - EPD C driver 호출          |
+-------------------------------+
                ^
                | SPI / GPIO
                v
+-------------------------------+
| Waveshare 12.48" EPD (B)     |
+-------------------------------+
```

주요 구성 요소(예정/부분 구현):

- Go 메인 바이너리: [`cmd/epdcal/main.go`](cmd/epdcal/main.go)
- 설정 로딩/저장: [`internal/config/config.go`](internal/config/config.go)
- ICS HTTP Fetch + 캐시: [`internal/ics/fetch.go`](internal/ics/fetch.go)
- ICS 파싱/정규화: [`internal/ics/parse.go`](internal/ics/parse.go)
- 반복/예외 확장: [`internal/ics/expand.go`](internal/ics/expand.go)
- 이벤트 모델: [`internal/model/model.go`](internal/model/model.go)
- Web 서버 및 API: [`internal/web/web.go`](internal/web/web.go)
- Headless Chromium 캡처: [`internal/capture/chromium.go`](internal/capture/chromium.go)
- EPD C 드라이버 연동(cgo): [`internal/epd/epd.go`](internal/epd/epd.go), [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go)
- PNG → packed plane 변환: [`internal/convert/pack.go`](internal/convert/pack.go)
- 진행 상황 정리: [`progress.md`](progress.md)
- systemd 유닛: [`systemd/epdcal.service`](systemd/epdcal.service)

---

## 2. 하드웨어 / 디스플레이 드라이버

### 2.1 대상 패널

- Waveshare 12.48" tri-color e-paper (B)
- 해상도:
  - width = 1304
  - height = 984
- Tri-color (Black/Red/White), 듀얼 1bpp plane 사용:
  - Black plane
  - Red plane (전송 시 C 드라이버에서 비트 반전)

### 2.2 C 드라이버 (Waveshare 제공)

C 헤더 `EPD_12in48B.h`에서 제공되는 API를 cgo로 래핑합니다:

```c
UBYTE EPD_12in48B_Init(void);
void EPD_12in48B_Clear(void);
void EPD_12in48B_Display(const UBYTE *BlackImage, const UBYTE *RedImage);
void EPD_12in48B_TurnOnDisplay(void);
void EPD_12in48B_Sleep(void);
```

버퍼 형식:

- stride = 163 bytes per row (1304 / 8)
- 각 plane 크기 = 163 * 984 = 160392 bytes
- 인덱싱: `offset = y * 163 + xByte`

Go 측의 bit packing 규칙:

- 1bpp, MSB-first
- 픽셀 (x, y)에 대해:
  - `byteIndex = y*163 + (x >> 3)`
  - `mask = 0x80 >> (x & 7)`
- 초기 값:
  - 버퍼는 0xFF(white)로 초기화
  - bit=0 → 잉크(검정 또는 빨강)
  - bit=1 → white
- Red plane:
  - Go 쪽에서는 black과 동일하게 "0=잉크"로 유지
  - C 드라이버가 전송 시 `~RedImageByte`로 반전

---

## 3. ICS 구독 및 HTTP 캐시

### 3.1 ICS 구독

- 여러 ICS URL 지원
- Fetch 트리거:
  - 주기적(기본 15분)
  - Web UI에서 수동 Refresh
- OAuth / Google API / token.pickle 등 **사용 금지**
  - 순수 HTTP GET 기반 `.ics` 구독만 지원

### 3.2 HTTP 캐싱

모듈: [`internal/ics/fetch.go`](internal/ics/fetch.go)

- 응답 헤더:
  - `ETag`, `Last-Modified`를 로컬 메타파일로 저장
- 요청 시:
  - `If-None-Match`, `If-Modified-Since` 헤더 사용
- 304(Not Modified) 또는 네트워크 에러일 경우:
  - 마지막으로 성공적으로 저장한 `body.ics`를 사용
- 캐시 경로:
  - 기본: `/var/lib/epdcal/ics-cache/`
  - 디버그 모드(`--debug`): `./cache/ics-cache/`

---

## 4. iCalendar 처리 (Timezone / Recurrence / 예외)

타임존/반복 이벤트 처리는 이 프로젝트에서 **가장 중요한 부분**입니다. breadth 보다 **정확성**을 우선합니다.

### 4.1 타임존(TZID/VTIMEZONE)

- ICS 내부의 `VTIMEZONE` 블록을 파싱
- `DTSTART;TZID=Zone/...`, `DTEND;TZID=Zone/...` 지원
- `Z`(UTC)로 끝나는 DATE-TIME은 UTC로 파싱
- floating time(타임존 없는 LOCAL-TIME)도 최대한 합리적으로 처리
- 모든 occurrence는 **표시용 타임존**으로 변환:
  - `config.Timezone` (예: `Asia/Seoul`)
- DST가 있는 타임존에 대해서도:
  - `VTIMEZONE` 정의와 Go time zone DB를 최대한 활용하여 자연스러운 동작 보장

### 4.2 반복 이벤트 (RRULE)

RRULE 파싱/확장은 [`internal/ics/expand.go`](internal/ics/expand.go)에서 처리하며, `github.com/teambition/rrule-go`등의 라이브러리 사용을 전제로 합니다.

최소 지원 범위:

- `FREQ=DAILY/WEEKLY/MONTHLY/YEARLY`
- `BYDAY`
- `BYMONTHDAY`
- `INTERVAL`
- `COUNT`
- `UNTIL`

전략:

- 각 VEVENT에 대해:
  - `RRULE`/`RDATE` 없음: 단일 occurrence 생성
  - `RRULE` 존재:
    - horizon window 내에서만 occurrence 확장
- 확장 범위:
  - `[rangeStart, rangeEnd]` (예: now - backfill ~ now + horizonDays)
  - backfill: 자정 경계(00:00)를 넘는 이벤트를 위해 설정
- 발생 수 상한:
  - 이벤트당 max occurrences (예: 5000개)
  - 상한을 초과하면 워닝 로그 기록 후 추가 확장 중단

### 4.3 예외: EXDATE / RECURRENCE-ID

예외 처리 규칙:

- `EXDATE`:
  - base DTSTART 의 타임존/UTC 기준으로 시각을 맞추어 비교
  - 해당 occurrence를 제거
- `RECURRENCE-ID`:
  - (UID, RECURRENCE-ID timestamp)를 키로 override VEVENT를 맵핑
  - base RRULE 확장 시:
    - 동일 키의 occurrence가 있으면 base occurrence를 override 이벤트로 대체

### 4.4 All-day 이벤트

- DATE 타입과 DATE-TIME 타입 구분
- All-day(DATE) 이벤트는 표시용 타임존 기준:
  - start: local date 의 00:00
  - end: 다음 날 00:00 (exclusive)
- 렌더링 시:
  - all-day 섹션이 따로 존재할 수 있으며(옵션),
  - 하루를 완전히 커버하는 이벤트로 취급

### 4.5 UID 안정성 / 중복 제거

- 여러 캘린더(ICS URL)에서 이벤트를 merge할 때:
  - key = (calendarID 또는 url, UID, recurrence-instance key)
  - 동일 key 를 가진 occurrence는 하나로 합침
- recurrence-instance key:
  - base: DTSTART(정규화된 시작 시각)
  - override: RECURRENCE-ID을 기준으로 결정

---

## 5. 설정 파일 및 런타임 디렉터리

### 5.1 설정 파일

- 기본 경로: `/etc/epdcal/config.yaml`
- CLI 플래그: `--config /path/to/config.yaml` 로 변경 가능
- 최초 실행 시:
  - 파일이 없으면 디폴트 설정으로 생성
  - Web UI URL 출력
  - 파일 퍼미션: 0600

설정 구조(요약):

```yaml
listen: "127.0.0.1:8080"
timezone: "Asia/Seoul"
week_start: "monday"  # 또는 "sunday"
refresh_minutes: 15
horizon_days: 7
show_all_day: true
highlight_red:
  - "휴가"
  - "공휴일"
ics:
  - id: "personal"
    name: "개인 캘린더"
    url: "https://example.com/personal.ics"
  - id: "work"
    name: "회사 캘린더"
    url: "https://example.com/work.ics"
basic_auth:
  username: "user"
  password: "pass"
```

구체적인 struct 정의는 [`internal/config/config.go`](internal/config/config.go)에 구현됩니다.

### 5.2 런타임 디렉터리

- 기본:
  - 설정: `/etc/epdcal/config.yaml`
  - 캐시/아티팩트: `/var/lib/epdcal/`
    - `/var/lib/epdcal/ics-cache/` – ICS HTTP 캐시
    - `/var/lib/epdcal/preview.png` – 마지막 렌더링 Preview
    - `/var/lib/epdcal/black.bin`, `/var/lib/epdcal/red.bin` – packed plane (예정)
- 디버그 모드 (`--debug`):
  - 설정: `./config.yaml`
  - 캐시: `./cache/ics-cache/`, `./cache/preview.png` 등

---

## 6. Web UI & HTTP API

### 6.1 HTTP 서버

메인 서버 시작 코드는 [`cmd/epdcal/main.go`](cmd/epdcal/main.go)에서:

- `web.StartServer(ctx, conf, debug)` 호출로 시작
- 기본 listen: `127.0.0.1:8080`
- `--listen` 플래그로 덮어쓰기 가능

### 6.2 엔드포인트 설계

- `GET /`  
  - Web UI 기본 페이지 (Next.js 정적 빌드)
- `GET /calendar`  
  - EPD용 캘린더 레이아웃 전용 페이지(Next.js)
  - headless Chromium 캡처 타깃
- `GET /api/events`  
  - 확장된 occurrence 리스트 반환
  - 응답 예시:
    ```json
    {
      "occurrences": [...],
      "truncated_uids": [...],
      "range_start": "2025-01-01T00:00:00+09:00",
      "range_end": "2025-01-08T00:00:00+09:00",
      "display_timezone": "Asia/Seoul",
      "week_start": "monday"
    }
    ```
- `GET /api/battery`  
  - 배터리 상태 리포트 (mock → 추후 PiSugar3 I2C로 교체 가능)
  - 예:
    ```json
    { "percent": 73, "voltage_mv": 0 }
    ```
- `GET /api/config` (TODO)  
  - 현재 설정 조회(JSON)
- `POST /api/config` (TODO)  
  - 설정 업데이트(JSON)
- `POST /api/refresh` (TODO)  
  - 즉시 ICS fetch + expand + render + display 트리거
- `POST /api/render` (TODO)  
  - ICS fetch + expand + render만 수행 (display는 터치 안 함)
- `GET /preview.png` (TODO)  
  - 마지막 Preview PNG 반환
- `GET /health`  
  - 헬스체크(예: 200 OK, body = "OK")

### 6.3 Web UI – `/calendar` 페이지

프론트엔드 구현은 Next.js 기준으로 진행되며, 실제 코드는 `webui/` 디렉토리에 위치합니다 (빌드 결과는 Go 바이너리에 embed).

주요 특징:

- 고정 캔버스 크기:
  - `<main className="w-[1304px] h-[1200px] ...">`
  - 상단은 날짜/타임존/배터리/상태 영역, 하단은 월간 그리드
- `/api/events`, `/api/battery`에서 데이터를 fetch
- 데이터 로딩 완료 시 root element에 `data-ready="true"` 속성 추가
  - headless Chromium에서 이 속성을 기다렸다가 스크린샷 캡처
- ICS 이벤트 표시:
  - 월간 그리드(5주 또는 6주)
  - 오늘 강조, 주말 강조
  - All-day 이벤트와 시간 이벤트를 다르게 표시
- 배터리 표시:
  - `/api/battery`의 percent 구간별로 5단계 아이콘 표시
  - Font Awesome `battery-empty`, `battery-quarter`, `battery-half`, `battery-three-quarters`, `battery-full` 사용

---

## 7. 렌더링 파이프라인

### 7.1 전체 흐름

1. **ICS Fetch/Parse/Expand**
   - `runRefreshCycle`에서 ICS 소스 전부에 대해 fetch + parse 수행
   - parse 결과를 recurrence 확장 모듈로 전달해 `[rangeStart, rangeEnd]` 범위의 occurrence 생성
2. **Web UI 데이터 제공**
   - `/api/events`에서 occurrence 리스트를 JSON으로 반환
   - `/api/battery`에서 배터리 상태 반환
3. **Web UI 렌더링**
   - Next.js `/calendar` 페이지에서 JSON을 기반으로 레이아웃 렌더
   - 로딩 완료 후 `data-ready="true"`로 표시
4. **Chromium 캡처**
   - Go 측에서 headless Chromium(chromedp)을 이용해 `/calendar` 페이지를 엽니다.
   - `data-ready="true"` element가 `Visible` 될 때까지 대기 후,
   - viewport = 1304x1200 으로 full screenshot(PNG) 캡처
   - [`internal/capture/chromium.go`](internal/capture/chromium.go) 참고
5. **PNG → Black/Red Plane**
   - PNG를 `image.NRGBA`로 디코드
   - `internal/convert/pack.go`에서:
     - 한 번의 루프로 각 픽셀의 색을 분석
     - black plane / red plane 각각의 bit 설정 (0=잉크, 1=white)
6. **EPD 디스플레이**
   - `internal/epd` 패키지에서:
     - `EPD_12in48B_Init`
     - 필요 시 `EPD_12in48B_Clear`
     - `EPD_12in48B_Display(black, red)`
     - `EPD_12in48B_Sleep`
   - `--render-only` 플래그가 설정된 경우 실제 EPD 호출은 생략하고, PNG/packed buffer만 생성(개발/테스트용)

### 7.2 디버그 모드 & Dump

- `--dump` 플래그:
  - `preview.png` (Chromium 캡처 결과)
  - `black.bin`, `red.bin` (packed plane) – TODO
- `--debug` 플래그:
  - `/etc`, `/var/lib` 대신 로컬 디렉터리 사용
  - 개발 PC에서 root 권한 없이 테스트 가능

---

## 8. 빌드 & 실행 방법 (요약)

### 8.1 사전 준비

- Go (1.22+ 권장)
- Node.js / pnpm (Web UI 빌드용, 이미 빌드된 정적 파일이 있다면 생략 가능)
- Raspberry Pi:
  - SPI / GPIO 설정 (Waveshare 가이드 참고)
  - Chromium or Chromium headless 설치 (Raspbian)

### 8.2 Web UI 빌드 (예시)

```bash
cd webui
pnpm install
pnpm build
# Next.js 출력물을 Go embed 대상 디렉터리로 복사 (예: ../internal/web/static 또는 embed FS)
```

(실제 빌드 경로/스크립트는 구현 시점에 README에 추가로 명시)

### 8.3 Go 바이너리 빌드

```bash
cd /path/to/repo
go build -o epdcal ./cmd/epdcal
```

Raspberry Pi에서 직접 빌드하거나, cross-compile 설정을 사용할 수 있습니다.

### 8.4 기본 실행 예시

개발/디버그 환경 (Chromium 캡처 테스트):

```bash
./epdcal --debug --listen 127.0.0.1:8080 --once --dump
```

- `--debug`:
  - `./config.yaml`, `./cache/` 를 사용
- `--once`:
  - 한 번 refresh cycle → (옵션) capture 테스트 후 종료
- `--dump`:
  - `./cache/preview.png` 생성 (Chromium 캡처 결과)

서비스 모드 (Raspberry Pi, systemd):

1. 설정 파일 위치:
   - `/etc/epdcal/config.yaml` 생성/수정
2. systemd 유닛 설치:
   - [`systemd/epdcal.service`](systemd/epdcal.service)를 `/etc/systemd/system/epdcal.service`로 복사
3. enable & start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable epdcal
   sudo systemctl start epdcal
   ```

---

## 9. Recurrence/TZ 테스트 전략

ICS 반복 및 타임존 처리는 unit test로 강하게 검증합니다. 테스트는 [`internal/ics`](internal/ics) 패키지 아래에 위치합니다.

테스트용 ICS fixture:

- `internal/ics/testdata/simple.ics`
  - 단일 non-recurring 이벤트
- `internal/ics/testdata/weekly_exdate.ics`
  - weekly recurring 이벤트 + EXDATE
- `internal/ics/testdata/override_recurrence_id.ics`
  - RECURRENCE-ID를 사용한 단일 occurrence override
- `internal/ics/testdata/tz_utc_allday.ics`
  - TZID 이벤트, UTC 이벤트, all-day 이벤트 혼합

테스트 코드(예정):

- `internal/ics/parse_test.go`
  - VEVENT 필드(DTSTART/DTEND, RRULE, EXDATE, RECURRENCE-ID, UID 등) 파싱 검증
- `internal/ics/expand_test.go`
  - 확장된 occurrence 의 Start/End가 표시용 타임존에서 기대값과 일치하는지 검증
  - EXDATE/RECURRENCE-ID 적용 결과 중복/누락 여부 검사

---

## 10. Known Limitations (예정/계획)

다음 항목들은 구현 라이브러리 및 시간 제약에 따라 제한될 수 있으며, 실제 구현 상태에 맞추어 이 섹션을 계속 업데이트해야 합니다.

- RRULE:
  - 매우 복잡한 규칙 (BYSETPOS, 복잡한 BYxxx 조합 등)은 부분 지원 또는 미지원일 수 있음
- VTIMEZONE:
  - 특수한/예외적인 히스토리컬 타임존 규칙은 Go 표준 시간대 DB 및 ICS 정의에 의존
- Rendering:
  - 초기에 렌더링은 단순한 월간 or 주간 뷰로 제한
  - 긴 설명/제목은 줄바꿈 또는 생략 처리
- Web UI:
  - 초기 버전에서는 설정 페이지 UX가 단순할 수 있음
  - 모바일 브라우저에서의 사용성 최적화는 2차 과제

각 제한사항은 구현 진행에 따라 [`progress.md`](progress.md)와 이 README의 이 섹션에 반영합니다.

---

## 11. Troubleshooting

### 11.1 Chromium/Chromedp 관련 문제

- 증상:
  - `--once --dump` 실행 시 `preview.png`가 생성되지 않거나 비어 있음
- 점검:
  - Raspberry Pi에 Chromium/Chromium headless가 설치되어 있는지 확인
  - 환경변수 `DISPLAY` 설정이 필요한지 (X11 기반 vs headless)
  - `/calendar` 페이지에 접근했을 때 실제로 렌더가 완료되는지 브라우저로 확인
- 로그:
  - `internal/capture/chromium.go`에서 chromedp 에러를 로그로 남깁니다.

### 11.2 EPD 표시 문제

- 증상:
  - 화면이 갱신되지 않거나, 검정/빨강이 반전됨, 노이즈 발생
- 점검:
  - SPI, GPIO, 전원 연결 상태 확인 (Waveshare 공식 가이드 참고)
  - `EPD_12in48B_Init`, `EPD_12in48B_Clear`, `EPD_12in48B_Display`, `EPD_12in48B_Sleep` 호출 순서 확인
  - `black.bin`, `red.bin`을 오실로스코프 또는 로직 분석기로 디버깅할 수 있다면, 전송되는 비트 패턴 검증
- 코드:
  - packed buffer 변환 로직(`internal/convert/pack.go`)에서 stride, bit 순서(MSB-first) 재확인

### 11.3 ICS 파싱/시간 문제

- 증상:
  - 이벤트 시간이 1시간씩 밀리거나, all-day 이벤트가 잘못된 날에 표시
- 점검:
  - `config.Timezone`가 올바른 IANA 이름인지 확인 (`Asia/Seoul` 등)
  - ICS 내 `VTIMEZONE` 정의와 실제 사용 시간대가 일치하는지
  - UNTIL, EXDATE, RECURRENCE-ID가 UTC인지 localtime인지 확인
- 해결:
  - 문제가 되는 ICS 일부를 `internal/ics/testdata`로 추가하고, 단위 테스트를 작성해 회귀 방지

### 11.4 파일 권한/경로 문제

- 증상:
  - 설정 파일 생성 실패, 캐시 디렉터리 생성 실패
- 점검:
  - `/etc/epdcal/`, `/var/lib/epdcal/`에 대해 적절한 권한이 있는지 확인
  - 디버그 모드(`--debug`)로 실행하여 로컬 디렉터리 사용이 가능한지 확인

---

## 12. 향후 작업 및 기여

진행 상태와 상세 TODO는 [`progress.md`](progress.md)에 정리합니다.  

기여 아이디어:

- ICS/TZ Recurrence 처리 고도화
- 실제 PiSugar3/I2C 기반 배터리 리더 구현
- 다양한 레이아웃(주간 뷰, ToDo 리스트 통합 등) 추가
- 설정 Web UI 개선 (다국어 지원, 인증/권한 강화)

Pull Request 또는 Issue 작성 시, 실제 사용 중인 ICS 예제(민감 정보 제거)를 함께 첨부해 주시면 Recurrence/TZ 처리 개선에 큰 도움이 됩니다.