# EPD ICS Calendar – Progress

이 문서는 Raspberry Pi용 Waveshare 12.48" 3색 e‑paper(B) 패널(1304x984)을 구동하는 **ICS 기반 캘린더 디스플레이 + Web UI + Recurrence/TZ 처리** 프로젝트의 진행 상황과 설계 사항을 정리한 것이다.

---

## 0. 현재 상태 요약 (2025-12-18 기준)

### 0.1 이미 구현/연결된 부분 (DONE)

아래 항목들은 **현재 레포에 코드가 존재하고, 기본 동작 플로우가 연결된 상태**이다.

- **CLI / 메인 루프**
  - 엔트리 포인트: `cmd/epdcal/main.go`
  - 주요 플래그:
    - `--config` : 설정 파일 경로
    - `--listen` : HTTP 바인드 주소 override
    - `--once` : 1회 실행 모드 (fetch + render + capture + display 후 종료)
    - `--render-only` : 디스플레이 하드웨어 미사용 (preview/capture만)
    - `--dump` : `preview.png`, `black.bin`, `red.bin` 등 디버그 아티팩트 출력
    - `--debug` : `/etc` / `/var/lib` 대신 로컬 `./config.yaml`, `./cache` 사용
  - SIGINT/SIGTERM 처리 및 context 취소
  - 주기 실행:
    - `robfig/cron/v3` 사용
    - `config.refresh` (cron string) 기준으로 정시 스케줄 동작 (`*/15 * * * *` 등)
  - 메인 플로우:
    - cron/once 트리거 → ICS fetch/parse/expand → `/calendar` 캡처 → PNG → packed plane → EPD 표시

- **설정 로딩 및 기본값 처리**
  - `internal/config/config.go`
  - 기본 config 경로: `/etc/epdcal/config.yaml` (플래그로 override 가능)
  - `--debug` 시 `./config.yaml` 사용
  - 초기 config 자동 생성 및 퍼미션 0600 설정 로직 구현
  - 스케줄 필드:
    - `refresh` (cron string) – 메인 스케줄
    - `refresh_minutes` – 레거시 필드; 값이 존재하고 `refresh`가 비어 있으면 `"*/N * * * *"` 로 변환 후 `refresh`에 반영

- **로깅**
  - `internal/log/log.go`
  - leveled logger (Info/Error 등) 사용

- **ICS Fetch + HTTP 캐시**
  - `internal/ics/fetch.go`
  - 기능:
    - 다중 ICS 소스에 대한 fetch
    - ETag / Last-Modified 저장
    - If-None-Match / If-Modified-Since 요청 헤더 사용
    - 304 / 네트워크 에러 시 로컬 캐시 body.ics fallback
  - 캐시 디렉터리:
    - 기본: `/var/lib/epdcal/ics-cache`
    - `--debug`: `./cache/ics-cache`

- **ICS Parse / Recurrence 확장 (코어 로직)**
  - `internal/ics/parse.go`
    - VEVENT/VTIMEZONE 파싱
    - DTSTART/DTEND/TZID/EXDATE/RECURRENCE-ID/UID/RRULE 필드 파싱
  - `internal/ics/expand.go`
    - RRULE(`FREQ`, `BYDAY`, `BYMONTHDAY`, `INTERVAL`, `COUNT`, `UNTIL` 등) 확장
    - `EXDATE` 적용
    - `RECURRENCE-ID` 기반 override 적용
    - horizon window 내에서만 occurrence 생성, 이벤트당 상한 적용
  - `internal/model/model.go`
    - `Occurrence` 등 공통 모델 정의
  - **상태**:
    - fetch/parse/expand 코어 로직은 구현/연결 완료
    - fixture 기반 unit test는 아직 작성 중 (TODO, 아래 0.2/11에서 별도 명시)

- **Web 서버 및 API**
  - `internal/web/web.go`
  - 구현된 엔드포인트:
    - `GET /health` – 헬스체크
    - `GET /api/events` – ICS fetch/parse/expand 결과를 기반으로 occurrence JSON 반환
    - `GET /api/battery` – 배터리 상태 JSON 반환 (I2C 또는 mock)
    - `GET /preview.png` – 마지막 캡처된 Preview PNG 서빙
      - 기본: `/var/lib/epdcal/preview.png`
      - `--debug`: `./cache/preview.png`
  - 정적 Web UI (Next.js 빌드 결과)를 embed FS 로 서빙:
    - `/`, `/calendar` 등은 정적 HTML로 제공
    - `/api/*`, `/preview.png` 등은 정적 서빙에서 제외

- **배터리 리더 (I2C + mock)**
  - `internal/battery/battery.go`
  - `Reader` 인터페이스 및 `Status` 구조체 정의
  - **mock 리더**:
    - 개발용, 20~100% 사이 랜덤 percent, voltage_mv=0
  - **I2C 리더**:
    - periph.io 기반으로 특정 I2C 주소(예: PiSugar3 호환)에서 전압/퍼센트 읽기
  - `DefaultReader()`:
    - Linux + I2C 가 정상 동작하면 I2C 리더 사용
    - 그 외에는 mock 리더로 fallback
  - `/api/battery` 는 위 `DefaultReader()` 에 기반해 동작

- **Web UI `/calendar` 페이지**
  - `webui/src/app/calendar/page.tsx` (Next.js, App Router)
  - 특징:
    - 고정 캔버스 크기: 1304x1200 레이아웃 (EPD 비율에 맞춤)
    - `/api/events` 호출:
      - occurrence 를 날짜별로 그룹핑
      - 월간 그리드(5주/6주) 생성
      - 오늘/주말/다른 달 여부에 따라 스타일링
    - `/api/battery` 호출:
      - 배터리 퍼센트에 따라 5단계 Font Awesome 아이콘(`battery-empty`..`battery-full`) 표시
    - `data-ready` 속성:
      - `/api/events` + `/api/battery` 두 요청이 모두 성공하면 root div 에 `data-ready="true"` 설정
      - headless Chromium 캡처에서 이 속성을 기준으로 렌더 완료 여부 판단

- **Chromium 캡처 헬퍼 및 캡처 파이프라인**
  - `internal/capture/chromium.go`
    - `CaptureCalendarPNG(opts)`:
      - viewport (기본 1304x1200) 설정
      - `/calendar` 페이지로 네비게이션
      - `[data-ready="true"]` element 가 visible 될 때까지 대기
      - full screenshot (PNG) 파일로 저장
  - `cmd/epdcal/main.go` 의 `runCapturePipeline`:
    - refresh cycle 이후 `/calendar` 캡처
    - PNG 경로:
      - 기본: `/var/lib/epdcal/preview.png`
      - `--debug`: `./cache/preview.png`
    - PNG 디코드 (`image/png` → `image.NRGBA` 변환)
    - `internal/convert/pack.go` 로 black/red packed plane 변환
    - `--dump` 옵션 시 black/red plane 을 `black.bin` / `red.bin` 으로 함께 저장
    - `--render-only` 가 아니고 EPD 드라이버가 활성화된 경우, plane 을 EPD 로 전송

- **PNG → black/red packed plane 변환**
  - `internal/convert/pack.go`
  - 입력: `*image.NRGBA` (폭 1304, 높이 ≥ 984)
  - 처리:
    - 높이가 984보다 크면 중앙 기준으로 세로 crop (1304x984)
    - black/red plane 버퍼를 0xFF(white) 로 초기화
    - 각 픽셀에 대해:
      - alpha < 128 → white
      - 밝기(Y)와 redness 를 계산하여:
        - 충분히 어두우면 black plane bit=0
        - 충분히 붉으면 red plane bit=0
        - 그 외는 white 유지
  - 출력:
    - `black []byte`, `red []byte` (각 160,392 바이트)

- **EPD SPI 드라이버 (periph.io 기반, Waveshare C SDK 포팅)**
  - `internal/epd/epd_spi.go`
  - periph.io (`host/v3`, `conn/v3/spi`, `conn/v3/gpio`) 기반으로 순수 Go 구현
  - 기능:
    - SPI + GPIO 초기화 (`Init(ctx)`)
      - BCM 핀 매핑 (C `DEV_Config.h` 기반) 예:
        - CS: 8(M1), 7(S1), 17(M2), 18(S2)
        - DC: 13(M1S1), 22(M2S2)
        - RST: 6(M1S1), 23(M2S2)
        - BUSY: 5(M1), 19(S1), 27(M2), 24(S2)
    - DEV 계층 포팅:
      - `digitalWrite`, `digitalRead`, `delayUs`, `delayMs`
      - `spiWriteByte` 등
    - EPD 시퀀스 (C `EPD_12in48B_*.c` 기반) 포팅:
      - Reset / SendCommand / SendData 헬퍼
      - segment 별 busy-wait (`m1ReadBusy`, `m2ReadBusy`, `s1ReadBusy`, `s2ReadBusy`)
      - LUT 테이블 로딩
      - 고수준 메서드:
        - `InitPanel()` (전원/부팅/LUT 설정)
        - `Clear()`
        - `Display(black, red []byte)`
        - `TurnOnDisplay()`
        - `Sleep()`
  - 현재:
    - cron/once 마다 `/calendar` → PNG → packed plane → EPD Display까지 end-to-end 파이프라인 동작

- **Makefile / 설치 스크립트**
  - `Makefile`
  - 주요 타깃:
    - `webui-build` : Next.js `webui` 빌드 후 `internal/web/static` 으로 export 복사
    - `build` : webui-build + Go 빌드 → `epdcal` 바이너리 생성
    - `build-pi`, `build-pi64` : cross build (linux/arm, linux/arm64)
    - `test` : `go test ./...`
    - `run` : `./epdcal --render-only --dump` 실행
    - `install` : `build` + `systemd-install`
    - `systemd-install` :
      - `$(PREFIX)/bin` 에 바이너리 설치
      - `$(ETCDIR)` (`/etc/epdcal`), `$(VARLIB)` (`/var/lib/epdcal`) 디렉터리 생성 및 권한 설정
      - `systemd/epdcal.service` 를 `$(SYSTEMD_DIR)` 에 설치
- **systemd 유닛**
  - `systemd/epdcal.service`
  - 내용 요약:
    - `ExecStart=/usr/local/bin/epdcal --config /etc/epdcal/config.yaml`
    - `WorkingDirectory=/var/lib/epdcal`
    - `User=pi`, `Group=pi`
    - `Restart=on-failure`
    - `WantedBy=multi-user.target`
  - 설치/실행 예시는 `README.md` 에 명시

---

### 0.2 앞으로 구현/보완해야 할 부분 (TODO)

아래는 **아직 미완료이거나, 기본 구현은 있으나 보완이 필요한 작업들**을 정리한 것이다.

- **ICS Recurrence/TZ 단위 테스트**
  - `internal/ics/testdata/*.ics` fixture 작성
    - simple event
    - weekly recurring + EXDATE
    - override via RECURRENCE-ID
    - TZID event + UTC event + all-day event 혼합
  - `parse_test.go`, `expand_test.go` 에서:
    - 표시용 타임존 변환 후 occurrence start/end 검증
    - EXDATE/RECURRENCE-ID 적용 검증
    - multi-calendar merge 및 de-dup 검증

- **TZID/VTIMEZONE 및 DST 경계 테스트 보완**
  - 현재 로직은 구현되어 있으나:
    - 다양한 실제 타임존(예: 미국/유럽 DST 전환 구간)에 대한 케이스를 fixture로 만들어 정확도 검증 필요

- **설정/관리용 Web API 확장**
  - 아직 미구현 또는 스펙 수준에 머무른 항목:
    - `GET /api/config` : 현재 config JSON 조회
    - `POST /api/config` : ICS URL/refresh/timezone/표시 옵션 등 업데이트
    - `POST /api/refresh` : 즉시 fetch + render + display 트리거
    - `POST /api/render` : fetch + render만 (EPD 미출력)

- **런타임 캐시 및 에러 핸들링 고도화**
  - 현재:
    - `/preview.png` 는 마지막 캡처 이미지를 파일 기반으로 서빙
  - 추가 목표:
    - 마지막 성공 렌더링된 packed plane/PNG 를 별도 보관
    - 새 capture/EPD 업데이트 실패 시:
      - 이전 성공 프레임으로 graceful fallback
    - ICS fetch/parse/expand 단계와 `/api/events` 제공 사이의 중복 작업 최소화
      - 필요 시 in-memory 캐시 또는 shared storage 도입

- **Basic Auth 보안 옵션**
  - config 에 Basic Auth 필드 정의는 되어 있으나:
    - 실제 HTTP 핸들러에 적용/미적용을 분기하는 미들웨어 구현 필요
    - `/health` 만 예외로 두고 나머지 `/`, `/api/*`, `/preview.png` 에 인증 적용

- **문서/디버그 편의성 보완**
  - README:
    - 현실 동작 기준으로 Recurrence/TZ 처리 예제 추가 (실제 ICS 예시와 기대 표시 결과)
  - progress.md:
    - 테스트 진행상황, 실제 하드웨어 동작 확인 결과 등을 향후 업데이트

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
- (설계 스펙) C 코드(`EPD_12in48B.c/.h`)를 레퍼런스로 삼아 Go 기반 드라이버 구현
  - 실제 구현은 periph.io 기반 SPI 드라이버 (`internal/epd/epd_spi.go`) 로 포팅
- 버퍼 사양:
  - width = 1304, height = 984
  - stride = 163 bytes/row (1304 / 8)
  - plane buffer size = 163 * 984 = 160,392 bytes
  - 인덱싱: `offset = y*163 + xByte`
- Go 쪽 bit packing:
  - MSB-first 1bpp
  - 픽셀 (x, y)에 대해:
    - `byteIndex = y*163 + (x >> 3)`
    - `mask = 0x80 >> (x & 7)`
  - 초기 버퍼는 0xFF(white)로 채움
  - bit=0 → 잉크(black/red), bit=1 → white
  - Red plane은 Go에서 `0=red` 로 유지, 패널로 전송 시 C 레퍼런스 구현과 동일한 의미가 되도록 전송 시점에서 반전 처리

### 2.2 ICS 구독 (OAuth 금지)

- 여러 ICS URL 지원
- 주기적 fetch (cron 스케줄 기반) + Web UI에서 on-demand fetch
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
  - `GET /api/config` – JSON 설정 조회 (TODO)
  - `POST /api/config` – JSON 설정 갱신 (TODO)
  - `POST /api/refresh` – 즉시 fetch + render + display (TODO)
  - `POST /api/render` – fetch + render만 (디스플레이는 건드리지 않음, TODO)
  - `GET /preview.png` – 마지막 렌더링 preview 이미지 (DONE)
  - `GET /health` – healthcheck (인증 제외, DONE)
- 설정 항목:
  - ICS URL 추가/삭제
  - refresh 스케줄 (cron string: 예 `*/15 * * * *`)
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
    - `/health` 제외 모든 엔드포인트 보호 (구현 TODO)

### 2.5 설정 및 런타임 저장소

- 설정 파일: `/etc/epdcal/config.yaml` (플래그 `--config`로 변경 가능)
- 최초 실행 시:
  - config가 없다면 디폴트 생성
  - Web UI URL 출력
  - 파일 퍼미션: 0600
- 런타임 캐시:
  - `/var/lib/epdcal/` 내부:
    - ICS HTTP 메타/바디 캐시 (ETag, Last-Modified, body.ics)
    - 마지막 렌더링 결과 (preview.png, packed plane 등)
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

- periph.io 기반 SPI 드라이버로 Waveshare 패널 구동:
  - Init/Reset → LUT 설정 → Display → Sleep
- Go `internal/epd` 패키지에서 고수준 API 제공:
  - InitPanel / Clear / Display / Sleep
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

## 4. 리포지토리 구조(제안/현재)

```text
cmd/epdcal/main.go
internal/config/config.go
internal/web/web.go
internal/ics/fetch.go
internal/ics/parse.go
internal/ics/expand.go
internal/model/model.go
internal/convert/pack.go
internal/epd/epd_spi.go
internal/capture/chromium.go
internal/battery/battery.go
waveshare/...                     # (초기 개발 시 참고용 C SDK, 빌드에는 미사용 가능)
README.md
progress.md
systemd/epdcal.service
webui/...                         # Next.js Web UI 소스
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
- 스케줄링:
  - `github.com/robfig/cron/v3`:
    - `config.refresh` cron string을 기반으로 주기 실행
    - 타임존(Location) 지정

---

## 8. CLI 인터페이스

`epdcal` 플래그:

- `--config /path/to/config.yaml`
- `--listen 127.0.0.1:8080`
- `--once` : 한 번 fetch+render(+capture/display) 수행 후 종료
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
  - ICS URL 및 refresh cron 업데이트 후 서비스 재시작 없이 refresh 동작
  - `POST /api/refresh` / `POST /api/render` 가 기대대로 동작
- 보안:
  - OAuth 토큰, Google API, token.pickle 등 **전혀 사용하지 않음**

---

## 11. 작업 리스트 (DONE / TODO 정리)

### 11.1 DONE

- [x] CLI 플래그 및 메인 루프 스켈레톤 (`cmd/epdcal/main.go`)
- [x] config 로딩/저장 및 기본값 처리 (`internal/config`)
- [x] ICS fetch + HTTP 캐시 (`internal/ics/fetch.go`)
- [x] ICS parse + recurrence/TZ/EXDATE/RECURRENCE-ID 코어 로직 (`internal/ics/parse.go`, `internal/ics/expand.go`)
- [x] 공통 모델 정의 (`internal/model/model.go`)
- [x] Web UI(`/calendar`) 기본 레이아웃 및 이벤트 리스트 구현 (`webui/...`)
- [x] headless Chromium(chromedp) 기반 PNG 캡처 (`internal/capture/chromium.go`)
- [x] 주기 스케줄을 cron string(`config.refresh`) 기반으로 변경 (`robfig/cron/v3`)
- [x] PNG → black/red packed plane 변환(`internal/convert/pack.go`)
- [x] Waveshare C 드라이버를 참고하여 periph.io 기반 SPI 구현(`internal/epd/epd_spi.go`)
- [x] display 파이프라인 통합 (cron/once → `/calendar` 캡처 → PNG → pack → EPD Display)
- [x] Preview 이미지 HTTP 서빙 (`GET /preview.png`)
- [x] 배터리 mock + I2C 리더 및 `/api/battery` 연동 (`internal/battery/battery.go`)
- [x] 설치용 Makefile 타깃 (`install`, `systemd-install`)
- [x] systemd 서비스 유닛 (`systemd/epdcal.service`) 작성
- [x] README 에 설치/구동/known limitations / troubleshooting 정리

### 11.2 TODO (우선순위)

- [-] ICS fetch/parse/expand unit test 및 fixture
  - [ ] `internal/ics/testdata/*.ics` 작성
  - [ ] `parse_test.go` / `expand_test.go` 에서 Recurrence/TZ/EXDATE/RECURRENCE-ID 검증
- [ ] 다양한 TZID/VTIMEZONE 및 DST 경계 케이스에 대한 추가 테스트
- [ ] Web API (`/api/config`, `/api/refresh`, `/api/render`) 구현 및 Web UI 연동
- [ ] Basic Auth 미들웨어 구현 및 `/health` 제외 전 엔드포인트 보호
- [ ] 런타임 캐시 고도화:
  - [ ] 마지막 성공 렌더링된 packed plane/PNG 저장
  - [ ] 새 렌더/EPD 전송 실패 시 fallback 메커니즘
- [ ] 로그/문서 보완:
  - [ ] ICS Recurrence/TZ 예제와 실제 화면 캡처를 README/progress에 추가

이 문서는 구현 진행에 따라 지속적으로 업데이트되며, 특히 **ICS Recurrence/TZ 처리와 관련된 테스트 진행 상황 및 제한사항**을 중심으로 보완될 예정이다.
