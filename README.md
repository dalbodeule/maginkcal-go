# epdcal – ICS 기반 E‑Paper 캘린더 (Raspberry Pi / Waveshare 12.48" B Panel)

`epdcal` 은 Raspberry Pi (Raspbian/ARM) 에서 동작하는 단일 Go 애플리케이션으로,  
Waveshare 12.48" tri‑color e‑paper (B) 패널(1304x984)에 **ICS(iCalendar) 구독 캘린더**를 표시한다.

- 여러 개의 ICS URL 구독
- 타임존(TZID/VTIMEZONE), 반복(RRULE), 예외(EXDATE), override(RECURRENCE-ID), all‑day 이벤트 처리
- 로컬 Web UI 로 설정/상태 확인 및 수동 Refresh/Render
- cgo 를 통해 Waveshare C 드라이버(`EPD_12in48B.h`) 호출
- Google API / OAuth / token.pickle / Python / PIL 등은 **전혀 사용하지 않음**

이 문서는 설치/구동 방법, 설정 방법, ICS Recurrence/TZ 처리 전략, 한계점, 문제 해결 방법을 설명한다.  
자세한 설계 및 진행 상황은 `progress.md` 를 참고한다.

---

## 1. 기능 개요

### 1.1 주요 기능

- **ICS 구독**
  - 하나 이상의 ICS(iCalendar) URL 을 주기적으로 fetch
  - HTTP ETag / Last‑Modified 기반 캐싱 (If‑None‑Match / If‑Modified‑Since)
  - 네트워크 에러 또는 304 시 로컬 캐시 fallback

- **iCalendar 처리**
  - TZID/VTIMEZONE 블록 파싱
  - `DTSTART;TZID=...` / `DTEND;TZID=...` / UTC (`Z`) 시각 / floating time 처리
  - RRULE(`FREQ=DAILY/WEEKLY/MONTHLY/YEARLY`, `BYDAY`, `BYMONTHDAY`, `INTERVAL`, `COUNT`, `UNTIL`) 확장
  - `EXDATE` 로 occurrence 제거
  - `RECURRENCE-ID` VEVENT 로 단일 인스턴스 override
  - DATE 타입 all‑day 이벤트 처리

- **표시/렌더링**
  - `image.NRGBA` 로 캘린더 화면 렌더링 (텍스트/레이아웃)
  - red plane: 키워드 매칭 이벤트 또는 주말/공휴일 강조 등
  - 최종 이미지를 1bpp packed buffer (black/red plane) 로 변환 후 EPD 에 전송
  - `--dump` 옵션으로 `preview.png`, `black.bin`, `red.bin` 출력

- **Web UI**
  - Web UI 를 통해:
    - ICS URL 목록 관리
    - refresh 주기(분 단위 또는 cron 패턴), timezone, 표시 옵션 설정
    - “Refresh now” (fetch+render+display) 버튼
    - “Render preview” (fetch+render only) 버튼
    - 마지막/다음 스케줄, 마지막 오류 표시
  - `/preview.png` 로 마지막 렌더링 이미지를 브라우저에서 확인

- **디스플레이 드라이버**
  - Waveshare 제공 C 드라이버(`EPD_12in48B.h`) 를 cgo 로 래핑
  - `EPD_12in48B_Init/Display/Clear/Sleep` 호출

---

## 2. 하드웨어 및 패널 사양

- **패널**: Waveshare 12.48" tri‑color e‑paper (B)
- **해상도**: 1304 x 984
- **버퍼 형식**:
  - 1bpp, MSB‑first
  - stride: 163 bytes per row (1304 / 8)
  - plane buffer size: 163 × 984 = 160,392 bytes
  - 픽셀 (x, y)의 비트 위치:
    - `byteIndex = y*163 + (x >> 3)`
    - `mask = 0x80 >> (x & 7)`
  - `0` = 잉크(black 또는 red), `1` = white
  - C 드라이버는 red plane 바이트를 전송 전 `~` 로 반전:
    - Go 쪽에서는 `0 = red ink` semantics 로 채운 뒤 그대로 전달

EPD C API:

```c
UBYTE EPD_12in48B_Init(void);
void EPD_12in48B_Clear(void);
void EPD_12in48B_Display(const UBYTE *BlackImage, const UBYTE *RedImage);
void EPD_12in48B_TurnOnDisplay(void);
void EPD_12in48B_Sleep(void);
```

Go 에서는 cgo 를 이용해 위 함수들을 thin wrapper 로 감싸 `internal/epd` 패키지에서 사용한다.

---

## 3. 요구되는 소프트웨어 / 의존성

- OS: Raspberry Pi OS (Raspbian) / Linux ARM
- Go: 1.21 이상 권장
- C Toolchain:
  - `gcc`, `make`, etc.
- Waveshare 12.48" (B) C 드라이버:
  - 레포지토리 내 `waveshare/` 디렉터리에 vendored
- (선택) Headless 브라우저 사용 시:
  - Chromium + chromedp (추가 기능으로 사용할 경우)

빌드/런 시 Google API, Python, PIL, token.pickle 등은 필요하지 않다.

---

## 4. 빌드 및 설치

### 4.1 레포지토리 구조 (요약)

```text
cmd/epdcal/main.go      # 메인 엔트리 포인트
internal/config/        # 설정 로딩/검증
internal/web/           # HTTP/Web UI 서버
internal/ics/           # ICS fetch/parse/expand
internal/model/         # 공용 모델 (Occurrence 등)
internal/render/        # image.NRGBA 렌더링
internal/convert/       # NRGBA → packed plane 변환
internal/epd/           # cgo 기반 EPD 드라이버 래퍼
waveshare/              # vendored Waveshare C 드라이버
systemd/epdcal.service  # systemd 유닛 파일
progress.md             # 진행/설계 문서
README.md               # 이 문서
```

### 4.2 빌드

Raspberry Pi 상에서:

```bash
cd /path/to/maginkcal-go
go build -o epdcal ./cmd/epdcal
```

빌드 결과:

- `./epdcal` 실행 파일 생성

### 4.3 설치 (예시)

```bash
# 바이너리 설치
sudo install -m 0755 ./epdcal /usr/local/bin/epdcal

# 설정 디렉터리/파일
sudo mkdir -p /etc/epdcal
sudo touch /etc/epdcal/config.yaml
sudo chmod 600 /etc/epdcal/config.yaml

# 런타임 데이터 디렉터리
sudo mkdir -p /var/lib/epdcal
sudo chown pi:pi /var/lib/epdcal  # 필요 시 사용자에 맞게 조정
```

최초 실행 시 config 가 비어 있다면 기본값을 채우는 로직을 둘 수도 있으며,  
그렇지 않다면 README 에 나온 예시를 참고해 수동으로 작성한다.

---

## 5. 설정 파일 (`/etc/epdcal/config.yaml`)

### 5.1 예시

```yaml
listen: "127.0.0.1:8080"
timezone: "Asia/Seoul"
refresh: "*/15 * * * *"     # 15분마다
horizon_days: 7
show_all_day: true
highlight_red_keywords:
  - "중요"
  - "휴가"
  - "deadline"

ics:
  - id: "personal"
    url: "https://example.com/personal.ics"
  - id: "work"
    url: "https://example.com/work.ics"

basic_auth:
  enabled: true
  username: "admin"
  password: "change-me"
```

주요 필드:

- `listen`: HTTP 서버 bind 주소 (`127.0.0.1:8080` 권장)
- `timezone`: 표시용 타임존 (IANA 이름, 예: `Asia/Seoul`)
- `refresh`:
  - cron 스타일 문자열 (예: `*/15 * * * *`)
  - 지정한 스케줄에 맞춰 `fetch + render + display` 수행
- `horizon_days`:
  - 앞으로 몇 일치의 이벤트를 표시할지 (예: 7일)
- `show_all_day`: all‑day 섹션 표시 여부
- `highlight_red_keywords`:
  - 이벤트 제목/설명에 포함될 경우 red plane 으로 강조할 키워드 목록
- `ics`:
  - `id`: 내부 식별자
  - `url`: ICS 구독 URL (비공개 URL 포함 가능, **로그에 풀로 찍지 않도록 주의**)
- `basic_auth`:
  - `enabled`: true 시 Basic Auth 활성화
  - `username`, `password`: 인증 정보

설정 파일 퍼미션은 **0600** 으로 유지하여 URL/비밀번호가 노출되지 않도록 한다.

---

## 6. Web UI 및 HTTP API

### 6.1 엔드포인트 요약

- `GET /`  
  메인 HTML UI (설정/상태/액션 버튼 제공)

- `GET /api/config`  
  현재 설정 값을 JSON 형태로 반환

- `POST /api/config`  
  JSON body 를 받아 설정 값을 갱신.  
  (예: ICS URL 추가/삭제, refresh 스케줄 변경, timezone 변경 등)

- `POST /api/refresh`  
  즉시 `fetch + render + display` 실행.  
  (주기 스케줄과 별개로 수동 갱신 용도)

- `POST /api/render`  
  `fetch + render` 까지만 수행, EPD 디스플레이는 건드리지 않음.  
  Preview PNG 업데이트 용도.

- `GET /preview.png`  
  마지막 렌더링 결과 PNG 반환.  
  브라우저에서 EPD 에 전송될 화면을 미리 확인할 수 있다.

- `GET /health`  
  헬스 체크용 간단한 OK 응답.  
  Basic Auth 없이도 접근 가능하도록 유지.

### 6.2 보안

- 기본적으로 `listen: "127.0.0.1:8080"` 으로 설정하여 로컬에서만 접속 가능하게 한다.
- 다른 호스트/IP 에서 접근이 필요하다면:
  - `listen: "0.0.0.0:8080"` 처럼 변경
  - **반드시 Basic Auth 또는 방화벽, VPN 등의 추가 보호를 사용할 것**
- Basic Auth 가 활성화된 경우:
  - `/health` 를 제외한 모든 엔드포인트에서 인증 필요

---

## 7. ICS Recurrence/TZ 처리 개요

### 7.1 시간 정규화 전략

- 모든 occurrence 는 최종적으로 `config.Timezone` (예: `Asia/Seoul`) 기준 시각으로 변환 후 사용
- 파싱 규칙:
  - `DTSTART;TZID=Zone/...`:
    - ICS 내 `VTIMEZONE` 정의 또는 시스템 타임존 DB를 사용해 해석
  - `DTSTART:...Z` (UTC):
    - UTC 로 파싱 후 표시용 타임존으로 변환
  - floating time (TZID, `Z` 없음):
    - 캘린더/이벤트의 기본 타임존 규칙 또는 표시용 타임존으로 해석
  - `DATE` 타입(all‑day):
    - 표시용 타임존 기준:
      - 시작: `YYYY-MM-DD 00:00`
      - 종료: `다음 날 00:00` (exclusive)

### 7.2 Recurrence 확장

- RRULE 이 없는 VEVENT:
  - 단일 occurrence 만 생성
- RRULE 이 있는 VEVENT:
  - `FREQ`, `BYDAY`, `BYMONTHDAY`, `INTERVAL`, `COUNT`, `UNTIL` 등을 지원
  - `[rangeStart, rangeEnd]` (예: `now - backfill`, `now + horizon`) 범위 안에서만 occurrence 생성
  - 이벤트 당 일정 개수(예: 5000개) 상한을 두어 폭발 방지

- 예외/override:
  - `EXDATE`:
    - RRULE 로 생성된 occurrence 중 해당 날짜/시간과 일치하는 인스턴스를 제거
  - `RECURRENCE-ID`:
    - `(UID, RECURRENCE-ID timestamp)` 키로 base occurrence 탐색
    - 해당 occurrence 의 내용(시간/제목/위치 등)을 override VEVENT 로 대체

- UID / 중복 제거:
  - 여러 ICS 를 merge 할 때:
    - `(calendarID(or URL), UID, recurrence-instance key)` 로 occurrence 를 식별
    - 동일 키는 하나만 남기되, 나중 규칙/override 에 의해 갱신할 수 있음

### 7.3 라이브러리

- ICS 파싱:
  - 예: `github.com/arran4/golang-ical`
- RRULE 처리:
  - 예: `github.com/teambition/rrule-go`
- 구현 상 제한/예외 케이스는 아래 *한계 및 제한 사항* 에 명시

---

## 8. Known Limitations (알려진 제한 사항)

아래 항목은 구현/테스트 범위를 벗어나거나, 단순화한 부분이다.

- 매우 복잡한 RRULE 조합:
  - 예: BYSETPOS, 복수의 RRULE, RDATE/RRULE 혼합 등
  - 일반적인 데일리/위클리/먼슬리/이어리 + BYDAY/BYMONTHDAY/INTERVAL/COUNT/UNTIL 중심으로 동작 검증
- 일부 희귀 타임존 규칙:
  - ICS 내 VTIMEZONE 정의가 불완전하거나, 시스템 타임존 DB 와 상이한 경우
  - 이 경우 표시 시간에 약간의 오차가 생길 수 있음
- ICS 표준을 엄격히 따르지 않는 구현체:
  - 일부 서버는 비표준 확장 필드를 포함하거나, DATE/DATE-TIME/TZID 처리에 일관성이 부족할 수 있다.
- EPD 하드웨어 제약:
  - 업데이트 속도가 느리므로 너무 짧은 interval 로 빈번하게 업데이트하는 것은 권장하지 않는다.
  - 부분 업데이트(partial refresh)는 지원하지 않으며, 항상 full refresh 기준으로 구현

이들 제한 사항은 `progress.md` 와 코드 주석에도 가능한 한 명시하며,  
필요 시 향후 릴리스에서 보완할 수 있다.

---

## 9. 실행 방법

### 9.1 단발 실행 (테스트용)

```bash
epdcal --config /etc/epdcal/config.yaml --once --dump
```

- 지정된 ICS 를 fetch/parse/expand 한 뒤 한 번 렌더링 및 EPD 표시를 수행하고 종료
- `--dump` 사용 시:
  - `preview.png`
  - `black.bin`
  - `red.bin`
  등을 `/var/lib/epdcal/` (또는 설정된 디렉터리)에 저장

### 9.2 데몬 실행

```bash
epdcal --config /etc/epdcal/config.yaml
```

- 설정 파일의 `refresh` 스케줄에 맞춰 주기적으로 업데이트
- HTTP Web UI (`listen` 주소 기준) 가 활성화됨

### 9.3 Web UI 접속

- 예: `listen: "127.0.0.1:8080"` 인 경우,
  - Raspberry Pi 에서 브라우저를 열어 `http://127.0.0.1:8080/` 접속
  - 같은 네트워크의 PC 에서 접속하고 싶다면 `listen` 을 `0.0.0.0:8080` 으로 변경 후:
    - `http://<라즈베리파이 IP>:8080/` 으로 접속
- Basic Auth 활성화 시 브라우저에서 사용자명/비밀번호를 입력해야 한다.

---

## 10. systemd 서비스

예시 `systemd/epdcal.service`:

```ini
[Unit]
Description=EPD ICS Calendar
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/epdcal --config /etc/epdcal/config.yaml
Restart=on-failure
User=pi
Group=pi

[Install]
WantedBy=multi-user.target
```

설치:

```bash
sudo cp systemd/epdcal.service /etc/systemd/system/epdcal.service
sudo systemctl daemon-reload
sudo systemctl enable epdcal
sudo systemctl start epdcal
```

상태 확인:

```bash
systemctl status epdcal
journalctl -u epdcal -f
```

---

## 11. Troubleshooting (문제 해결)

### 11.1 화면이 업데이트되지 않음

- `journalctl -u epdcal -f` 로 로그 확인
- ICS fetch 에러:
  - 네트워크/URL 을 확인
  - HTTPS 인증서 문제 여부 확인
- Web UI 에서:
  - 마지막 오류 메시지(last error)를 확인

### 11.2 시간/타임존이 이상하게 보임

- `config.yaml` 의 `timezone` 이 올바른 IANA 이름인지 확인 (예: `Asia/Seoul`)
- ICS 파일 내 이벤트의 DTSTART/DTEND 가 어떤 형태인지(UTC / TZID / DATE) 확인
- DST 가 있는 타임존인 경우, Recurrence 경계(특히 DST 전후)에서 약간의 오차가 있을 수 있음

### 11.3 EPD 가 반응하지 않음

- SPI/I2C 등 하드웨어 연결 확인 (제공된 Waveshare C 예제 코드로 먼저 테스트해 보는 것을 권장)
- `EPD_12in48B_Init` 가 0 이 아닌 값을 반환하는지 로그에서 확인
- 충분한 전류/전압 공급 여부 확인 (대형 EPD 는 비교적 많은 전력을 소모)

### 11.4 ICS 이벤트 일부가 빠지거나 중복됨

- 해당 이벤트가:
  - EXDATE 에 의해 제거된 것은 아닌지
  - RECURRENCE-ID override 로 치환된 것은 아닌지
- 다수의 ICS 를 merge 할 경우:
  - 같은 UID/INSTANCE 키를 가진 이벤트가 여러 ICS 에 정의되어 중복 제거되었을 가능성
- 복잡한 RRULE 조합인 경우:
  - 현재 구현이 일부 패턴을 지원하지 않을 수 있음 (Known Limitations 섹션 참고)

---

## 12. License

라이선스 정보는 `LICENSE.md` 를 참고한다.

---

## 13. 개발 참고

- 상세 설계, 진행 상황, 향후 TODO 는 `progress.md` 에 정리되어 있다.
- ICS Recurrence/TZ 처리는 정확성을 최우선으로 하며,
  - `internal/ics/testdata/*.ics` fixture 와
  - `internal/ics` 의 unit test 로 지속적으로 검증할 계획이다.