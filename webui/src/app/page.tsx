"use client";

import nanumGothic from "./fonts/nanum";
import { useMemo, useState } from "react";

type WeekStart = "monday" | "sunday";

interface CalendarDay {
  date: Date;
  label: string; // e.g. "1"
  weekdayLabel: string; // e.g. "월"
  isToday: boolean;
  isWeekend: boolean;
}

const WEEKDAYS_MON_FIRST = ["월", "화", "수", "목", "금", "토", "일"];
const WEEKDAYS_SUN_FIRST = ["일", "월", "화", "수", "목", "금", "토"];

export default function Home() {
  const [weekStart, setWeekStart] = useState<WeekStart>("monday");

  const today = useMemo(() => new Date(), []);
  const now = today; // alias

  const { days, startDate, endDate } = useMemo(
    () => buildFiveWeekCalendar(today, weekStart),
    [today, weekStart],
  );

  const weekdayLabels =
    weekStart === "monday" ? WEEKDAYS_MON_FIRST : WEEKDAYS_SUN_FIRST;

  const rangeLabel = `${formatShortDate(startDate)} ~ ${formatShortDate(
    endDate,
  )}`;

  return (
    <div
      className={`${nanumGothic.className} min-h-screen bg-slate-100 text-slate-900 flex flex-col items-center py-4`}
    >
      <main className="w-full max-w-5xl rounded-xl bg-white shadow-sm px-4 py-5 sm:px-6 sm:py-6">
        {/* Header */}
        <header className="mb-4 border-b border-slate-200 pb-3 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl sm:text-3xl font-bold tracking-tight">
              {formatKoreanDate(today)}
            </h1>
            <p className="mt-1 text-xs sm:text-sm text-slate-500">
              {formatKoreanWeekday(today)} · Asia/Seoul ·{" "}
              {now.toLocaleTimeString("ko-KR", {
                hour: "2-digit",
                minute: "2-digit",
              })}
            </p>
            <p className="mt-1 text-xs sm:text-sm text-slate-500">
              표시 범위: {rangeLabel}
            </p>
          </div>

          <div className="flex flex-row items-center gap-2 text-xs sm:text-sm">
            <span className="text-slate-500">주 시작 요일</span>
            <div className="inline-flex rounded-full border border-slate-300 bg-slate-100 p-0.5">
              <button
                type="button"
                onClick={() => setWeekStart("monday")}
                className={`px-3 py-1 rounded-full transition-colors ${
                  weekStart === "monday"
                    ? "bg-slate-900 text-white"
                    : "text-slate-700 hover:bg-slate-200"
                }`}
              >
                월요일
              </button>
              <button
                type="button"
                onClick={() => setWeekStart("sunday")}
                className={`px-3 py-1 rounded-full transition-colors ${
                  weekStart === "sunday"
                    ? "bg-slate-900 text-white"
                    : "text-slate-700 hover:bg-slate-200"
                }`}
              >
                일요일
              </button>
            </div>
          </div>
        </header>

        {/* Calendar */}
        <section className="space-y-2">
          {/* 요일 헤더 */}
          <div className="grid grid-cols-7 text-center text-[11px] sm:text-xs font-semibold text-slate-500">
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
          <div className="grid grid-cols-7 gap-px rounded-lg border border-slate-300 bg-slate-300 overflow-hidden">
            {days.map((day, idx) => (
              <div
                key={idx}
                className={`min-h-16 bg-white px-1.5 py-1.5 flex flex-col ${
                  day.isToday ? "bg-slate-900/5" : ""
                }`}
              >
                {/* 날짜 헤더(숫자 + 오늘 표시) */}
                <div className="flex items-center justify-between mb-1">
                  <div className="flex items-baseline gap-1">
                    <span
                      className={`text-[11px] sm:text-xs font-semibold ${
                        day.isWeekend ? "text-red-600" : "text-slate-900"
                      } ${day.isToday ? "underline decoration-2" : ""}`}
                    >
                      {day.label}
                    </span>
                    {day.isToday && (
                      <span className="text-[10px] text-slate-500">오늘</span>
                    )}
                  </div>
                </div>

                {/* 일정 내용 영역 (지금은 비워 두고, 추후 /api/events 데이터 렌더링) */}
                <div className="flex-1">
                  <p className="text-[9px] sm:text-[10px] text-slate-300">
                    {/* TODO: /api/events 데이터를 바인딩해서 일정 요약 표시 */}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </section>
      </main>
    </div>
  );
}

// Helpers

function buildFiveWeekCalendar(
  today: Date,
  weekStart: WeekStart,
): { days: CalendarDay[]; startDate: Date; endDate: Date } {
  const base = startOfWeek(today, weekStart);
  const days: CalendarDay[] = [];
  for (let i = 0; i < 35; i++) {
    const d = addDays(base, i);
    days.push({
      date: d,
      label: String(d.getDate()),
      weekdayLabel: formatKoreanWeekday(d),
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

function formatKoreanDate(date: Date): string {
  return date.toLocaleDateString("ko-KR", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

function formatKoreanWeekday(date: Date): string {
  return date.toLocaleDateString("ko-KR", { weekday: "short" });
}

function formatShortDate(date: Date): string {
  const m = date.getMonth() + 1;
  const d = date.getDate();
  return `${m}/${d}`;
}
