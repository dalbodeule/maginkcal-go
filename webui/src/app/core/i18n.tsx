"use client";

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

export type Locale = "ko" | "en";

declare global {
  interface Window {
    __EPDCAL_CONFIG__?: {
      defaultLocale?: string;
    };
  }
}

type Messages = Record<string, string>;

const ko: Messages = {
  // 공통
  "common.app_name": "epdcal",
  "common.goto.calendar": "캘린더 보기",
  "common.goto.config": "설정 / Preview",
  "common.health.check": "백엔드 상태 확인",
  "common.health.again": "다시 확인",
  "common.health.waiting": "확인 대기중...",
  "common.health.ok": "정상",
  "common.health.error": "에러",
  "common.preview.title": "Preview (/preview.png)",

  // 홈(/)
  "home.title": "epdcal",
  "home.subtitle":
    'Raspberry Pi + Waveshare 12.48" e-paper 에 ICS 캘린더를 표시하는 작은 서비스입니다.',
  "home.quick_links": "바로가기",
  "home.section.howto": "사용 방법",
  "home.howto.step1":
    "설정 / Preview 페이지에서 ICS URL, 타임존, 스케줄 등을 설정합니다.",
  "home.howto.step2":
    "설정이 저장되면 백엔드가 주기적으로 ICS 를 fetch/parse/expand 하고, 내부 Go 렌더러가 EPD 이미지를 생성해 출력합니다.",
  "home.howto.step3":
    "/calendar 는 웹 미리보기 화면이며, EPD 출력 이미지는 Go 내부 렌더러가 직접 생성합니다.",
  "home.howto.step4":
    "/preview.png 에서 EPD 출력용 최신 이미지를 확인할 수 있습니다.",
  "home.section.auth": "인증 / 보안",
  "home.auth.description":
    "인증이 설정된 경우 로그인 화면에서 사용자 이름과 비밀번호를 입력하면 메인, 설정, 캘린더에 접근할 수 있습니다.",
  "home.auth.description2":
    "한 번 로그인하면 같은 브라우저 세션 동안은 자동으로 인증이 유지됩니다.",
  "home.footer.description":
    '이 페이지는 단순 안내용 메인 화면이며, 실제 캘린더 렌더링/설정은 상단의 "캘린더 보기" 및 "설정 / Preview" 링크를 통해 진행합니다.',

  // 캘린더(/calendar)
  "calendar.today": "오늘",
  "calendar.no_events": "일정 없음",
  "calendar.all_day_prefix": "종일 · ",
  "calendar.last_updated_prefix": "마지막 업데이트:",
  "calendar.error.load": "데이터를 불러오는 중 오류가 발생했습니다.",

  // 설정(/config)
  "config.title": "epdcal 설정",
  "config.subtitle":
    "ICS 구독 / 타임존 / 스케줄 / 표시 옵션을 설정하고, 우측에서 현재 Preview 이미지를 확인할 수 있습니다.",
  "config.section.settings": "설정",
  "config.section.general": "일반",
  "config.timezone.label": "타임존 (IANA 이름, 예: Asia/Seoul)",
  "config.refresh.label": "Refresh 스케줄 (cron string, 예: */15 * * * *)",
  "config.week_start.label": "주 시작 요일",
  "config.week_start.monday": "월요일",
  "config.week_start.sunday": "일요일",
  "config.show_all_day": "All-day 섹션 표시",
  "config.ics.section_title": "ICS 구독",
  "config.ics.add": "+ 추가",
  "config.ics.empty":
    "아직 등록된 ICS URL 이 없습니다. \"+ 추가\" 버튼을 눌러 캘린더를 등록하세요.",
  "config.ics.id": "ID",
  "config.ics.url": "URL",
  "config.ics.delete": "삭제",
  "config.ics.check": "저장된 ICS 접근 확인",
  "config.ics.checking": "ICS 확인 중...",
  "config.ics.check_hint": "변경한 URL은 저장한 뒤 검사할 수 있습니다.",
  "config.ics.status.ok": "접근 및 파싱 성공",
  "config.ics.status.http_error": "HTTP 오류",
  "config.ics.status.network_error": "네트워크 오류",
  "config.ics.status.invalid_ics": "iCalendar 파싱 실패",
  "config.ics.status.too_large": "파일 크기 초과 (10 MB)",
  "config.ics.events": "개 일정",
  "config.highlight.section_title": "표시 옵션 / 보안",
  "config.highlight.label":
    "Red highlight keywords (쉼표 또는 줄바꿈으로 구분)",
  "config.holiday_prefixes.label": "휴일 제목 접두어 (쉼표 또는 줄바꿈으로 구분)",
  "config.holiday_prefixes.placeholder": "쉬는 날",
  "config.holiday_prefixes.hint": "예: '쉬는 날'로 시작하는 일정은 휴일로 판정해 날짜와 일정을 빨간색으로 표시합니다. 비워두면 규칙을 끕니다.",
  "config.basic_auth.enable":
    "Basic Auth 활성화 (백엔드에서 /health 를 제외한 모든 엔드포인트 보호)",
  "config.basic_auth.username": "Username",
  "config.basic_auth.password": "Password",
  "config.save": "설정 저장",
  "config.saving": "저장 중...",
  "config.loading": "설정을 불러오는 중입니다...",
  "config.load_error": "설정을 불러오는 중 오류가 발생했습니다.",
  "config.save_error": "설정을 저장하는 중 오류가 발생했습니다.",
  "config.save_ok": "설정 파일과 실행 중 설정에 저장했습니다. 다음 조회/갱신부터 사용합니다. 인증을 바꿨다면 페이지를 새로고침하세요.",
  "config.refresh_now": "지금 EPD 갱신",
  "config.refresh_running": "갱신 중입니다. 내부 렌더링과 EPD 출력이 끝나면 Preview가 갱신됩니다.",
  "config.refresh_cooldown": "다음 수동 갱신까지",
  "config.refresh_ready": "수동 갱신 가능",
  "config.refresh_hint": "수동 갱신은 5분 쿨다운이 적용됩니다. 저장한 설정은 다음 갱신부터 사용합니다.",
  "config.preview.refresh": "Preview 새로고침",
  "config.preview.hint":
    "최신 EPD 렌더 결과를 확인하려면 \"Preview 새로고침\" 버튼을 누르거나 브라우저 캐시를 무시하고 다시 불러오십시오. 이 이미지는 Go 서버의 /preview.png 엔드포인트에서 제공됩니다.",
  "config.preview.aspect_hint":
    "1304 × 984 EPD 비율에 가깝게 표시됩니다.",
  "config.empty_config":
    "설정을 불러오지 못했습니다. API와 서비스 로그를 확인해 주세요.",

  // 추가: 홈/헬스체크, 캘린더, 설정 관련 보조 문구
  "home.health.request_failed": "요청 실패",
  "common.health.checking": "확인 중...",
  "calendar.loading": "로딩 중...",
  "calendar.no_title": "(제목 없음)",
  "config.nav.label": "페이지",
  "config.preview.error":
    "Preview 이미지를 불러오는 데 실패했습니다. Go 서버에서 /preview.png 가 제공되는지 확인하세요.",
};

const en: Messages = {
  // Common
  "common.app_name": "epdcal",
  "common.goto.calendar": "View calendar",
  "common.goto.config": "Settings / Preview",
  "common.health.check": "Backend health:",
  "common.health.again": "Check again",
  "common.health.waiting": "Waiting...",
  "common.health.ok": "OK",
  "common.health.error": "Error",
  "common.preview.title": "Preview (/preview.png)",

  // Home (/)
  "home.title": "epdcal",
  "home.subtitle":
    'A small service that renders an ICS calendar to a Raspberry Pi + Waveshare 12.48" e-paper.',
  "home.quick_links": "Shortcuts",
  "home.section.howto": "How to use",
  "home.howto.step1":
    "Configure ICS URL, timezone, and schedule on the Settings / Preview page.",
  "home.howto.step2":
    "Once saved, the backend periodically fetches/parses/expands ICS, then the internal Go renderer creates and sends the EPD image.",
  "home.howto.step3":
    "/calendar is a web preview; the EPD image is generated directly by the internal Go renderer.",
  "home.howto.step4":
    "You can see the latest EPD render at /preview.png.",
  "home.section.auth": "Authentication / Security",
  "home.auth.description":
    "When authentication is enabled, use the login page to access the main, settings, and calendar screens.",
  "home.auth.description2":
    "After a successful login, the browser will keep the session authenticated.",
  "home.footer.description":
    'This page is just a simple landing page; actual calendar rendering/configuration is done via "View calendar" and "Settings / Preview" links above.',

  // Calendar (/calendar)
  "calendar.today": "Today",
  "calendar.no_events": "No events",
  "calendar.all_day_prefix": "All-day · ",
  "calendar.last_updated_prefix": "Last updated:",
  "calendar.error.load":
    "An error occurred while loading data.",

  // Config (/config)
  "config.title": "epdcal Settings",
  "config.subtitle":
    "Configure ICS subscriptions / timezone / schedule / display options and see the current preview image on the right.",
  "config.section.settings": "Settings",
  "config.section.general": "General",
  "config.timezone.label": "Timezone (IANA name, e.g. Asia/Seoul)",
  "config.refresh.label": "Refresh schedule (cron string, e.g. */15 * * * *)",
  "config.week_start.label": "Week start",
  "config.week_start.monday": "Monday",
  "config.week_start.sunday": "Sunday",
  "config.show_all_day": "Show all-day section",
  "config.ics.section_title": "ICS subscriptions",
  "config.ics.add": "+ Add",
  "config.ics.empty":
    "No ICS URL is registered yet. Click \"+ Add\" to register a calendar.",
  "config.ics.id": "ID",
  "config.ics.url": "URL",
  "config.ics.delete": "Delete",
  "config.ics.check": "Check saved ICS access",
  "config.ics.checking": "Checking ICS...",
  "config.ics.check_hint": "Save URL changes before checking.",
  "config.ics.status.ok": "Reachable and parsed",
  "config.ics.status.http_error": "HTTP error",
  "config.ics.status.network_error": "Network error",
  "config.ics.status.invalid_ics": "Invalid iCalendar",
  "config.ics.status.too_large": "File exceeds 10 MB",
  "config.ics.events": "events",
  "config.highlight.section_title": "Display options / Security",
  "config.highlight.label":
    "Red highlight keywords (separated by comma or newline)",
  "config.holiday_prefixes.label": "Holiday title prefixes (comma or newline separated)",
  "config.holiday_prefixes.placeholder": "쉬는 날",
  "config.holiday_prefixes.hint": "Events whose titles start with a prefix are holidays; their dates and text are red. Leave empty to disable.",
  "config.basic_auth.enable":
    "Enable Basic Auth (protect all endpoints except /health on the backend)",
  "config.basic_auth.username": "Username",
  "config.basic_auth.password": "Password",
  "config.save": "Save settings",
  "config.saving": "Saving...",
  "config.loading": "Loading settings...",
  "config.load_error": "Failed to load settings.",
  "config.save_error": "Failed to save settings.",
  "config.save_ok": "Saved to config.yaml and active memory. New requests/refreshes use these settings. Reload if you changed authentication.",
  "config.refresh_now": "Refresh EPD now",
  "config.refresh_running": "Refreshing. Preview updates after internal rendering and display finish.",
  "config.refresh_cooldown": "Next manual refresh in",
  "config.refresh_ready": "Manual refresh available",
  "config.refresh_hint": "Manual refresh has a 5-minute cooldown. Saved settings apply on the next refresh.",
  "config.preview.refresh": "Refresh preview",
  "config.preview.hint":
    "To see the latest EPD render, click \"Refresh preview\" or reload ignoring browser cache. This image is served from the Go server's /preview.png endpoint.",
  "config.preview.aspect_hint":
    "Displayed with an aspect ratio close to 1304 × 984 for the EPD.",
  "config.empty_config":
    "Could not load settings. Check the API and service logs.",

  // Extra: home/health check, calendar, config helper messages
  "home.health.request_failed": "Request failed",
  "common.health.checking": "Checking...",
  "calendar.loading": "Loading...",
  "calendar.no_title": "(No title)",
  "config.nav.label": "Pages",
  "config.preview.error":
    "Failed to load preview image. Please check if /preview.png is served by the Go backend.",
};

const messagesByLocale: Record<Locale, Messages> = {
  ko,
  en,
};

const STORAGE_KEY = "epdcal.locale";
/**
 * 기본 로케일:
 * - 브라우저의 /calendar 미리보기는 쿼리스트링(lang)으로 언어를 지정할 수 있음
 * - 그 외 페이지 및 쿼리 없음: 브라우저 언어 기준, 없으면 EN 으로 fallback
 */
const DEFAULT_LOCALE: Locale = "en";

function normalizeLocale(raw: string | null | undefined): Locale | null {
  if (!raw) return null;
  const v = raw.toLowerCase();
  if (v.startsWith("ko")) return "ko";
  if (v.startsWith("en")) return "en";
  if (v === "ko" || v === "en") return v as Locale;
  return null;
}

function detectInitialLocale(): Locale {
  if (typeof window === "undefined") {
    return DEFAULT_LOCALE;
  }

  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    const fromStorage = normalizeLocale(stored);
    if (fromStorage) return fromStorage;
  } catch {
    // localStorage 접근 실패 시 무시
  }

  const fromServerConfig = normalizeLocale(
    window.__EPDCAL_CONFIG__?.defaultLocale,
  );
  if (fromServerConfig) return fromServerConfig;

  if (typeof navigator !== "undefined") {
    const fromNav = normalizeLocale(navigator.language);
    if (fromNav) return fromNav;
  }

  return DEFAULT_LOCALE;
}

interface I18nContextValue {
  locale: Locale;
  t: (key: string) => string;
  setLocale: (locale: Locale) => void;
}

const I18nContext = createContext<I18nContextValue | undefined>(undefined);

export interface I18nProviderProps {
  children: React.ReactNode;
  /**
   * 서버/초기 렌더 시 강제로 지정할 기본 로케일.
   * 지정하지 않으면 브라우저 설정/로컬 스토리지를 기준으로 자동 탐지한다.
   */
  initialLocale?: Locale;
}

export function I18nProvider({ children, initialLocale }: I18nProviderProps) {
  const [locale, setLocaleState] = useState<Locale>(
    initialLocale ?? DEFAULT_LOCALE,
  );

  // 클라이언트 마운트 이후에만 브라우저 기반 로케일 탐지 적용.
  // initialLocale 가 명시된 경우(예: /calendar?lang=...)에는 브라우저 자동 탐지를 건너뛴다.
  useEffect(() => {
    if (typeof window === "undefined") return;
    if (initialLocale) {
      // 명시적인 lang 쿼리 파라미터가 있으면 그대로 사용.
      return;
    }
    const timeout = window.setTimeout(() => {
      setLocaleState(detectInitialLocale());
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [initialLocale]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    if (typeof window !== "undefined") {
      try {
        window.localStorage.setItem(STORAGE_KEY, next);
      } catch {
        // ignore
      }
    }
  }, []);

  const messages = useMemo(() => {
    return messagesByLocale[locale] ?? messagesByLocale[DEFAULT_LOCALE];
  }, [locale]);

  const t = useCallback(
    (key: string): string => {
      if (Object.prototype.hasOwnProperty.call(messages, key)) {
        return messages[key];
      }
      // 키가 없으면 그대로 반환하여, 마이그레이션 과정에서도 뷰가 깨지지 않도록 한다.
      return key;
    },
    [messages],
  );

  const value = useMemo(
    () => ({
      locale,
      t,
      setLocale,
    }),
    [locale, t, setLocale],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error("useI18n must be used within an I18nProvider");
  }
  return ctx;
}

/**
 * 언어 선택용 옵션 리스트. 간단한 셀렉터/토글 UI에서 사용할 수 있다.
 */
export const availableLocales: { value: Locale; label: string }[] = [
  { value: "ko", label: "한국어" },
  { value: "en", label: "English" },
];
