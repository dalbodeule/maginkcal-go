# epdcal – ICS 기반 E-Paper 캘린더 (Raspberry Pi / Waveshare 12.48" B)

Raspberry Pi (Raspbian)에서 구동되는 Go 기반 데몬으로, Waveshare 12.48" tri-color e-paper (B) 패널(1304x984)에 **ICS(iCalendar) 구독**으로부터 가져온 캘린더를 렌더링하여 표시합니다.  
Google API / OAuth / Python / PIL 은 사용하지 않습니다.

- ICS URL 여러 개 구독
- 견고한 타임존 / 반복 일정(RRULE, EXDATE, RECURRENCE-ID 등) 처리
- 로컬 Web UI 로 설정 / 미리보기 / 수동 갱신
- 1bpp black/red plane 으로 패널 구동 (Waveshare C 드라이버 + cgo)
- systemd 서비스로 항상 실행

---

## 1. 기능 개요

### 1.1 ICS 구독 (OAuth 없음)

- 하나 이상의 ICS URL 을 설정 파일 또는 Web UI 에서 등록
- 주기적으로(기본 15분) 및 Web UI 의 "Refresh now" 버튼으로 on-demand 동기화
- HTTP 캐시 지원:
  - `ETag` / `Last-Modified` 저장
  - `If-None-Match` / `If-Modified-Since` 헤더 전송
  - 304 응답 시 이전 본문 재사용
- 네트워크/서버 오류 발생 시:
  - 데몬은 크래시 없이 계속 동작
  - 마지막으로 성공한 렌더링 이미지를 유지 및 재사용

구현 파일(계획):

- [`internal/ics/fetch.go`](internal/ics/fetch.go:1) – ICS 다운로드 및 HTTP 캐싱
- [`internal/ics/parse.go`](internal/ics/parse.go:1) – ICS 파싱 (VEVENT/VTIMEZONE 등)
- [`internal/ics/expand.go`](internal/ics/expand.go:1) – 반복 일정/예외/오버라이드 확장

### 1.2 타임존 및 iCalendar 처리

목표: **정확성을 우선**으로, 일반적인 사용 사례에서 안정적으로 동작하는 ICS 처리.

주요 요구 사항:

1. **TZID / VTIMEZONE**
   - ICS 내 `VTIMEZONE` 블록을 파싱
   - `DTSTART;TZID=...`, `DTEND;TZID=...` 를 해당 시간대로 해석
   - `Z`(UTC) 종결 및 floating time(타임존 없는 날짜-시간) 처리
   - 최종적으로 모든 일정은 설정된 `local_timezone`(예: `Asia/Seoul`) 로 변환하여 표시
   - DST(서머타임) 존재하는 지역에서도 합리적으로 동작

2. **반복 일정 (RRULE)**
   - 최소 지원:
     - `FREQ=DAILY/WEEKLY/MONTHLY/YEARLY`
   - 추가 지원:
     - `BYDAY`, `BYMONTHDAY`, `INTERVAL`, `COUNT`, `UNTIL`
   - 필요한 기간에 대해서만 확장:
     - 예: `[now - backfill, now + horizon]` (backfill 1일, horizon N일)

3. **예외 / 오버라이드**
   - `EXDATE` 로 특정 발생(occurrence) 제거
   - `RECURRENCE-ID` 를 사용한 개별 occurrence 재정의:
     - (UID, RECURRENCE-ID timestamp) 를 키로 override 이벤트 수집
     - 기본 RRULE 확장 결과에서 해당 occurrence 를 override 이벤트로 교체

4. **하루 종일(all-day) 이벤트**
   - `DATE` 값과 `DATE-TIME` 값을 구분
   - all-day 일정은 display timezone 기준 `[해당 날짜 00:00, 다음날 00:00)` 범위로 해석

5. **UID 안정성 / 중복 제거**
   - 여러 캘린더(ICS URL)에서 가져온 이벤트를 통합
   - 키: `(calendarID/url, UID, recurrence-instance key)` 조합으로 중복 제거

핵심 로직은:

- [`internal/ics/parse.go`](internal/ics/parse.go:1)
- [`internal/ics/expand.go`](internal/ics/expand.go:1)
- [`internal/model/model.go`](internal/model/model.go:1)

에 구현됩니다.

### 1.3 Web UI

내장 HTTP 서버를 통해 간단한 관리 UI 를 제공합니다.

- 기본 리슨 주소: `127.0.0.1:8080` (CLI `--listen` 으로 변경 가능)
- Endpoints:
  - `GET /` – HTML 설정/상태 페이지
  - `GET /api/config` – JSON 설정 조회
  - `POST /api/config` – JSON 설정 갱신
  - `POST /api/refresh` – ICS fetch + render + display 강제 실행
  - `POST /api/render` – fetch + render 만 실행 (EPD 미사용)
  - `GET /preview.png` – 마지막 렌더링된 미리보기 PNG
  - `GET /health` – 헬스 체크(인증 없이 접근 가능)
- 보안:
  - 기본은 loopback(127.0.0.1)에만 바인딩
  - 설정에서 Basic Auth 활성화 가능
  - Basic Auth 활성화 시 `/health` 를 제외한 모든 endpoint 보호

구현 파일:

- [`internal/web/web.go`](internal/web/web.go:1)

### 1.4 설정 & 런타임 캐시

- 설정 파일 (기본): `/etc/epdcal/config.yaml`
  - CLI 플래그 `--config` 로 경로 변경 가능
  - 최초 실행 시 파일이 없으면:
    - 기본 값으로 생성
    - 파일 권한을 `0600` 으로 설정
    - Web UI 접근 URL 을 로그/표준출력에 안내
- 런타임 캐시 디렉터리: `/var/lib/epdcal/`
  - ICS URL 별 HTTP 캐시 메타데이터(ETag, Last-Modified)
  - 마지막 렌더링 결과(`preview.png`, packed plane 등)

구현 파일:

- [`internal/config/config.go`](internal/config/config.go:1)

---

## 2. 아키텍처 및 코드 구조

### 2.1 전체 구조

프로젝트 루트 기준 디렉터리 구조(계획):

- [`cmd/epdcal/main.go`](cmd/epdcal/main.go:1) – CLI 엔트리포인트, 플래그 파싱, 메인 루프
- [`internal/config/config.go`](internal/config/config.go:1) – YAML 설정 로드/저장, 기본 값, 검증
- [`internal/web/web.go`](internal/web/web.go:1) – HTTP 서버, Web UI, JSON API, Basic Auth
- [`internal/ics/fetch.go`](internal/ics/fetch.go:1) – ICS 다운로드, HTTP 캐싱
- [`internal/ics/parse.go`](internal/ics/parse.go:1) – iCalendar 파서 래퍼, VEVENT/VTIMEZONE 해석
- [`internal/ics/expand.go`](internal/ics/expand.go:1) – RRULE/EXDATE/RECURRENCE-ID 확장
- [`internal/model/model.go`](internal/model/model.go:1) – 이벤트/occurrence/설정 스냅샷 도메인 모델
- [`internal/render/render.go`](internal/render/render.go:1) – `image.NRGBA` 로 캘린더 화면 렌더링
- [`internal/convert/pack.go`](internal/convert/pack.go:1) – NRGBA → 1bpp black/red plane 패킹
- [`internal/epd/epd_cgo.go`](internal/epd/epd_cgo.go:1) – Waveshare C 드라이버에 대한 cgo 바인딩
- [`internal/epd/epd.go`](internal/epd/epd.go:1) – 고수준 EPD 디스플레이 제어 API
- [`waveshare/`](waveshare/README.md:1) – Waveshare 12.48" B 패널용 C 드라이버 소스/헤더
- [`systemd/epdcal.service`](systemd/epdcal.service:1) – systemd 서비스 유닛
- [`progress.md`](progress.md:1) – 개발 진행/설계 메모
- [`README.md`](README.md:1) – 이 문서
- [`go.mod`](go.mod:1) – Go 모듈 및 Go 1.25.5 버전 선언
- (추가 예정) [`Makefile`](Makefile:1) – 빌드/테스트/배포 자동화

### 2.2 폴더 구조 정리 (권장 트리)

실제 생성될 디렉터리 트리를 개념적으로 정리하면 다음과 같습니다.

- [`cmd/`](cmd:1)
  - [`epdcal/`](cmd/epdcal:1)
    - [`main.go`](cmd/epdcal/main.go:1) – 엔트리포인트
- [`internal/`](internal:1)
  - [`config/`](internal/config:1)
    - [`config.go`](internal/config/config.go:1)
  - [`web/`](internal/web:1)
    - [`web.go`](internal/web/web.go:1)
  - [`ics/`](internal/ics:1)
    - [`fetch.go`](internal/ics/fetch.go:1)
    - [`parse.go`](internal/ics/parse.go:1)
    - [`expand.go`](internal/ics/expand.go:1)
    - [`parse_test.go`](internal/ics/parse_test.go:1)
    - [`expand_test.go`](internal/ics/expand_test.go:1)
    - [`testdata/`](internal/ics/testdata:1) – 테스트용 ICS fixture
  - [`model/`](internal/model:1)
    - [`model.go`](internal/model/model.go:1)
  - [`render/`](internal/render:1)
    - [`render.go`](internal/render/render.go:1)
  - [`convert/`](internal/convert:1)
    - [`pack.go`](internal/convert/pack.go:1)
  - [`epd/`](internal/epd:1)
    - [`epd.go`](internal/epd/epd.go:1)
    - [`epd_cgo.go`](internal/epd/epd_cgo.go:1)
- [`waveshare/`](waveshare:1)
  - (`EPD_12in48B.h`, C 소스 파일 등)
- [`systemd/`](systemd:1)
  - [`epdcal.service`](systemd/epdcal.service:1)
- 루트
  - [`README.md`](README.md:1)
  - [`progress.md`](progress.md:1)
  - [`go.mod`](go.mod:1)
  - [`Makefile`](Makefile:1) (예정)

이 구조를 기준으로, 실행 바이너리 `epdcal` 은 일반적으로 프로젝트 루트에서 빌드하며, 시스템 설치 시 `/usr/local/bin/epdcal` 로 복사합니다.

---

## 3. 설치 및 빌드

### 3.1 요구 사항

- Go 1.25.5 (또는 호환 버전) – [`go.mod`](go.mod:1)에 명시
- Raspberry Pi (Raspbian 계열)
- Waveshare 12.48" B e-paper 패널 + 해당 C 드라이버 라이브러리
- C 컴파일러 (예: `gcc`)

### 3.2 소스 코드 가져오기

```bash
git clone <this-repo-url> epdcal
cd epdcal
```

### 3.3 빌드 (Raspberry Pi 상에서)

```bash
go build -o epdcal ./cmd/epdcal
```

### 3.4 크로스 컴파일 (x86_64 → Raspberry Pi)

호스트가 macOS / Linux x86_64 일 때:

```bash
GOOS=linux GOARCH=arm GOARM=7 go build -o epdcal ./cmd/epdcal
```

또는 64bit OS(라즈비안 aarch64):

```bash
GOOS=linux GOARCH=arm64 go build -o epdcal ./cmd/epdcal
```

Waveshare C 드라이버와 cgo 연결 시 추가적인 `CGO_CFLAGS`, `CGO_LDFLAGS` 설정이 필요할 수 있습니다. 자세한 내용은 추후 [`waveshare/README.md`](waveshare/README.md:1) 에 정리합니다.

---

## 4. 설정 방법

### 4.1 기본 설정 파일 위치

- 기본 경로: `/etc/epdcal/config.yaml`
- CLI 로 다른 위치 지정:

```bash
./epdcal --config /path/to/config.yaml
```

초기 실행 시 해당 파일이 존재하지 않으면:

- 기본 값으로 파일 생성
- 권한 `0600` 적용
- 로그/표준출력에 Web UI 접속 URL 출력 (예: `http://127.0.0.1:8080/`)

### 4.2 설정 예시 (`config.yaml`)

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
  - url: "https://example.com/calendar1.ics"
    id: "work"
    name: "Work Calendar"
  - url: "https://example.com/calendar2.ics"
    id: "personal"
    name: "Personal"
basic_auth:
  username: "admin"
  password: "change-me"
```

필드 설명:

- `listen` – Web UI / API 가 바인딩할 주소 (`host:port`)
- `timezone` – 표시용 기본 타임존 (IANA 이름, 예: `Asia/Seoul`)
- `refresh_minutes` – 자동 갱신 주기 (분 단위)
- `horizon_days` – 앞으로 N일 치 일정 표시
- `show_all_day` – all-day 이벤트 섹션 표시 여부
- `highlight_red` – 해당 키워드가 제목/설명에 포함되면 빨간색으로 강조
- `ics` – 구독할 ICS URL 목록
  - `url` – ICS 구독 주소 (비공개 정보, 로그에 전체 URL은 기록하지 않음)
  - `id` / `name` – 내부/표시용 식별자
- `basic_auth` – Web UI Basic Auth (선택)
  - `username` / `password` – 인증 정보 (로컬 네트워크에서도 안전하게 관리 필요)

---

## 5. Web UI 사용법

1. 서비스/바이너리를 실행:

   ```bash
   ./epdcal
   ```

   또는 systemd 서비스로 실행 (아래 참고).

2. 브라우저에서 접속:

   ```
   http://127.0.0.1:8080/
   ```

3. 기능:

   - **Settings**
     - ICS URL 추가/삭제
     - Refresh interval, Timezone, Horizon days, All-day 섹션, Highlight 키워드 설정
   - **Actions**
     - "Refresh now" – 즉시 ICS fetch + render + display
     - "Render preview" – 패널에는 표시하지 않고 미리보기만 업데이트
   - **Status**
     - Last refresh time
     - Next scheduled refresh time
     - Last error (최근 에러 요약)
   - **Preview**
     - `/preview.png` 에서 마지막 렌더링 이미지를 확인 가능

Basic Auth 가 활성화되어 있다면, 브라우저에서 사용자명/비밀번호를 요구합니다. `/health` 엔드포인트는 인증 없이 `200 OK` 를 반환합니다.

---

## 6. ICS 타임존 / 반복 처리 상세

### 6.1 시간 정규화 전략

- 최종 표시는 항상 `config.Timezone` 기준 (예: `Asia/Seoul`)
- 각 이벤트의 시작/종료 시간 해석:

  1. `DTSTART;TZID=Zone/Name:...`  
     - ICS 의 `VTIMEZONE` 블록이 있는 경우 해당 정의를 우선 사용
     - 없을 경우 IANA 타임존 DB 로 해석 시도
  2. `DTSTART:...Z`  
     - UTC 로 해석
  3. `DTSTART:...` (floating time)  
     - 타임존 정보가 없으면 display timezone 기준 시간으로 처리
  4. `VALUE=DATE` (all-day)  
     - 로컬 날짜의 00:00 ~ 다음날 00:00(배타) 로 해석

- 모든 occurrence 는 display timezone 으로 변환 후, 날짜별로 그룹화하여 렌더링 파이프라인으로 전달

### 6.2 RRULE / EXDATE / RECURRENCE-ID

- 각 VEVENT 에 대해:

  - RRULE/RDATE 없음 → 단일 occurrence 생성
  - RRULE 존재 → 설정된 기간 내에서만 occurrence 생성
  - EXDATE → 해당 occurrence 제거
  - RECURRENCE-ID → 특정 occurrence를 override 이벤트로 교체

- 확장 범위:
  - 전역 범위: `[rangeStart, rangeEnd]`
  - 일반적으로:
    - `rangeStart = now - backfillDuration`
    - `rangeEnd = now + horizonDays`
  - 무한 루프 방지:
    - 이벤트 당 최대 occurrence 개수 제한 (예: 5000개)
    - 초과 시 로그에 경고 기록

사용 라이브러리:

- ICS 파서: Go ICS 파서 라이브러리 선택 및 [`internal/ics/parse.go`](internal/ics/parse.go:1) 에 래핑
- RRULE 확장:
  - `github.com/teambition/rrule-go` 등 사용 가능
  - 해당 라이브러리의 한계를 README "Known Limitations" 섹션에 명시

### 6.3 단위 테스트

- 테스트 ICS fixture 파일들: [`internal/ics/testdata/`](internal/ics/testdata/README.md:1) (계획)
  - 단순 단일 이벤트
  - 주간 반복 + EXDATE
  - RECURRENCE-ID 로 개별 occurrence override
  - TZID, UTC, all-day 이벤트가 섞인 케이스
- 테스트 코드:
  - [`internal/ics/parse_test.go`](internal/ics/parse_test.go:1)
  - [`internal/ics/expand_test.go`](internal/ics/expand_test.go:1)
- 검증 사항:
  - display timezone 으로 변환된 occurrence 의 시작/종료 시간이 기대값과 일치
  - EXDATE 적용 후 제거된 occurrence 가 존재하지 않음
  - override 된 occurrence 는 원본 대신 override 이벤트가 사용됨
  - 중복 occurrence 없음

---

## 7. 렌더링 파이프라인

### 7.1 이미지 렌더링

- 캔버스: `image.NRGBA` (1304x984)
- 폰트: `x/image/font/opentype` 로 로드한 폰트 사용
- 레이아웃(기본 MVP):
  - 상단: 현재 날짜 / 요일
  - 본문: 앞으로 N일간의 이벤트 목록 (날짜별 그룹)
  - All-day 이벤트는 별도 섹션 또는 상단에 강조
- 색상 규칙:
  - **Red plane 사용**:
    - Highlight 키워드가 포함된 이벤트
    - 주말(토/일) 및 향후 holiday 기능(선택적)

### 7.2 Packed Plane 변환

- [`internal/convert/pack.go`](internal/convert/pack.go:1) 에서 구현
- 변환 규칙:
  - 초기: `blackPlane = 0xFF`, `redPlane = 0xFF`
  - 픽셀 (x, y)에 대해:
    - `byteIndex = y*163 + (x >> 3)`
    - `mask = 0x80 >> (x & 7)`
  - 검은색 픽셀:
    - `blackPlane[byteIndex] &= ^mask`
  - 빨간색 픽셀:
    - `redPlane[byteIndex] &= ^mask`
  - C 드라이버는 `RedImage` 바이트를 전송 전에 `~` 연산하므로:
    - Go 측에서 0bit = "red ink" 이고, 실제 전송 시 다시 반전되어 패널로 출력

### 7.3 디버그 출력 (`--dump`)

- CLI 플래그 `--dump` 사용 시:
  - `black.bin` – Black plane raw buffer (160392 bytes)
  - `red.bin` – Red plane raw buffer (160392 bytes)
  - `preview.png` – 렌더링 결과 PNG
- 출력 위치는 기본적으로 현재 작업 디렉터리 또는 `/var/lib/epdcal/` 하위 디렉터리(구현 시 결정) 사용

---

## 8. EPD 표시 파이프라인

- cgo 로 호출되는 C 함수:

  ```c
  UBYTE EPD_12in48B_Init(void);
  void EPD_12in48B_Clear(void);
  void EPD_12in48B_Display(const UBYTE *BlackImage, const UBYTE *RedImage);
  void EPD_12in48B_TurnOnDisplay(void);
  void EPD_12in48B_Sleep(void);
  ```

- Go 측 래퍼 (예시):

  ```go
  func Init() error
  func Clear() error
  func Display(black, red []byte) error
  func Sleep() error
  ```

- 동작 흐름:

  1. `EPD_12in48B_Init()` 호출 (한 번)
  2. 필요 시 `EPD_12in48B_Clear()`
  3. 최신 렌더링 결과에서 black/red plane 생성
  4. `EPD_12in48B_Display(black, red)` 호출
  5. 장시간 업데이트가 없을 경우 `EPD_12in48B_Sleep()`

- `--render-only` 모드:
  - 위 C 함수는 호출하지 않고, 렌더링/pack/dump 만 수행
  - 개발/디버깅용

---

## 9. systemd 서비스

예시 유닛 파일: [`systemd/epdcal.service`](systemd/epdcal.service:1)

```ini
[Unit]
Description=EPD ICS Calendar Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=epdcal
Group=epdcal
ExecStart=/usr/local/bin/epdcal --config=/etc/epdcal/config.yaml
Restart=on-failure
RestartSec=10
Environment=TZ=Asia/Seoul

[Install]
WantedBy=multi-user.target
```

설치 절차(예시):

```bash
sudo useradd -r -s /usr/sbin/nologin epdcal
sudo mkdir -p /etc/epdcal /var/lib/epdcal
sudo chown -R epdcal:epdcal /etc/epdcal /var/lib/epdcal

sudo cp epdcal /usr/local/bin/
sudo cp systemd/epdcal.service /etc/systemd/system/

sudo systemctl daemon-reload
sudo systemctl enable epdcal
sudo systemctl start epdcal
```

---

## 10. CLI 옵션

엔트리포인트: [`cmd/epdcal/main.go`](cmd/epdcal/main.go:1)

지원 플래그(계획):

- `--config /path/to/config.yaml`  
  설정 파일 경로 (기본 `/etc/epdcal/config.yaml`)

- `--listen 127.0.0.1:8080`  
  Web UI / API 리슨 주소 (설정 파일보다 CLI 가 우선)

- `--once`  
  한 번 `fetch + render + display` 수행 후 종료  
  (systemd 타이머 혹은 크론에서 사용할 수 있음)

- `--render-only`  
  EPD 하드웨어 호출 없이 렌더링/pack/dump 만 수행

- `--dump`  
  `black.bin`, `red.bin`, `preview.png` 디버그 파일 출력

---

## 11. Known Limitations (예정/기록용)

실제 구현 후 다음과 같은 제한 사항을 README 에 계속 업데이트합니다.

- 일부 복잡한 RRULE 조합 (예: BYSETPOS, 복수 RRULE, 고급 BYxxx 조합) 은 완전히 지원되지 않을 수 있음
- ICS 에 포함된 커스텀 time zone (비표준 VTIMEZONE) 정의에 대해서는 Go 타임존 DB 와 정확히 일치하지 않을 수 있음
- 매우 많은 이벤트/반복을 가진 ICS 의 경우, 성능 및 메모리 사용을 위해 occurrence 개수를 제한할 수 있음
- Web UI 는 단일 사용자 시나리오에 최적화되어 있으며, 고급 권한/역할 관리 기능은 제공하지 않음

---

## 12. Troubleshooting

1. **Web UI 접속 불가**
   - 프로세스가 실행 중인지 확인:

     ```bash
     systemctl status epdcal
     ```

   - `listen` 설정이 `127.0.0.1` 인지, 포트 충돌이 없는지 확인
   - 원격에서 접속하려면 `listen` 을 `0.0.0.0:8080` 등으로 변경하고 방화벽 규칙 확인

2. **캘린더가 오래된 상태에서 갱신되지 않음**
   - 로그에서 fetch 에러/ICS 파싱 에러가 있는지 확인
   - ICS 서버가 `ETag` / `Last-Modified` 를 잘못 반환하는 경우, 수동으로 캐시 파일 삭제 후 재시도 필요

3. **시간/타임존이 이상하게 보임**
   - `config.yaml` 의 `timezone` 이 올바른 IANA 이름인지 확인
   - ICS 파일의 `VTIMEZONE` 정의가 표준에 맞는지 확인
   - `--dump` 모드로 occurrence 타임스탬프를 로깅하여 디버그

4. **EPD 가 갱신되지 않거나 화면이 깨져 보임**
   - `--render-only` 모드에서 `preview.png` 가 정상인지 먼저 확인
   - `black.bin` / `red.bin` 크기가 160392 bytes 인지 확인
   - Waveshare C 드라이버 및 배선이 올바르게 연결되었는지 확인

---

## 13. 개발 진행 상황

개발 진행 및 세부 TODO 는 [`progress.md`](progress.md:1) 에 정리되어 있습니다.  
이 문서는 전체 기능/요구사항을 개괄적으로 설명하는 README 이며, 세부 구현 계획은 progress 문서를 참고하십시오.

---

## 14. 폴더 구조 및 실행 파일 배치 정리

### 14.1 개발 환경에서의 구조

- 소스 코드는 모두 Git 리포지토리 루트 하위에 위치합니다.
  - 애플리케이션 엔트리포인트: [`cmd/epdcal/main.go`](cmd/epdcal/main.go:1)
  - 라이브러리/도메인 로직: [`internal/`](internal:1) 이하
  - 외부 C 드라이버: [`waveshare/`](waveshare:1)
  - 배포 스크립트/서비스 정의: [`systemd/`](systemd:1)
  - 문서: [`README.md`](README.md:1), [`progress.md`](progress.md:1)
  - 모듈 정의: [`go.mod`](go.mod:1)

- 개발 중 빌드 결과물(실행 파일 `epdcal`)은 일반적으로 리포지토리 루트에 생성합니다:

  ```bash
  go build -o epdcal ./cmd/epdcal
  ```

  이 실행 파일을 직접 실행하거나, systemd 서비스 설치 시 `/usr/local/bin/epdcal` 로 복사합니다.

### 14.2 운영 환경에서의 배치

- 바이너리: `/usr/local/bin/epdcal`
- 설정: `/etc/epdcal/config.yaml`
- 런타임 데이터/캐시: `/var/lib/epdcal/`
  - `ics-cache/` (ETag/Last-Modified, ICS 본문)
  - `preview.png`
  - `black.bin`, `red.bin` (선택, `--dump` 사용 시)
- 로그:
  - systemd 서비스로 실행 시 journald 로 출력
  - 필요 시 `/var/log/syslog` 나 `journalctl -u epdcal` 로 조회

폴더와 파일 위치는 [`internal/config/config.go`](internal/config/config.go:1) 및 [`cmd/epdcal/main.go`](cmd/epdcal/main.go:1) 의 기본 상수/플래그로 정의할 예정입니다.

---

## 15. Makefile 개요 (빌드/테스트/배포 자동화)

프로젝트 루트에 [`Makefile`](Makefile:1) 을 두고, 반복적인 빌드/테스트/배포 작업을 단순화합니다. (아래는 계획/설계 내용이며, 실제 Makefile 구현 시 이 구조를 따릅니다.)

### 15.1 주요 타겟 설계

- `make build`  
  - 현재 호스트 환경(개발 머신)용 `epdcal` 빌드
  - 내부 명령: `go build -o epdcal ./cmd/epdcal`

- `make build-pi`  
  - Raspberry Pi (32bit ARM) 용 바이너리 빌드
  - 내부 명령:
    - `GOOS=linux GOARCH=arm GOARM=7 go build -o epdcal ./cmd/epdcal`

- `make build-pi64`  
  - Raspberry Pi 64bit(aarch64) 용 바이너리 빌드
  - 내부 명령:
    - `GOOS=linux GOARCH=arm64 go build -o epdcal ./cmd/epdcal`

- `make test`  
  - 전체 Go 테스트 실행
  - 내부 명령:
    - `go test ./...`

- `make lint` (선택)  
  - go vet 또는 golangci-lint 등을 이용한 정적 분석
  - 초기 버전에서는 `go vet ./...` 정도로 시작 가능

- `make install`  
  - 빌드 후 `/usr/local/bin/epdcal` 로 복사
  - `/etc/epdcal` 및 `/var/lib/epdcal` 디렉터리 생성/권한 설정
  - `systemd/epdcal.service` 를 `/etc/systemd/system/` 에 설치

- `make run`  
  - 개발용 실행 (`--render-only --dump` 와 함께 실행하는 식으로 구성 가능)

- `make clean`  
  - 빌드 산출물(`epdcal`, `black.bin`, `red.bin`, `preview.png` 등) 제거

### 15.2 예시 Makefile 스니펫 (설계용)

> 실제 [`Makefile`](Makefile:1) 에는 아래와 유사한 내용이 들어갈 예정입니다.

```make
BINARY := epdcal
PKG := ./cmd/epdcal

build:
	go build -o $(BINARY) $(PKG)

build-pi:
	GOOS=linux GOARCH=arm GOARM=7 go build -o $(BINARY) $(PKG)

build-pi64:
	GOOS=linux GOARCH=arm64 go build -o $(BINARY) $(PKG)

test:
	go test ./...

run:
	./$(BINARY) --render-only --dump

clean:
	rm -f $(BINARY) black.bin red.bin preview.png
```

운영 환경 설치/서비스 등록용 타겟(`install`, `systemd-install` 등)은 프로젝트 요구에 맞춰 추가합니다.

---

## 16. 로깅 전략

### 16.1 기본 로깅 방향

- 모든 서비스 로직은 **표준 출력/표준 에러** 로 로그를 남기고, systemd 환경에서는 journald 가 이를 수집하도록 합니다.
- 로그 라이브러리는 Go 1.25.5 기준:
  - 초기에는 표준 라이브러리 `log` 패키지를 사용
  - 필요 시 간단한 래퍼를 두어 로그 레벨, 태그, 구조화 필드를 지원

예상 로깅 인터페이스(설계):

```go
// internal/log/log.go (예정)
func Info(msg string, kv ...any)
func Error(msg string, err error, kv ...any)
func Debug(msg string, kv ...any)
```

각 함수는 `[timestamp] [level] [component] message key=value ...` 형태로 출력합니다.

### 16.2 민감 정보 취급

- ICS URL 은 **절대 전체를 로그에 남기지 않음**
  - 일부 host 정보나 해시를 사용해 식별만 가능하게 함
  - 예: `https://private.example.com/cal.ics` → `ics://private.example.com/... (redacted)`
- 이벤트 제목/내용 등도 디버그 모드가 아닌 이상 상세히 출력하지 않도록 주의
  - 에러 상황에서는 UID, 시작시각 등 비식별 정보 중심으로 로깅

### 16.3 컴포넌트 별 로깅 예시

- ICS Fetch:
  - level=INFO: fetch 시작/성공/304 not modified
  - level=ERROR: HTTP 에러, 타임아웃, 응답 파싱 실패
- ICS Parse/Expand:
  - level=ERROR: ICS 문법 에러, 지원하지 않는 RRULE, VTIMEZONE 해석 실패
  - level=DEBUG: 특정 이벤트/occurrence 의 timestamp 디버그 정보 (`--dump` 또는 디버그 모드에서만)
- Render:
  - level=INFO: 렌더 시작/완료, 소요 시간
  - level=ERROR: 폰트 로드 실패, 이미지 변환 실패 등
- EPD:
  - level=INFO: Init/Clear/Display/Sleep 호출
  - level=ERROR: C 드라이버에서 에러 리턴, 하드웨어 통신 문제

### 16.4 systemd/journald 와의 연동

- 서비스가 journald 를 사용하는 환경(Raspbian systemd)에서는:
  - `ExecStart` 에 특별한 redirection 없이 `epdcal` 실행
  - `journalctl -u epdcal` 을 통해 로그 확인
  - 필요 시 `SyslogIdentifier=epdcal` 등 추가 설정 가능

---

## 17. 요약

- 폴더 구조는 `cmd/`, `internal/`, `waveshare/`, `systemd/` 를 중심으로 구성하며, 실행 바이너리 `epdcal` 은 루트에서 빌드 후 `/usr/local/bin/epdcal` 로 배포합니다.
- [`Makefile`](Makefile:1) 을 통해 빌드/테스트/설치/정리를 자동화할 계획입니다.
- 로깅은 표준 출력/에러 + journald 조합을 기본으로 하며, 민감 정보(ICS URL 등)는 반드시 마스킹하여 기록합니다.

이 문서를 기준으로 실제 코드, Makefile, 로깅 유틸리티를 단계적으로 구현해 나가면 됩니다.