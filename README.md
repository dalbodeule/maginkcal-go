# epdcal – ICS 기반 E‑Paper 캘린더 (Raspberry Pi / Waveshare 12.48" B Panel)

`epdcal` 은 Raspberry Pi (Raspbian/ARM) 에서 동작하는 단일 Go 애플리케이션으로,  
Waveshare 12.48" tri‑color e‑paper (B) 패널(1304x984)에 **ICS(iCalendar) 구독 캘린더**를 표시한다.

- 현재 버전: `0.1.1`

- 여러 개의 ICS URL 구독
- 타임존(TZID/VTIMEZONE), 반복(RRULE), 예외(EXDATE), override(RECURRENCE-ID), all‑day 이벤트 처리
- 로컬 Web UI 로 설정 편집, 캘린더 및 최신 Preview 확인
- cgo 를 통해 Waveshare C 드라이버(`EPD_12in48B.h`) 호출
- 배터리 상태 조회 시 I2C(`/dev/i2c-1`) 접근
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
    - ICS URL 목록, cron 주기, timezone, 표시 옵션 편집
    - 설정을 YAML과 실행 중 메모리에 저장해 다음 요청/갱신부터 적용
    - 저장된 ICS URL의 접근 및 파싱 상태 확인
    - 제목 접두어 기반 휴일 규칙 편집
    - 캘린더와 마지막 생성된 Preview 확인
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
- Go: 1.27 이상 필요
- C Toolchain:
  - `gcc`, `make`, etc.
- Waveshare 12.48" (B) C 드라이버 및 GPIO 라이브러리:
  - 레포지토리 내 `internal/epd/c` 디렉터리에 포함된 C 소스를 정적 라이브러리로 빌드 (`make -C internal/epd/c`)
  - Debian 계열에서는 `liblgpio-dev` 패키지 필요
- 한글 렌더링 글꼴:
  - 시스템 한글 글꼴을 자동 탐색하며, 찾지 못한 경우 `fonts-nanum` 설치를 권장
- Chromium/headless browser는 필요하지 않다. Go 내부 렌더러가 PNG를 생성한다.

```bash
sudo apt update
sudo apt install build-essential liblgpio-dev
# 시스템에 사용 가능한 한글 글꼴이 없다면 설치
sudo apt install fonts-nanum
```

빌드/런 시 Google API, Python, PIL, token.pickle 등은 필요하지 않다.

---

## 4. 빌드 및 설치

### 4.1 레포지토리 구조 (요약)

```text
cmd/epdcal/main.go      # 메인 엔트리 포인트
internal/config/        # 설정 로딩/검증
internal/web/           # HTTP/Web UI 서버 + 정적 파일 서빙
internal/ics/           # ICS fetch/parse/expand
internal/model/         # 공용 모델 (Occurrence 등)
internal/render/        # Go 기반 캘린더 PNG 렌더러
internal/convert/       # PNG(image.NRGBA) → packed plane 변환
internal/epd/           # cgo 기반 EPD 드라이버 래퍼 및 C 소스(internal/epd/c)
webui/                  # Next.js Web UI 소스
systemd/epdcal.service  # systemd 유닛 파일
progress.md             # 진행/설계 문서
README.md               # 이 문서
```

### 4.2 빌드

#### 4.2.1 C 드라이버 정적 라이브러리 빌드

`internal/epd/epd_cgo.go` 는 `internal/epd/c` 에서 빌드한 정적 라이브러리(`libepddrv.a`)를 링크한다.
최초 한 번, 또는 C 소스를 변경한 뒤에는 다음을 실행한다:

```bash
cd /path/to/maginkcal-go/internal/epd/c
make
cd -    # 원래 디렉터리로 복귀
```

#### 4.2.2 Raspberry Pi 상에서 Go 바이너리 빌드

Web UI 정적 파일(`internal/web/static`)은 Go 바이너리에 포함된다. 개발 머신에서
`cd webui && npm ci && cd .. && make webui-build`로 최신 UI를 만든 다음 Pi에
`webui.zip`을 전달한다. Pi에서는 C 드라이버 빌드 후 `make build-pi-cgo`로
32비트 ARM용 하드웨어 드라이버 포함 바이너리를 만든다.

```bash
cd /path/to/maginkcal-go
make -C internal/epd/c libepddrv.a
make build-pi-cgo
```

렌더러는 먼저 `EPDCAL_FONT_REGULAR`/`EPDCAL_FONT_BOLD` 환경 변수를 확인하고,
지정 경로가 없거나 폰트에 필요한 한글 글리프가 없으면 `fontscan`으로 시스템
폰트를 검색한다. Nanum Gothic, Noto CJK, Apple SD Gothic, Malgun Gothic 등을
우선 검색한다. 사용할 수 있는 한글 폰트가 없으면 시작 단계에서 오류를 기록하고
프로세스가 종료되므로 `journalctl -u epdcal`에서 원인을 확인할 수 있다.
환경 변수는 `/etc/default/epdcal`에 절대 경로로 지정할 수 있다. 시스템 폰트 인덱스는
서비스의 캐시 디렉터리에 저장되어 이후 시작 시 재사용된다.

바이너리를 systemd 밖에서 직접 실행하면 바이너리가 `epdcal.env`를 자동으로 읽지는
않는다. 신뢰할 수 있는 env 파일을 현재 셸에 export한 뒤 실행한다.

```bash
set -a
. /etc/default/epdcal
set +a
./epdcal --config /etc/epdcal/config.yaml
```

다른 경로를 쓰면 `. /절대/경로/epdcal.env`로 지정한다. 이 파일을 셸 구문으로
불러오므로 본인이 관리하는 신뢰된 파일만 사용한다. systemd 서비스에서는 unit의
`EnvironmentFile=`이 이 경로를 관리한다.

특정 패밀리를 선택하려면 `config.yaml`의 `font_family`에 `나눔고딕` 또는
`Noto Sans KR`을 지정한다. 요청한 패밀리를 사용할 수 없으면 시작 시 오류로 알린다.
`EPDCAL_FONT_REGULAR` 환경 변수가 지정되어 있으면 해당 파일 경로가 `font_family`보다
우선한다. `layout_json`이 비어 있거나 공백뿐이면 기본 레이아웃 JSON으로 채워진다.

빌드 결과:

- `./epdcal` 실행 파일 생성

`make build-pi`와 `make build-pi64`는 현재 C 드라이버를 포함하지 않아 실제
EPD 출력에는 사용할 수 없다. 개발 머신에서 전체 빌드는 `cd webui && npm ci`
후 저장소 루트에서 `make build`로 실행한다.

#### 4.2.3 Web UI 빌드 / cross-build 참고

리소스가 제한적인 Raspberry Pi Zero 2W 등에서는 Next.js Web UI 빌드를
개발 머신에서 수행한 뒤, 정적 파일만 Pi 로 가져오는 플로우를 사용할 수 있다:

- 개발 머신에서:
  - `webui` 디렉터리에서 Next.js 빌드 및 export 를 수행하고,
  - 결과물을 `internal/web/static` 으로 복사한 뒤,
  - 이를 `webui.zip` 으로 압축해 Pi 로 전송.
- Raspberry Pi 에서는:
  - `webui.zip` 을 풀어 `internal/web/static` 을 복원한 뒤,
  - 위 4.2.1/4.2.2 단계에 따라 C 라이브러리 및 Go 바이너리를 빌드.

자세한 빌드/배포 플로우는 `[`progress.md`](progress.md:1)` 의 최신 업데이트를 참고할 수 있다.

### 4.3 설치 (예시)

```bash
# Pi에서 C 드라이버 포함 새 바이너리 생성
make build-pi-cgo

# 바이너리, unit 파일 설치 및 실행 중 서비스 재시작
sudo make systemd-install
```

`systemd-install`은 `epdcal` 계정과 그룹, 설정 파일, 캐시/에셋 디렉터리의
권한을 맞추고 unit 파일을 다시 생성한다. 실행 중인 서비스가 있으면
`daemon-reload` 후 자동으로 재시작하므로 새로 설치한 바이너리가 즉시 사용된다.
Pi에서는 먼저 `make build-pi-cgo`로 바이너리를 생성해야 한다.
설정 파일이 없으면 샘플을 설치한다. Web UI에서 저장한 설정은
파일과 실행 중 메모리에 함께 반영된다.

설치 시 `/etc/default/epdcal`도 없을 때만 생성한다. OpenWeather 키를 쓰려면
유닛 파일을 직접 수정하지 말고 다음처럼 환경 파일만 편집한다.

```bash
sudoedit /etc/default/epdcal
# EPDCAL_OPENWEATHER_API_KEY=발급받은키
sudo systemctl restart epdcal
```

재설치는 기존 환경 파일을 덮어쓰지 않는다. 이 파일은 `root:epdcal` 소유,
`0640` 권한으로 설치되어 서비스 프로세스가 읽을 수 있다. 키는 YAML과 웹 API에
저장/노출되지 않는다. 기본 경로는 `/etc/default/epdcal`이며, 설치 시 `ENVFILE=/다른/경로`
Make 변수를 지정하면 환경 파일 설치 위치와 생성되는 unit의 `EnvironmentFile` 경로를
함께 변경할 수 있다. 예: `sudo make systemd-install ENVFILE=/etc/epdcal/epdcal.env`.
경로 변경 뒤 재설치할 때도 같은 `ENVFILE` 값을 사용해야 한다.

Web UI의 `/config`에서 설정을 저장하면 `/etc/epdcal/config.yaml`에 기록된다.
새 ICS 목록, 휴일 규칙, 인증은 다음 HTTP 요청부터 사용한다. cron 주기와
타임존은 다음 예약 시각 계산부터 사용한다. 이미 진행 중인 ICS 조회는 이전
설정을 사용할 수 있지만, 이후 HTTP 요청은 새 설정을 읽는다. HTTP `listen` 주소는 열린 소켓이므로 Web UI에서 수정하지
않으며, 파일에서 직접 변경했다면 서비스를 재시작해야 한다.
권한 문제로 저장에 실패하면
`sudo journalctl -u epdcal -n 100`에서 오류를 확인한다.
`/config`의 **지금 EPD 갱신**은 현재 실행 중인 설정으로 ICS 조회,
Go 렌더링, EPD 출력을 바로 수행한다. 설정을 저장한 뒤 누르면 새 규칙을
반영할 수 있다.
수동 요청은 서버에서 5분 쿨다운을 적용하며, 예약/초기 갱신이 실행 중이면
겹쳐 실행하지 않는다. **Preview 새로고침**은 이미 만들어진 PNG만 다시 불러온다.
설정 변경과 EPD 제어가 가능한 웹 서버이므로 외부에 공개할 경우
Web 로그인과 신뢰할 수 있는 네트워크 접근 제한을 함께 사용한다.

---

## 5. 설정 파일 (`/etc/epdcal/config.yaml`)

### 5.1 예시

```yaml
listen: "127.0.0.1:8080"
timezone: "Asia/Seoul"
refresh: "*/15 * * * *"     # 15분마다
horizon_days: 7
show_all_day: true
highlight_red:
  - "중요"
  - "휴가"
  - "deadline"
holiday_prefixes:
  - "쉬는 날"             # 제목이 이 문자열로 시작하면 휴일

ics:
  - id: "personal"
    url: "https://example.com/personal.ics"
  - id: "work"
    url: "https://example.com/work.ics"

basic_auth:
  username: "admin"
  password: "change-me"
```

### 달력 레이아웃 JSON과 에셋

`/config`의 **달력 레이아웃 JSON**에서 글자 크기, 일정 시간 표시, 날짜별 일정 수,
그리드 위치와 이미지 오버레이를 조정할 수 있다. 예를 들어:

```json
{
  "show_weather": true,
  "show_battery": true,
  "show_event_times": true,
  "show_empty_days": true,
  "max_events_per_day": 3,
  "header_font_size": 36,
  "date_font_size": 16,
  "event_font_size": 12,
  "grid_top": 179,
  "assets": [
    { "file": "logo.png", "x": 24, "y": 132, "width": 120, "height": 40 }
  ]
}
```

커스텀 이미지는 `/var/lib/epdcal/assets/`에 PNG/JPEG로 넣고 JSON에는 파일명만
지정한다. 예: `sudo install -o epdcal -g epdcal -m 0644 logo.png /var/lib/epdcal/assets/logo.png`.
파일 크기·이미지 크기·표시 좌표는 제한되며, 경로 이동이나 임의 코드
실행은 지원하지 않는다. 설치 전 로컬 미리보기는 프로젝트의 `./assets/`를 사용한다.

날씨 위치와 활성화는 `/config`에서 설정하고, JSON의 `show_weather`로 표시 여부를
조절한다. 화면에는 달력 헤더의 마지막 업데이트 옆에 위치, 현재 상태, 현재 기온을 한 줄로
간단히 표시하며, 달력 그리드 위치/크기는 날씨 표시 때문에 바뀌지 않는다. OpenWeather Current Weather API 2.5의
현재 상태만 조회하므로 OpenWeather 계정에서 해당 API 구독이 활성화되어
있어야 한다. 날씨에는 별도 갱신 스케줄이 없으며, 전체 EPD 갱신(`refresh` cron) 시
날씨 데이터를 확인한다. 예를 들어 `refresh: "*/30 * * * *"`로 설정하면 30분마다
달력 전체를 다시 렌더링하고 EPD도 갱신한다. `refresh: "0 * * * *"`는 매시간 갱신이다.
이는 날씨 패널만 따로 갱신하는 주기가 아니라 전체 화면의 갱신 주기다. 정상 응답은
30분, 오류 응답은 5분 메모리 캐시되므로 전체 갱신 주기가 30분보다 짧으면 화면은
갱신되어도 날씨 데이터는 캐시된 값일 수 있다.

위도/경도는 각각 `-90~90`, `-180~180`의 유한한 숫자여야 하며 소수점 이하 최대 6자리까지
허용한다. 지역명은 화면에 보여줄 라벨이고 실제 조회 위치는 위도/경도로 결정된다.

주요 필드:

- `listen`: HTTP 서버 bind 주소 (`127.0.0.1:8080` 권장)
- `timezone`: 표시용 타임존 (IANA 이름, 예: `Asia/Seoul`)
- `refresh`:
  - cron 스타일 문자열 (예: `*/15 * * * *`)
  - 지정한 스케줄에 맞춰 `fetch + render + display` 수행. 날씨 패널이 켜져 있으면
    이 전체 화면 갱신 때 날씨 데이터도 확인한다.
- `horizon_days`:
  - 이전 설정 파일과의 호환을 위해 보존한다. 현재 5주 달력에는 적용되지 않는다.
- `show_all_day`: all‑day 섹션 표시 여부
- `highlight_red`:
  - 이벤트 제목/설명에 포함될 경우 red plane 으로 강조할 키워드 목록
- `holiday_prefixes`:
  - 이벤트 제목이 접두어로 시작하면 휴일로 판정해 날짜와 일정을 빨간색으로 표시
  - 기본값 `쉬는 날`; `[]`로 지정하면 비활성화
- `ics`:
  - `id`: 내부 식별자
  - `url`: ICS 구독 URL (비공개 URL 포함 가능, **로그에 풀로 찍지 않도록 주의**)
- `basic_auth`:
  - `username`, `password`: 둘 다 설정하면 Web 로그인과 API Basic Auth 활성화
- `layout_json`:
  - JSON string 으로 렌더 레이아웃 옵션을 관리한다. 신규 설정에는 기본 예제가 저장되며,
    오래된 설정에서 누락된 경우에도 기본값이 로드/API 응답에 적용된다.

설정 파일 퍼미션은 **0600** 으로 유지하여 URL/비밀번호가 노출되지 않도록 한다.

---

## 6. Web UI 및 HTTP API

### 6.1 엔드포인트 요약

- `GET /`, `GET /calendar`, `GET /config`:
  메인, 달력, 설정 화면

- `GET /api/config`  
  현재 설정 값을 JSON 형태로 반환

- `POST /api/config`  
  JSON body 를 검증한 뒤 설정 파일과 실행 중 설정에 반영한다.

- `GET /api/ics/status`:
  저장된 ICS URL을 서버에서 직접 요청하고 iCalendar 파싱을 검사한다. 캐시된
  결과로 성공을 보고하지 않는다. URL과 토큰은 응답에 포함하지 않는다.

- `GET /api/events`:
  설정된 ICS 소스에서 읽은 일정 목록을 반환한다.

- `GET /api/battery`:
  배터리 잔량과 전압을 반환한다. 읽을 수 없으면 `available:false`를 반환한다.

- `GET /api/weather`:
  활성화 시 OpenWeather 현재 상태를 반환한다. 패널도 현재 상태만 한 줄로 표시하며, API 키는 응답에 포함하지 않는다.

- `GET /preview.png`  
  마지막 렌더링 결과 PNG 반환.  
  브라우저에서 EPD 에 전송될 화면을 미리 확인할 수 있다.

- `GET /health`  
  헬스 체크용 간단한 OK 응답.  
  Basic Auth 없이도 접근 가능하도록 유지.

- `GET /login`, `POST /login`, `GET/POST /logout`:
  브라우저용 로그인/로그아웃 화면과 세션 쿠키 인증을 제공한다.

### 6.2 보안

- 기본적으로 `listen: "127.0.0.1:8080"` 으로 설정하여 로컬에서만 접속 가능하게 한다.
- 다른 호스트/IP 에서 접근이 필요하다면:
  - `listen: "0.0.0.0:8080"` 처럼 변경
  - **반드시 Web 로그인과 방화벽, VPN 등의 추가 보호를 사용할 것**
- 로그인 세션은 서버 메모리에만 보관되며 서비스 재시작 또는 인증정보 저장 시 초기화된다.
- Plain HTTP에서는 로그인 정보가 암호화되지 않으므로 외부 네트워크에 직접 노출하지 않는다.
- API 클라이언트는 세션 쿠키 또는 기존 HTTP Basic Auth를 사용할 수 있다.

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
- 인증 활성화 시 `/login` 화면에서 사용자명/비밀번호를 입력한다.

---

## 10. systemd 서비스

설치 스크립트는 `Makefile`의 `PREFIX`, `ETCDIR`, `VARLIB` 값을 반영해서
`/etc/systemd/system/epdcal.service` 를 생성한다.

기본 유닛 `systemd/epdcal.service`:

```ini
[Unit]
Description=EPD ICS Calendar Display Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/epdcal --config /etc/epdcal/config.yaml
WorkingDirectory=/var/lib/epdcal
Restart=on-failure
User=epdcal
Group=epdcal
SupplementaryGroups=gpio
SupplementaryGroups=i2c
ExecStartPre=/bin/sleep 60
MemoryDenyWriteExecute=true
ReadWritePaths=/etc/epdcal /var/lib/epdcal

[Install]
WantedBy=multi-user.target
```

설치:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now epdcal
```

상태 확인:

```bash
systemctl status epdcal
journalctl -u epdcal -f
```

### GPIO 권한 확인

EPD 드라이버가 `gpiochip0 Export Failed`를 출력하면 PNG 렌더링 오류와는
별개의 GPIO 접근 실패다. systemd 유닛은 `SupplementaryGroups=gpio`와
`DeviceAllow`를 적용하지만, `sudo -u epdcal ...`로 직접 실행하면 유닛의
보조 그룹/장치 허용 설정은 적용되지 않는다. 우선 설치된 서비스로 확인한다.

```bash
sudo systemctl restart epdcal
systemctl show epdcal -p SupplementaryGroups -p DeviceAllow
id epdcal
getent group gpio
ls -l /dev/gpiochip*
```

서비스로 실행해도 접근이 거부되면 `gpio` 그룹 존재 여부와 gpiochip 장치의
그룹/권한을 확인한다. 직접 실행 테스트가 꼭 필요하면 `epdcal` 계정에
`gpio` 보조 그룹을 추가한 뒤 새 프로세스로 실행한다.

```bash
sudo usermod -aG gpio epdcal
```

이 서비스는 GPIO 초기화 실패 시 렌더 전용 모드로 계속 실행하므로, 이 경우
PNG 생성은 가능할 수 있지만 EPD 패널 업데이트는 되지 않는다.

I2C 배터리 정보를 제대로 읽으려면 다음이 전제되어야 한다.

- Raspberry Pi 에서 I2C 가 활성화되어 있어야 한다
- `/dev/i2c-1` 이 존재해야 한다
- 서비스 계정이 `i2c` 그룹 권한을 가져야 한다

PiSugar 3 기본 주소는 `0x57`이며, 앱은 `/dev/i2c-1`에서 직접 배터리 잔량을 읽는다.
PiSugar 데몬은 사용하지 않는다. 장치, 권한 또는 읽기 오류로 잔량을 알 수 없으면
홈과 캘린더에는 낮은 배터리 아이콘과 `??%`가 표시되고 `/api/battery`는
`{"available":false,"percent":null,"voltage_mv":null}`을 반환한다.
`/api/config`는 설정 파일을 읽고 저장하며, 저장 전 입력을 검증한다.

60초 지연은 부팅 직후 GPIO/I2C 디바이스와 네트워크가 안정화될 시간을 주기 위한 것이다.

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
