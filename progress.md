# EPD ICS Calendar – Progress

이 문서는 Raspberry Pi용 Waveshare 12.48" 3색 e‑paper(B) 패널(1304x984)을 구동하는 **ICS 기반 캘린더 디스플레이 + Web UI + Recurrence/TZ 처리** 프로젝트의 진행 상황과 설계 사항을 정리한 것이다.

---

## 0. 현재 구현 상태 요약 (2025-12-18 기준)

### 0.1 구현 상태

- 현재 레포지토리는 초기 상태이며, **구현 코드는 아직 작성되지 않았다.**
- 사용자 요구사항(아래 <Kilo Code Agent Prompt> 기반)을 바탕으로:
  - 전체 기능/비기능 요구사항
  - 아키텍처 방향성
  - 패키지 레이아웃
  - ICS Recurrence/TZ 처리 전략
  - 테스트/수용 기준
  를 정리한 설계 문서 단계이다.

### 0.2 향후 구현 대상(고수준)

1. **구성/부트스트랩**
   - `cmd/epdcal/main.go` 에서:
     - CLI 플래그 파싱 (`--config`, `--listen`, `--once`, `--render-only`, `--dump`)
     - config 로딩/기본값 생성(`/etc/epdcal/config.yaml`)
     - 주기 스케줄: 기본 15분, Web UI에서 변경 가능
     - EPD 초기화 및 종료 처리(cgo C 드라이버 래핑)
2. **ICS fetch/cache 파이프라인**
   - 다중 ICS URL에 대한 주기 fetch
   - ETag/Last-Modified 기반 HTTP 캐싱 + 로컬 캐시 fallback
3. **ICS parse + Recurrence/TZ 확장**
   - `VTIMEZONE`/`TZID`/RRULE/EXDATE/RECURRENCE-ID/all-day 이벤트 처리
   - `rrule-go` 등 활용하여 horizon 내 occurrence 확장
   - unit test(ICS fixture 기반)로 정확도 검증
4. **렌더링 파이프라인**
   - Go `image.NRGBA` 기반 텍스트 렌더링
   - 이벤트 리스트/날짜 헤더 등 레이아웃
   - `image.NRGBA → 1bpp packed buffer(black/red)` 변환
5. **EPD cgo 드라이버 래핑**
   - Waveshare C 드라이버(`EPD_12in48B.h`)를 cgo로 연결
   - Init/Clear/Display/Sleep 래퍼 함수 구현
6. **Web UI (단일 Go HTTP 서버)**
   - `/` 에 간단한 HTML UI 제공
   - Config 표시/수정, 수동 Refresh/Render, 상태 조회
   - `/preview.png` 로 마지막 렌더링 결과 PNG 제공
7. **설정/런타임 저장소**
   - `/etc/epdcal/config.yaml` (0600)
   - `/var/lib/epdcal/` 에 캐시/렌더링 아티팩트 저장
8. **systemd 서비스**
   - `systemd/epdcal.service` 작성 및 배포 가이드

---

## 1. 프로젝트 개요

- 단일 Go 애플리케이션 `epdcal` (Raspbian/ARM 대상)
- Waveshare 12.48" tri‑color e‑paper (B) 패널(1304x984)에 **캘린더**를 표시
- 하나 이상의 ICS(iCalendar) 구독 URL로부터 일정 데이터를 가져와:
  - 타임존/반복/예외/RECURRENCE-ID 를 고려하여 occurrence 확장
  - 설정된 local timezone 기준으로 이벤트를 그룹핑/표시
- 로컬 Web UI를 통해:
  - ICS URL/리프레시 간격/타임존/표시 옵션을 설정
  - 수동 Refresh 및 Render preview 트리거
  - 상태(마지막 갱신, 다음 스케줄, 최근 오류)를 확인
- Google API, OAuth, token.pickle, Python/PIL 등은 **일절 사용하지 않는다.**

---

## 2. 요구사항 정리

### 2.1 하드웨어 / 디스플레이 드라이버

- 대상 패널: Waveshare 12.48" tri-color e-paper (B), 해상도 1304x984
- 제공 C 헤더 `EPD_12in48B.h`:

  ```c
  UBYTE EPD_12in48B_Init(void);
  void EPD_12in48B_Clear(void);
  void EPD_12in48B_Display(const UBYTE *BlackImage, const UBYTE *RedImage);
  void EPD_12in48B_TurnOnDisplay(void);
  void EPD_12in48B_Sleep(void);
  ```

- C 함수 `EPD_12in48B_Display`에서 요구하는 버퍼:
  - width = 1304, height = 984
  - stride = 163 bytes per row (1304 / 8)
  - buffer size per plane = 163 * 984 = 160392 bytes
  - 인덱싱: `offset = y*163 + xByte`
  - BlackImage 바이트는 그대로 전송, RedImage 바이트는 전송 전에 `~`(bitwise NOT) 처리됨

- Go 측 bit packing 규칙:
  - 1bpp, MSB-first
  - 픽셀 (x, y)에 대해:
    - `byteIndex = y*163 + (x >> 3)`
    - `mask = 0x80 >> (x & 7)`
  - 초기 버퍼는 0xFF (white)로 채운다.
  - bit=0 → 잉크(black/red), bit=1 → white
  - Red plane의 의미:
    - Go에서는 `0 = red ink` semantics 유지
    - C 드라이버에서 전송 시 `~RedImageByte` 처리로 실제 패널에 red 인쇄

### 2.2 ICS 구독 (OAuth 미사용)

- 여러 개의 ICS URL을 지원
- fetch 트리거:
  - 주기적: 기본 15분마다
  - on-demand: Web UI에서 버튼을 눌러 수동 갱신
- HTTP 캐싱:
  - 각 ICS URL에 대해:
    - ETag, Last-Modified 값 저장
    - 요청 시 `If-None-Match` / `If-Modified-Since` 헤더 설정
  - 응답:
    - 200 OK: body를 저장 및 파싱
    - 304 Not Modified: 로컬 캐시 body 재사용
    - 네트워크/HTTP 에러 시:
      - 마지막으로 성공한 body 캐시가 있으면 fallback
      - 없을 경우 해당 소스는 스킵하되, 데몬은 계속 동작

### 2.3 iCalendar 처리 (Time Zone / Recurrence / 예외 – 중요)

1) **TZID / VTIMEZONE**

- ICS 내 `VTIMEZONE` 정의를 파싱
- `DTSTART;TZID=...`, `DTEND;TZID=...` 처리
- `Z`(UTC) suffix 가 있는 시간과 floating/local time을 구분
- 모든 occurrence는 설정된 `config.Timezone`(예: `Asia/Seoul`)으로 변환 후 사용
- DST가 있는 타임존에서도, RFC 5545에 최대한 맞게 동작

2) **반복 이벤트 (RRULE)**

- 최소 지원 RRULE:
  - `FREQ=DAILY`, `WEEKLY`, `MONTHLY`, `YEARLY`
  - `BYDAY`, `BYMONTHDAY`, `INTERVAL`, `COUNT`, `UNTIL`
- 필요한 기간에 대해서만 occurrence 확장:
  - `[now - backfill, now + horizon]` 범위에 대해 생성
  - `backfill`은 자정 경계를 넘는 이벤트 처리용 (예: 밤 11시 ~ 새벽 1시)

3) **예외 / override**

- `EXDATE`:
  - RRULE로 생성된 occurrence 중, 특정 날짜/시간을 제거
- `RECURRENCE-ID`:
  - `(UID, RECURRENCE-ID timestamp)` 키로 base occurrence를 찾아 override
  - override VEVENT는 해당 인스턴스의 시간/속성(요약/위치 등)을 대체

4) **All-day 이벤트**

- `DATE` 타입 vs `DATE-TIME` 타입을 구분
- All-day 이벤트는 표시용 타임존 기준 local date로 처리:
  - 시작: 00:00 local
  - 종료: 다음 날 00:00 local (exclusive)

5) **UID 안정성 / 중복 제거**

- 여러 ICS를 merge할 때 중복 제거:
  - 키: `(calendarID(or url), UID, recurrence-instance key)`
  - 동일 키로 중복되는 occurrence는 하나로 처리

### 2.4 Web UI (Simple)

- HTTP 서버 (기본 `127.0.0.1:8080`)
- 기능:
  - 설정 페이지:
    - ICS URL 추가/삭제
    - refresh interval (분 단위 또는 cron-like 설정)
    - timezone 설정
    - 표시 옵션:
      - days range (예: 향후 7일)
      - all-day 섹션 표시 여부
      - red highlight keywords
  - 액션 버튼:
    - “Refresh now”: ICS fetch + parse + expand + render + display
    - “Render preview”: fetch/parse/expand + render만 수행, EPD는 건드리지 않음
  - 상태 표시:
    - last refresh time
    - next scheduled refresh time
    - last error (있다면)

- HTTP Endpoints:
  - `GET /` : HTML UI
  - `GET /api/config` : JSON config 반환
  - `POST /api/config` : JSON으로 config 업데이트
  - `POST /api/refresh` : 즉시 fetch + render + display
  - `POST /api/render` : fetch + render만 수행
  - `GET /preview.png` : 마지막 렌더링 결과 PNG 제공
  - `GET /health` : 서비스 헬스 체크 (인증 제외)

- 보안:
  - 기본 bind: `127.0.0.1:8080`
  - 플래그 `--listen` 을 통해 bind 주소 override 허용
  - 선택적 Basic Auth:
    - 설정에 username/password 가 있으면 활성화
    - `/health` 를 제외한 모든 endpoint에 적용

### 2.5 설정 및 런타임 저장소

- 설정 파일:
  - 기본: `/etc/epdcal/config.yaml`
  - `--config` 플래그로 경로 override 가능
- 최초 실행 시:
  - config 파일이 없으면:
    - 기본값으로 파일 생성
    - 퍼미션 0600으로 설정
    - Web UI URL을 콘솔에 출력
- 런타임 캐시:
  - `/var/lib/epdcal/`:
    - ICS HTTP 메타 (ETag, Last-Modified)
    - ICS body 캐시 (`body.ics` 등)
    - 마지막 렌더링된 버퍼/이미지 (예: `preview.png`, packed plane)
- 개발/테스트용:
  - 필요한 경우 flag 또는 build tag로 경로를 로컬 디렉터리(`./config.yaml`, `./cache`)로 바꾸는 모드 제공 가능

### 2.6 렌더링 (Minimal MVP)

- 목표:
  - 현재 날짜 및 요일 표시
  - 향후 N일(기본 7일) 동안의 이벤트 리스트를 시간 순으로 표시
- 색상 사용 규칙:
  - Black plane:
    - 기본 텍스트, 선분, 테두리
  - Red plane:
    - 설정된 highlight keywords 를 포함하는 이벤트
    - (선택) 주말/공휴일 날짜 강조
- 구현 방식:
  - Go에서 `image.NRGBA`를 사용해 레이아웃/텍스트를 직접 그린다.
    - 폰트: `x/image/font/opentype` 로 TTF 로드
  - 렌더가 완료된 `image.NRGBA`를 packed black/red plane으로 변환
- 디버그:
  - `--dump` 옵션:
    - `preview.png`
    - `black.bin`
    - `red.bin`
    등을 지정된 디렉터리에 출력

### 2.7 디스플레이 파이프라인

- cgo를 통한 C 드라이버 호출:
  - `EPD_12in48B_Init()`
  - 필요 시 `EPD_12in48B_Clear()`
  - `EPD_12in48B_Display(black, red)`
  - `EPD_12in48B_TurnOnDisplay()`
  - `EPD_12in48B_Sleep()`
- Go `internal/epd` 패키지:
  - 위 C 함수의 thin wrapper를 제공
  - 초기화/에러 핸들링/리소스 정리를 Go 측에서 관리

---

## 3. 비기능 요구사항

- 플랫폼:
  - Raspberry Pi (Linux ARM) 전용
  - 필요 시 build tag를 사용해 플랫폼 분리
- 성능:
  - `image.NRGBA`의 픽셀 접근은 직접 인덱싱으로 처리 (`At()` 반복 호출 지양)
  - packed buffer 변환은 단일 패스에서 완료
- 안정성:
  - 네트워크/ICS fetch 에러로 데몬이 종료되면 안 됨
  - 마지막으로 성공한 렌더링 이미지를 유지
- 로깅:
  - 충분한 디버깅 정보 제공
  - ICS URL 전체를 로그에 남기지 않고, 일부만 또는 해시/식별자 형태로만 기록

---

## 4. 리포지토리 구조 (제안)

```text
cmd/epdcal/main.go
internal/config/config.go
internal/web/web.go
internal/ics/fetch.go
internal/ics/parse.go
internal/ics/expand.go
internal/model/model.go
internal/render/render.go
internal/convert/pack.go
internal/epd/epd.go
internal/epd/epd_cgo.go
waveshare/...               # vendored C driver
README.md
progress.md
systemd/epdcal.service
```

---

## 5. 시간/타임존 정규화 전략

- 단일 **표시용 타임존**: `config.Timezone` (IANA 형식, 예: `Asia/Seoul`)
- 각 이벤트의 시작/종료 파싱:
  - `DTSTART;TZID=Zone/...`:
    - 해당 TZID에 대응하는 `VTIMEZONE` 정의 사용
  - `DTSTART:...Z`:
    - UTC 시각으로 인식 후 표시용 타임존으로 변환
  - floating time (TZID/`Z` 없음):
    - 캘린더/이벤트의 기본 타임존 정의에 따라 해석 (필요 시 설정값 또는 시스템 로캘 사용)
  - `DATE` 타입(all-day):
    - 표시용 타임존 기준:
      - `start = YYYY-MM-DD 00:00`
      - `end = 다음 날 00:00` (exclusive)
- 모든 occurrence는 표시용 타임존으로 변환 후 day별 그룹핑/정렬에 사용

---

## 6. 반복(Recurrence) 확장 전략

- VEVENT 단위 처리 로직:
  - RRULE/RDATE 없음:
    - 단일 occurrence 생성
  - RRULE 있음:
    - RRULE을 파싱 후 지정된 horizon window 내에서만 occurrence 생성
- 예외 처리:
  - `EXDATE`:
    - base DTSTART와 같은 기준(타임존/UTC)으로 비교하여 해당 occurrence 제거
  - `RECURRENCE-ID`:
    - `(UID, RECURRENCE-ID timestamp)` 키로 override VEVENT를 수집
    - base rule 확장 시 동일 키의 occurrence를 발견하면:
      - 해당 occurrence를 override VEVENT 내용으로 교체
- 무한 루프/폭발 방지:
  - 확장 범위: `[rangeStart, rangeEnd]`
  - 이벤트당 occurrence 상한 (예: 5000건)
  - 상한 초과 시:
    - 로그에 경고를 남기고, 이후 occurrence는 잘라낸다.

---

## 7. 라이브러리 사용 계획

- ICS 파서:
  - 예: `github.com/arran4/golang-ical`
  - VCALENDAR / VEVENT / VTIMEZONE 파싱
- RRULE 처리:
  - 예: `github.com/teambition/rrule-go`
  - ICS RRULE string을 rrule-go 객체로 변환하여 사용
  - UNTIL의 타임존/UTC 처리 시 RFC 5545에 최대한 부합하도록 구현
- 폰트/렌더링:
  - 표준 Go 이미지 패키지 + `golang.org/x/image/font/opentype`
- 스케줄링:
  - 기본 최소 구현은 `time.Ticker` 또는 `time.AfterFunc` 기반
  - 필요 시 cron 라이브러리(`robfig/cron/v3`)를 도입해 표현력 있는 스케줄 지원

---

## 8. CLI 인터페이스 (초기 스펙)

`epdcal` 실행 플래그:

- `--config /path/to/config.yaml` : 설정 파일 경로
- `--listen 127.0.0.1:8080` : Web UI/HTTP 서버 bind 주소
- `--once` :
  - 한 번 `fetch + render + display` 를 수행한 뒤 종료
- `--render-only` :
  - EPD 하드웨어는 건드리지 않고 PNG/preview만 생성
- `--dump` :
  - `black.bin`, `red.bin`, `preview.png` 등 디버그 아티팩트 출력
- (옵션) `--log-level`, `--debug` 등 필요 시 추가

---

## 9. 테스트 전략 (ICS Recurrence/TZ)

- 우선순위: 기능 범위 확대보다 **정확한 Recurrence/TZ 동작** 검증을 우선한다.
- `internal/ics/` 아래에 unit test 및 fixture 구성:

  - `internal/ics/testdata/`:
    - `simple_event.ics`:
      - 단일 non-recurring 이벤트
    - `weekly_with_exdate.ics`:
      - 주간 반복 + 특정 날짜 EXDATE
    - `override_with_recurrence_id.ics`:
      - base RRULE + 특정 occurrence override (RECURRENCE-ID)
    - `tzid_utc_allday_mix.ics`:
      - TZID가 있는 이벤트
      - UTC(`Z`) 이벤트
      - all-day 이벤트 혼합

  - 테스트 파일:
    - `parse_test.go`
    - `expand_test.go`

- 검증 포인트:
  - 표시용 타임존으로 변환된 occurrence `Start`/`End` 타임스탬프가 기대값과 일치
  - EXDATE로 지정된 occurrence가 결과 집합에서 제거
  - RECURRENCE-ID override 인스턴스가 base occurrence를 올바르게 대체
  - 여러 캘린더를 합쳤을 때 UID/instance 키 기반 de-dup이 제대로 작동

---

## 10. Acceptance Criteria

- 준비한 실제 ICS (또는 테스트 fixture)를 사용했을 때:
  - 주간 반복 미팅 + EXDATE:
    - EXDATE로 제거된 occurrence가 화면에 보이지 않는다.
  - RECURRENCE-ID로 override 된 occurrence:
    - base 인스턴스 대신 override 정의가 1회만 표시된다.
  - TZID 이벤트, UTC 이벤트, all-day 이벤트:
    - 설정된 local timezone 기준으로 올바른 날짜/시간에 표시된다.
- Web UI:
  - ICS URL 및 refresh interval/timezone/표시 옵션을 Web UI에서 변경 후:
    - 서비스 재시작 없이 설정이 반영되고 스케줄이 동작
  - `POST /api/refresh`:
    - 즉시 fetch + render + display 수행
  - `POST /api/render`:
    - display를 건드리지 않고 렌더/preview만 업데이트
- 보안/정책:
  - OAuth, Google API, token.pickle, Python/PIL 등은 코드베이스 어디에서도 사용되지 않는다.

---

## 11. 앞으로의 작업 (TODO, 고수준)

- [ ] `cmd/epdcal/main.go` 스켈레톤 구현
  - [ ] 플래그 파싱 및 config 로딩
  - [ ] Web 서버 시작 및 기본 핸들러 등록
  - [ ] once/daemon 모드 제어 로직
- [ ] `internal/config` 구현
  - [ ] `/etc/epdcal/config.yaml` 기본값 생성 + 0600 퍼미션
  - [ ] refresh interval / timezone / ICS URL 목록 / 표시 옵션 구조 정의
- [ ] `internal/ics` 구현
  - [ ] fetch + HTTP 캐시 (ETag/Last-Modified) 처리
  - [ ] parse + Recurrence/TZ/EXDATE/RECURRENCE-ID 처리 코어 로직
  - [ ] testdata + unit test (`parse_test.go`, `expand_test.go`)
- [ ] `internal/render` 구현
  - [ ] `image.NRGBA` 기반 레이아웃 및 텍스트 렌더링
  - [ ] all-day 섹션/일반 이벤트 리스트 렌더링
- [ ] `internal/convert/pack.go` 구현
  - [ ] `image.NRGBA → black/red packed plane` 변환 (1 pass)
  - [ ] 1304x984 해상도/stride 대응
- [ ] `internal/epd` cgo 래퍼 구현
  - [ ] Waveshare C 드라이버(vendored) 포함
  - [ ] Init/Clear/Display/Sleep 래핑
- [ ] Web UI / HTTP API
  - [ ] `GET /` HTML UI
  - [ ] `GET/POST /api/config`
  - [ ] `POST /api/refresh`
  - [ ] `POST /api/render`
  - [ ] `GET /preview.png`
  - [ ] Basic Auth 옵션
- [ ] 런타임 캐시/에러 핸들링
  - [ ] 마지막 성공 렌더링 preview/planes 유지
  - [ ] fetch/render 실패 시 graceful fallback
- [ ] `systemd/epdcal.service` 작성 및 README에 설치/운영 가이드 추가

이 문서는 구현 진행에 따라 지속적으로 업데이트되며, 특히 **ICS Recurrence/TZ 처리와 관련된 테스트 결과/제한사항**을 중심으로 보완될 예정이다.
