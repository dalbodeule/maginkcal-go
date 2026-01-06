"use client";

import { useEffect, useMemo, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faBatteryEmpty,
  faBatteryFull,
  faBatteryQuarter,
  faBatteryHalf,
  faBatteryThreeQuarters,
} from "@fortawesome/free-solid-svg-icons";
import { useSearchParams } from "next/navigation";
import nanumGothic from "../fonts/nanum";
import { I18nProvider, Locale, useI18n } from "@/app/core/i18n";

type WeekStart = "monday" | "sunday";

interface EventsResponse {
  range_start: string;
  range_end: string;
  display_timezone: string;
  week_start?: string;
  occurrences?: OccurrenceDTO[];
}

interface OccurrenceDTO {
  source_id: string;
  uid: string;
  instance_key: string;
  summary: string;
  description: string;
  location: string;
  all_day: boolean;
  start: string;
  end: string;
}

interface CalendarDay {
  date: Date;
  label: string; // e.g. "1"
  weekdayLabel: string; // e.g. "월" / "Mon"
  isToday: boolean;
  isWeekend: boolean;
}

const WEEKDAYS_MON_FIRST: Record<Locale, string[]> = {
  ko: ["월", "화", "수", "목", "금", "토", "일"],
  en: ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"],
};

const WEEKDAYS_SUN_FIRST: Record<Locale, string[]> = {
  ko: ["일", "월", "화", "수", "목", "금", "토"],
  en: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"],
};

function CalendarContent() {
  const { locale, t } = useI18n();
  const [weekStart, setWeekStart] = useState<WeekStart>("monday");
  const [displayTimezone, setDisplayTimezone] = useState("Asia/Seoul");
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [eventsByDate, setEventsByDate] = useState<
    Record<string, OccurrenceDTO[]>
  >({});
  const [batteryPercent, setBatteryPercent] = useState<number | null>(null);
  const [eventsLoaded, setEventsLoaded] = useState(false);
  const [batteryLoaded, setBatteryLoaded] = useState(false);

  const today = useMemo(() => new Date(), []);
  const now = today; // alias

  // /api/events 호출: week_start, display_timezone, 이벤트 목록, 마지막 업데이트 시각만 사용
  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const res = await fetch(window.location.origin + "/api/events");
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`);
        }
        const data: EventsResponse = await res.json();
        if (cancelled) return;

        const apiWeekStart: WeekStart =
          data.week_start === "sunday" ? "sunday" : "monday";
        setWeekStart(apiWeekStart);

        if (data.display_timezone) {
          setDisplayTimezone(data.display_timezone);
        }

        // 날짜별로 occurrence 를 그룹핑
        const grouped: Record<string, OccurrenceDTO[]> = {};
        for (const occ of data.occurrences ?? []) {
          const key = dateKeyFromISO(occ.start);
          if (!grouped[key]) {
            grouped[key] = [];
          }
          grouped[key].push(occ);
        }
        setEventsByDate(grouped);

        // 가장 마지막 업데이트 시각은 클라이언트 기준 fetch 완료 시점으로 사용
        setLastUpdatedAt(new Date());
        setEventsLoaded(true);
        setError(null);
      } catch (e: any) {
        if (!cancelled) {
          setError(e?.message ?? t("calendar.error.load"));
          // 오류가 있어도 화면은 렌더링되도록 eventsLoaded 를 true 로 설정
          setEventsLoaded(true);
        }
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, [t]);

  // /api/battery 호출: 배터리 퍼센트(0~100)를 가져와 5단계 인디케이터에 사용
  useEffect(() => {
    let cancelled = false;

    async function loadBattery() {
      try {
        const res = await fetch(window.location.origin + "/api/battery");
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`);
        }
        const data: { percent?: number } = await res.json();
        if (cancelled) return;

        if (typeof data.percent === "number") {
          let p = data.percent;
          if (p < 0) p = 0;
          if (p > 100) p = 100;
          setBatteryPercent(p);
        }
        // 퍼센트가 없더라도 캡처 진행에는 지장이 없으므로 loaded 로 처리
        setBatteryLoaded(true);
      } catch {
        // 배터리 정보는 필수는 아니므로 에러는 UI에 드러내지 않고 무시하되,
        // 캡처가 data-ready 를 기다리며 멈추지 않도록 loaded 로 표시한다.
        if (!cancelled) {
          setBatteryLoaded(true);
        }
      }
    }

    void loadBattery();

    return () => {
      cancelled = true;
    };
  }, []);

  const { days } = useMemo(
    () => buildFiveWeekCalendar(today, weekStart, locale),
    [today, weekStart, locale],
  );

  const weekdayLabels = useMemo(() => {
    const source =
      weekStart === "monday"
        ? WEEKDAYS_MON_FIRST[locale]
        : WEEKDAYS_SUN_FIRST[locale];
    return source ?? WEEKDAYS_MON_FIRST["en"];
  }, [weekStart, locale]);

  // 캘린더 UI 및 캡처 파이프라인은 /api/events 와 /api/battery 가
  // 모두 성공적으로 로딩된 이후에만 data-ready="true" 로 전환된다.
  const ready = eventsLoaded && batteryLoaded;

  return (
    <div
      data-ready={ready ? "true" : "false"}
      className={`${nanumGothic.className} min-h-screen bg-slate-100 text-slate-900 flex items-center justify-center overflow-auto`}
    >
      <main className="w-[984px] h-[1308px] rounded-xl bg-white shadow-sm px-6 py-6 flex flex-col relative">
        {/* Battery indicator (top-right, 5-step based on FontAwesome semantics) */}
        <BatteryIndicator percent={batteryPercent} />

        {/* Header */}
        <header className="mb-4 border-b border-slate-200 pb-3 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-[64px] sm:text-4xl font-extrabold tracking-tight">
              {formatDate(today, locale)}
            </h1>
            <p className="mt-1 text-[40px] sm:text-base text-slate-700 font-semibold">
              {formatWeekday(today, locale)} · {displayTimezone} ·{" "}
              {now.toLocaleTimeString(localeToIntl(locale), {
                hour: "2-digit",
                minute: "2-digit",
                hour12: false,
              })}
            </p>
            <p className="mt-1 text-[28px] sm:text-sm text-slate-700 font-medium">
              {t("calendar.last_updated_prefix")}{" "}
              {lastUpdatedAt
                ? formatDateTime(lastUpdatedAt, locale)
                : t("calendar.loading")}
            </p>
          </div>
        </header>

        {error && (
          <div className="mb-2 rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
            {error}
          </div>
        )}

        {/* Calendar */}
        <section className="flex-1 flex flex-col space-y-2">
          {/* 요일 헤더 */}
          <div className="grid grid-cols-7 text-center text-[28px] sm:text-sm font-semibold text-slate-500">
            {weekdayLabels.map((w, idx) => {
              const isWeekend =
                (weekStart === "monday" && (idx === 5 || idx === 6)) ||
                (weekStart === "sunday" && (idx === 0 || idx === 6));
              return (
                <div
                  key={w}
                  className={`py-1 ${
                    isWeekend ? "text-red-600" : "text-slate-600"
                  }`}
                >
                  {w}
                </div>
              );
            })}
          </div>

          {/* 날짜 그리드 (5주 = 35일) */}
          <div className="flex-1 grid grid-cols-7 grid-rows-5 gap-px rounded-lg border border-slate-300 bg-slate-300 overflow-hidden">
            {days.map((day, idx) => {
              const inCurrentMonth =
                day.date.getFullYear() === today.getFullYear() &&
                day.date.getMonth() === today.getMonth();

              const dateKey = dateKeyFromDate(day.date);
              const events = eventsByDate[dateKey] ?? [];

              // 색상 규칙:
              // - 이번 달인 평일: 검정
              // - 이번 달인 주말: 빨강
              // - 이번 달이 아닌 주말: 빨강
              // - 이번 달이 아닌 평일: 회색
              const dateColorClass = !inCurrentMonth
                ? day.isWeekend
                  ? "text-red-600"
                  : "text-slate-300"
                : day.isWeekend
                ? "text-red-600"
                : "text-slate-900";

              const cellBgClass = !inCurrentMonth ? "bg-slate-50" : "bg-white";

              return (
                <div
                  key={idx}
                  className={`${cellBgClass} px-1.5 py-1.5 flex flex-col h-full ${
                    day.isToday ? "bg-slate-900/5" : ""
                  }`}
                >
                  {/* 날짜 헤더(숫자 + 오늘 표시) */}
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-baseline gap-1">
                      <span
                        className={`text-[22px] sm:text-base font-bold ${dateColorClass} ${
                          day.isToday ? "underline decoration-2" : ""
                        }`}
                      >
                        {day.label}
                      </span>
                      {day.isToday && (
                        <span className="text-[20px] text-slate-700 font-semibold">
                          {t("calendar.today")}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* 일정 내용 영역: /api/events 데이터 렌더링 */}
                  <div className="flex-1 space-y-0.5">
                    {events.length === 0 ? (
                      <p className="text-[18px] sm:text-xs text-slate-400 font-medium">
                        {t("calendar.no_events")}
                      </p>
                    ) : (
                      events.slice(0, 3).map((ev, i) => (
                        <p
                          key={i}
                          className="text-[18px] sm:text-xs text-slate-900 font-semibold truncate"
                        >
                          {formatEventLine(ev, locale, t)}
                        </p>
                      ))
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      </main>
    </div>
  );
}

export default function CalendarPage() {
  const searchParams = useSearchParams();
  const langParam = searchParams.get("lang");

  let initialLocale: Locale | undefined;
  if (langParam === "ko" || langParam === "en") {
    initialLocale = langParam;
  }

  return (
    <I18nProvider initialLocale={initialLocale}>
      <CalendarContent />
    </I18nProvider>
  );
}

// Helpers

function localeToIntl(locale: Locale): string {
  switch (locale) {
    case "ko":
      return "ko-KR";
    case "en":
    default:
      return "en-US";
  }
}

function buildFiveWeekCalendar(
  today: Date,
  weekStart: WeekStart,
  locale: Locale,
): { days: CalendarDay[]; startDate: Date; endDate: Date } {
  const base = startOfWeek(today, weekStart);
  const days: CalendarDay[] = [];
  for (let i = 0; i < 35; i++) {
    const d = addDays(base, i);
    days.push({
      date: d,
      label: String(d.getDate()),
      weekdayLabel: formatWeekday(d, locale),
      isToday: isSameDate(d, today),
      isWeekend: isWeekend(d),
    });
  }
  const end = addDays(base, 34);
  return { days, startDate: base, endDate: end };
}

function startOfWeek(date: Date, weekStart: WeekStart): Date {
  const d = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const day = d.getDay(); // 0=Sun,1=Mon,...6=Sat
  let diff = 0;

  if (weekStart === "monday") {
    // Monday=0, Sunday=6 로 맞추기
    diff = (day + 6) % 7;
  } else {
    // Sunday=0
    diff = day;
  }

  d.setDate(d.getDate() - diff);
  return d;
}

function addDays(date: Date, days: number): Date {
  const d = new Date(date);
  d.setDate(d.getDate() + days);
  return d;
}

function isSameDate(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function isWeekend(date: Date): boolean {
  const day = date.getDay();
  return day === 0 || day === 6; // Sun or Sat
}

function formatDate(date: Date, locale: Locale): string {
  return date.toLocaleDateString(localeToIntl(locale), {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

function formatWeekday(date: Date, locale: Locale): string {
  return date.toLocaleDateString(localeToIntl(locale), { weekday: "short" });
}

function dateKeyFromDate(date: Date): string {
  const y = date.getFullYear();
  const m = date.getMonth() + 1;
  const d = date.getDate();
  return `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
}

function dateKeyFromISO(iso: string): string {
  const d = new Date(iso);
  return dateKeyFromDate(d);
}

function formatEventLine(
  ev: OccurrenceDTO,
  locale: Locale,
  t: (key: string) => string,
): string {
  const title = ev.summary || t("calendar.no_title");

  if (ev.all_day) {
    // 종일 이벤트: 시간 표시 없이 제목만.
    return `${t("calendar.all_day_prefix")}${title}`;
  }

  const start = new Date(ev.start);
  const end = new Date(ev.end);

  const timeOpts: Intl.DateTimeFormatOptions = {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  };

  const intl = localeToIntl(locale);
  const startStr = start.toLocaleTimeString(intl, timeOpts);
  const endStr = end.toLocaleTimeString(intl, timeOpts);

  return `${startStr}~${endStr} ${title}`;
}

function formatDateTime(date: Date, locale: Locale): string {
  const d = date.toLocaleDateString(localeToIntl(locale), {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
  const tStr = date.toLocaleTimeString(localeToIntl(locale), {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
  return `${d} ${tStr}`;
}

// Battery indicator component and helpers

type BatteryLevel = "empty" | "quarter" | "half" | "three-quarters" | "full";

function batteryLevelFromPercent(p: number | null): BatteryLevel {
  if (p == null) return "empty";
  if (p >= 80) return "full";
  if (p >= 60) return "three-quarters";
  if (p >= 40) return "half";
  if (p >= 20) return "quarter";
  return "empty";
}

// BatteryIndicator renders a 5-step FontAwesome-like battery icon.
function BatteryIndicator(props: { percent: number | null }) {
  const level = batteryLevelFromPercent(props.percent);

  let icon = (
    <FontAwesomeIcon icon={faBatteryEmpty} className="text-[20px]" />
  );
  switch (level) {
    case "quarter":
      icon = (
        <FontAwesomeIcon icon={faBatteryQuarter} className="text-[20px]" />
      );
      break;
    case "half":
      icon = (
        <FontAwesomeIcon icon={faBatteryHalf} className="text-[20px]" />
      );
      break;
    case "three-quarters":
      icon = (
        <
          FontAwesomeIcon
          icon={faBatteryThreeQuarters}
          className="text-[20px]"
        />
      );
      break;
    case "full":
      icon = (
        <FontAwesomeIcon icon={faBatteryFull} className="text-[20px]" />
      );
      break;
  }

  return (
    <div className="absolute top-3 right-4 flex items-center gap-1 text-slate-700">
      {icon}
      {props.percent != null && (
        <span className="text-[20px] font-bold">{props.percent}%</span>
      )}
    </div>
  );
}