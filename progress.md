# EPD ICS Calendar – Progress

이 문서는 Raspberry Pi용 Waveshare 12.48" 3색 e-paper(B) 패널(1304x984)을 구동하는 **ICS 기반 캘린더 디스플레이/웹 UI/스케줄러** 프로젝트의 진행 상황과 설계 사항을 정리한 것이다.

---

## 0. 현재 구현 상태 요약 (2025-12-18 기준)

### 0.1 구현/동작하는 부분

- **CLI / 메인 루프**
  - [`cmd/epdcal/main.go`](cmd/epdcal/main.go)
  - 플래그:
    - `--config`, `--listen`, `--once`, `--render-only`(예약), `--dump`, `--debug`
  - SIGINT/SIGTERM 처리, context 취소, 주기적 refresh loop 구현

- **설정 로딩 및 디버그 모드**
  - [`internal/config/config.go`](internal/config/config.go)
  - 기본 설정 경로: `/etc/epdcal/config.yaml`
  - `--debug` 모드에서 `./config.yaml` 사용
  - 최초 실행 시 기본 config 생성 및 퍼미션 0600 설정 로직 구현

- **로깅**
  - [`internal/log`](internal/log) 패키지에서 leveled logger 사용 (Info/Error 등)

- **ICS Fetch + HTTP 캐시**
  - [`internal/ics/fetch.go`](internal/ics/fetch.go)
  - 기능:
    - 다중 ICS 소스에 대한 fetch
    - ETag / Last-Modified 저장
    - If-None-Match / If-Modified-Since 요청 헤더 사용
    - 304 / 네트워크 에러 시 로컬 캐시 body.ics fallback
  - 캐시 디렉터리:
    - 기본: `/var/lib/epdcal/ics-cache`
    - `--debug`: `./cache/ics-cache`

- **ICS Parse / Recurrence 확장 (코어 구현)**
  - [`internal/ics/parse.go`](internal/ics/parse.go)
    - VEVENT/VTIMEZONE 파싱, DTSTART/DTEND/TZID/EXDATE/RECURRENCE-ID/UID/RRULE 필드 파싱
  - [`internal/ics/expand.go`](internal/ics/expand.go)
    - RRULE(`FREQ`, `BYDAY`, `BYMONTHDAY`, `INTERVAL`, `COUNT`, `UNTIL` 등) 확장 로직 구현
    - EXDATE, RECURRENCE-ID override 적용
    - horizon window 내에서만 occurrence 생성, per-event 상한 적용
  - [`internal/model/model.go`](internal/model/model.go)
    - `Occurrence` 등 공통 모델 정의
  - 현재 상태:
    - **기능 구현은 되어 있으나**, fixture 기반 unit test는 아직 작성 중 (TODO)

- **Web 서버 및 API 일부**
  - [`internal/web/web.go`](internal/web/web.go)
  - 구현된 엔드포인트:
    - `GET /health` – 헬스체크
    - `GET /api/events` – ICS fetch/parse/expand 결과를 기반으로 occurrence JSON 반환
    - `GET /api/battery` – mock 배터리 정보 반환
  - 정적 Web UI (Next.js 빌드 결과)를 embed FS로 서빙:
    - `/`, `/calendar` 등
    - `/api/*`, `/preview.png` 등은 정적 서빙에서 제외

- **배터리 mock 리더**
  - [`internal/battery/battery.go`](internal/battery/battery.go)
  - `Reader` 인터페이스와 `Status` 구조체 정의
  - 개발용 mock 구현:
    - 20~100% 사이의 랜덤 percent, voltage_mv=0
  - `/api/battery`는 현재 mock reader 기반

- **Web UI `/calendar` 페이지**
  - [`webui/src/app/calendar/page.tsx`](webui/src/app/calendar/page.tsx)
  - 특징:
    - 고정 캔버스 크기: 984x1304 레이아웃 (EPD 비율에 맞춘 레이아웃)
    - `/api/events` 호출:
      - occurrence를 날짜별로 그룹핑
      - 월간 그리드(5주/6주) 생성
      - 오늘/주말/다른 달 표시 등 스타일링
    - `/api/battery` 호출:
      - 배터리 퍼센트에 따라 5단계 Font Awesome 아이콘 표시
    - `data-ready` 속성:
      - `/api/events`와 `/api/battery` 두 요청이 모두 성공하면 root div에 `data-ready="true"` 설정
      - headless Chromium 캡처에서 이 속성을 기준으로 렌더 완료 판단

- **Chromium 캡처 헬퍼 및 테스트 경로**
  - [`internal/capture/chromium.go`](internal/capture/chromium.go)
    - `CaptureCalendarPNG(opts)`:
      - viewport(기본 984x1304) 설정
      - `/calendar` 페이지로 네비게이션
      - `[data-ready="true"]` element가 visible 될 때까지 대기
      - full screenshot(PNG) 저장
  - [`cmd/epdcal/main.go`](cmd/epdcal/main.go) 의 `runCaptureTest`:
    - `--once --dump` 모드에서:
      - `runRefreshCycle` 수행 후
      - Chromium 캡처 실행
      - 출력 경로:
        - 기본: `/var/lib/epdcal/preview.png`
        - `--debug`: `./cache/preview.png`

### 0.2 아직 미구현 / 진행 중인 부분

- ICS Recurrence/TZ 단위 테스트:
  - `internal/ics/testdata/*.ics` fixture 및 `parse_test.go` / `expand_test.go` 작성
- TZID/VTIMEZONE 및 DST 경계에 대한 **정확한 검증**:
  - 현재 로직은 구현되어 있으나, 다양한 타임존/경계 케이스에 대한 테스트 미완료
- PNG → black/red packed plane 변환:
  - [`internal/convert/pack.go`](internal/convert/pack.go) 구현 필요
- EPD C 드라이버 cgo 래핑 및 하드웨어 통합:
  - [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go), [`internal/epd/epd.go`](internal/epd/epd.go) 구현
  - `--render-only` 처리 포함
- 정식 display 파이프라인 통합:
  - refresh loop에서:
    - ICS expand → `/calendar` 렌더 → 캡처 → PNG → packed plane → EPD 표시까지 연결
- 설정/관리용 Web API:
  - `/api/config` (GET/POST), `/api/refresh`, `/api/render`, `GET /preview.png` (실제 파일 제공)
- 런타임 캐시 확장:
  - 마지막 성공 렌더링된 packed plane/PNG 보관 및 에러 시 fallback
- Basic Auth:
  - 설정에 따른 웹 UI/ API 보호 ( `/health` 제외 )
- systemd 유닛:
  - [`systemd/epdcal.service`](systemd/epdcal.service) 작성 및 설치 가이드

이하 섹션은 여전히 **타깃 스펙/설계 문서**로 유지하며, 위의 0.x 섹션이 실제 구현 진행 상황을 반영한다.

---

## 1. 프로젝트 개요

- 단일 Go 바이너리 `epdcal` (Raspbian/ARM용)
- 기능:
  - 여러 개의 ICS(iCalendar) 구독 URL에서 이벤트 수집 (OAuth/Google API 사용 금지)
  - 타임존/반복/예외/RECURRENCE-ID 를 최대한 RFC 5545에 맞게 처리
  - 로컬 Web UI:
    - 설정(ICS URL/타임존/리프레시 간격/표시 옵션) 관리
    - 수동 Refresh / Render Preview 트리거
    - 상태(최근 갱신 시간, 다음 스케줄, 마지막 오류) 노출
  - Waveshare 12.48" tri-color e-paper (B) 패널에 캘린더 이미지 출력
- 렌더링 전략:
  - Go 내장 텍스트 렌더링 대신, Web UI(`/calendar`)를 **표준 레이아웃**으로 사용
  - Headless Chromium(chromedp)을 통해 `/calendar`를 1304x984(또는 1304x1200 캔버스)로 캡처 후,
    - PNG → 2-plane 1bpp packed buffer(black/red) 변환
    - EPD C 드라이버를 통해 전송

---

## 2. 요구사항 정리

### 2.1 하드웨어 / 디스플레이 드라이버

- Waveshare 12.48" tri-color e-paper (B), 해상도 1304x984
- C 헤더 `EPD_12in48B.h` 에서 제공하는 API 사용(cgo 연동):
  - `UBYTE EPD_12in48B_Init(void);`
  - `void EPD_12in48B_Clear(void);`
  - `void EPD_12in48B_Display(const UBYTE *BlackImage, const UBYTE *RedImage);`
  - `void EPD_12in48B_TurnOnDisplay(void);`
  - `void EPD_12in48B_Sleep(void);`
- 버퍼 사양:
  - width = 1304, height = 984
  - stride = 163 bytes/row (1304 / 8)
  - plane buffer size = 163 * 984 = 160392 bytes
  - 인덱싱: `offset = y*163 + xByte`
- Go 쪽 bit packing:
  - MSB-first 1bpp
  - 픽셀 (x, y)에 대해:
    - `byteIndex = y*163 + (x >> 3)`
    - `mask = 0x80 >> (x & 7)`
  - 초기 버퍼는 0xFF(white)로 채움
  - bit=0 → 잉크(black/red), bit=1 → white
  - Red plane은 Go에서 0=red 로 유지, C 라이브러리에서 전송 시 `~RedImageByte` 수행

### 2.2 ICS 구독 (OAuth 금지)

- 여러 ICS URL 지원
- 주기적 fetch (기본 15분) + Web UI에서 on-demand fetch
- HTTP 캐싱:
  - ETag, Last-Modified 저장
  - If-None-Match / If-Modified-Since 헤더 사용
  - 304 또는 네트워크 에러 시 로컬 캐시 fallback

### 2.3 iCalendar 처리 (TZ/반복/예외 – 중요)

1. **TZID / VTIMEZONE**
   - ICS 내 `VTIMEZONE` 정의를 파싱
   - `DTSTART;TZID=...` / `DTEND;TZID=...` 처리
   - `Z`(UTC) 시각, floating time, local time 구분
   - `config.Timezone`(예: `Asia/Seoul`) 기준으로 모든 occurrence를 **표시용 타임존**으로 변환
   - DST가 있는 타임존에서도 합리적으로 동작

2. **반복 이벤트 (RRULE)**
   - 최소 지원:
     - `FREQ=DAILY/WEEKLY/MONTHLY/YEARLY`
     - `BYDAY` / `BYMONTHDAY` / `INTERVAL` / `COUNT` / `UNTIL`
   - 필요한 기간에 대해서만 발생 인스턴스 생성:
     - `[now - backfill, now + horizon]` 범위 내 확장
     - `backfill`은 자정 걸치는 이벤트를 위해 필요

3. **예외 / override**
   - `EXDATE` 로 특정 occurrence 제거
   - `RECURRENCE-ID` 가진 VEVENT를 **override 인스턴스**로 처리:
     - `(UID, RECURRENCE-ID)` 키로 base occurrence 를 치환

4. **All-day 이벤트**
   - DATE vs DATE-TIME 구분
   - all-day는 local date 기준:
     - 시작: 00:00 local
     - 종료: 다음 날 00:00 (exclusive)

5. **UID 안정성 / 중복 제거**
   - 여러 캘린더 합칠 때:
     - `(calendarID/url, UID, recurrence-instance key)` 로 de-dup

6. **라이브러리 전략**
   - ICS 파서는 기존 Go 라이브러리 사용 (예: `arran4/golang-ical`)
   - RRULE 확장은:
     - `teambition/rrule-go` 등으로 RRULE string 변환/위임
     - 또는 필요한 subset만 직접 구현
   - TZID/EXDATE/RECURRENCE-ID는 반드시 처리 (완전하지 않더라도 명시적 동작/제한을 README에 기록)

### 2.4 Web UI

- HTTP 서버 (기본 `127.0.0.1:8080`)
- 페이지 / API:
  - `GET /` – HTML UI
  - `GET /api/config` – JSON 설정 조회
  - `POST /api/config` – JSON 설정 갱신
  - `POST /api/refresh` – 즉시 fetch + render + display
  - `POST /api/render` – fetch + render만 (디스플레이는 건드리지 않음)
  - `GET /preview.png` – 마지막 렌더링 preview 이미지
  - `GET /health` – healthcheck (인증 제외)
- 설정 항목:
  - ICS URL 추가/삭제
  - refresh interval (분)
  - timezone
  - 표시 옵션:
    - days range (예: 7일)
    - all-day 섹션 표시 여부
    - red highlight keywords
- 보안:
  - 기본 bind: `127.0.0.1:8080`
  - `--listen` 플래그로 override 허용
  - 선택적 Basic Auth:
    - 설정에서 username/password 지정 시 활성화
    - `/health` 제외 모든 엔드포인트 보호

### 2.5 설정 및 런타임 저장소

- 설정 파일: `/etc/epdcal/config.yaml` (플래그 `--config`로 변경 가능)
- 최초 실행 시:
  - config가 없다면 디폴트 생성
  - Web UI URL 출력
  - 파일 퍼미션: 0600
- 런타임 캐시:
  - `/var/lib/epdcal/` 내부:
    - ICS HTTP 메타/바디 캐시 (ETag, Last-Modified, body.ics)
    - 마지막 렌더링 결과 (preview.png, black.bin, red.bin 등)
- 개발/디버그 모드:
  - `--debug` 사용 시:
    - config: `./config.yaml`
    - 캐시: `./cache/...`

### 2.6 렌더링 (MVP)

- 핵심 목표:
  - 현재 날짜/요일
  - 향후 N일(기본 7일)의 이벤트 리스트
- 색상 사용:
  - **Red**:
    - config에서 지정한 키워드가 제목/설명에 포함된 이벤트
    - 주말/공휴일 강조(선택적)
- 구현:
  - 최종 UI 레이아웃은 Web UI(`/calendar`)에서 구현
  - Go에서는 headless Chromium으로 해당 페이지를 PNG로 캡처
  - PNG → black/red plane 변환
- 디버그:
  - `--dump`:
    - `preview.png`
    - `black.bin`
    - `red.bin` 등 아티팩트 출력

### 2.7 디스플레이 파이프라인

- cgo를 통해 C 드라이버 래핑:
  - `EPD_12in48B_Init()`
  - 필요 시 `EPD_12in48B_Clear()`
  - `EPD_12in48B_Display(black, red)`
  - `EPD_12in48B_Sleep()`
- Go `internal/epd` 패키지에서 고수준 API 제공:
  - Init / Clear / Display / Sleep
  - `--render-only` 옵션으로 실제 하드웨어 출력 비활성화 가능(개발/테스트용)

---

## 3. 비기능 요구사항

- 플랫폼:
  - Raspberry Pi (Linux ARM) 전용
  - 필요 시 build tag로 플랫폼 분기
- 성능:
  - `image.NRGBA`에 직접 인덱싱 (per-pixel `At()` 루프 금지)
  - packed buffer 변환은 single pass로 처리
- 신뢰성:
  - Fetch 에러로 데몬이 죽지 않아야 함
  - 마지막 성공 렌더링 이미지/버퍼를 유지
  - 로그:
    - 충분한 디버깅 정보
    - ICS URL은 절대 전체를 로그에 남기지 말 것 (redact)

---

## 4. 리포지토리 구조(제안)

```text
cmd/epdcal/main.go
internal/config/config.go
internal/web/web.go
internal/ics/fetch.go
internal/ics/parse.go
internal/ics/expand.go
internal/model/model.go
internal/convert/pack.go
internal/epd/epd.go
internal/epd/epd_cgo.go
internal/capture/chromium.go
waveshare/...                     # vendored C driver
README.md
progress.md
systemd/epdcal.service
```

---

## 5. 시간/타임존 정규화 전략

- 단일 **표시용 타임존**: `config.Timezone` (IANA, 예: `Asia/Seoul`)
- 각 이벤트의 시작/종료 파싱 규칙:
  - `DTSTART;TZID=Zone/...`:
    - 해당 TZID에 정의된 `VTIMEZONE` 정보 사용
  - `DTSTART:...Z`:
    - UTC로 인식 후 표시용 타임존으로 변환
  - DATE 타입(all-day):
    - local date 기준:
      - start: 00:00 local
      - end: 다음 날 00:00 local (exclusive)
- 모든 occurrence는 표시용 타임존으로 변환된 후 day별 그룹핑/렌더링

---

## 6. 반복(Recurrence) 확장

- VEVENT 단위 처리:
  - RRULE/RDATE 없음:
    - 단일 occurrence 생성
  - RRULE 존재:
    - 주어진 horizon window 내에서만 확장
- 예외 처리:
  - `EXDATE`:
    - base DTSTART와 동일한 타임존/UTC 기준으로 매칭, 해당 occurrence 제거
  - `RECURRENCE-ID`:
    - `(UID, RECURRENCE-ID timestamp)` 키로 override VEVENT 수집
    - base rule 확장 시 동일 키 발견 시:
      - base occurrence를 override 이벤트의 내용/시간으로 교체
- 무한루프 방지:
  - 확장 범위: `[rangeStart, rangeEnd]`
  - 이벤트당 occurrence 상한(예: 5000개)
  - 상한 초과 시 경고 로그 및 추가 truncation

---

## 7. 라이브러리 사용 계획

- ICS 파서:
  - 예: `github.com/arran4/golang-ical`
  - VCALENDAR / VEVENT / VTIMEZONE 파싱
- RRULE:
  - 예: `github.com/teambition/rrule-go`
  - ICS의 RRULE string을 rrule-go로 변환
  - UNTIL 값의 타임존/UTC 처리(RFC 5545 기준)를 최대한 준수
- 제한사항:
  - 특수/희귀 RRULE, 복잡한 BYSETPOS 등은 README에 명시적 "Known limitations"로 기술

---

## 8. CLI 인터페이스

`epdcal` 플래그:

- `--config /path/to/config.yaml`
- `--listen 127.0.0.1:8080`
- `--once` : 한 번 fetch+render(+display) 수행 후 종료
- `--render-only` : 디스플레이 하드웨어는 건드리지 않음
- `--dump` : `black.bin`, `red.bin`, `preview.png` 등 디버그 아티팩트 출력
- `--debug` : `/etc` / `/var/lib` 대신 로컬 `./config.yaml`, `./cache` 사용

---

## 9. 테스트 전략 (ICS Recurrence/TZ)

- 우선순위: breadth 보다 **정확성**
- `internal/ics/` 아래에 unit test 및 fixture 구성:
  - `internal/ics/testdata/`:
    - simple event
    - recurring weekly + EXDATE
    - override via RECURRENCE-ID
    - tzid event + utc event + all-day event
  - 테스트 파일:
    - `parse_test.go`
    - `expand_test.go`
- 검증 포인트:
  - 표시용 타임존으로 변환된 occurrence start/end timestamp
  - EXDATE 적용으로 제거된 occurrence
  - RECURRENCE-ID override 로 대체된 occurrence
  - multi-calendar merge 시 de-dup 로직

---

## 10. Acceptance Criteria

- 실제 ICS로 검증:
  - 주간 반복 미팅 + EXDATE
  - RECURRENCE-ID로 override 된 단일 occurrence
  - TZID를 가진 이벤트, UTC 이벤트, all-day 이벤트
- Preview / Display 결과:
  - 선택한 local timezone 기준으로 올바른 occurrence 표시
  - EXDATE로 제거된 occurrence가 나타나지 않아야 함
  - override 인스턴스가 중복 없이 정확히 한 번만 표시
- Web UI:
  - ICS URL 업데이트 후 서비스 재시작 없이 refresh 동작
  - `POST /api/refresh` / `POST /api/render` 가 기대대로 동작
- 보안:
  - OAuth 토큰, Google API, token.pickle 등 **전혀 사용하지 않음**

---

## 11. 앞으로의 작업(TODO, 고수준)

- [-] ICS fetch/parse/expand 기본 구현 및 unit test 완료  
      - **현황**: fetch/parse/expand 코어 로직 구현 완료, unit test/fixture는 미작성
- [-] TZID/VTIMEZONE 처리 및 표시용 타임존 변환 검증  
      - **현황**: 로직 구현은 되어 있으나 다양한 타임존/경계 케이스에 대한 테스트 미완료
- [x] Web UI(`/calendar`) 기본 레이아웃 및 이벤트 리스트 구현
- [x] headless Chromium(chromedp) 기반 PNG 캡처 구현  
      - `--once --dump`에서 `/calendar` → `preview.png` 캡처 테스트 경로 동작
- [ ] PNG → black/red packed plane 변환(`internal/convert/pack.go`)
- [ ] Waveshare C 드라이버를 cgo로 래핑(`internal/epd/`)
- [ ] display 파이프라인 통합 (fetch → expand → render → capture → pack → display)
- [ ] Web API (`/api/config`, `/api/refresh`, `/api/render`, `/preview.png`) 구현
- [ ] systemd 서비스 유닛(`systemd/epdcal.service`) 작성
- [x] README에 known limitations / troubleshooting 정리
