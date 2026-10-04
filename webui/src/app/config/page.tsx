"use client";

import { useEffect, useMemo, useState } from "react";
import Image from "next/image";
import nanumGothic from "../fonts/nanum";
import { I18nProvider, useI18n } from "@/app/core/i18n";

type WeekStart = "monday" | "sunday";

interface ICSConfigItem {
  id: string;
  url: string;
}

interface BasicAuthConfig {
  enabled: boolean;
  username: string;
  password: string;
}

interface WeatherConfig {
  enabled: boolean;
  location: string;
  latitude: number | null;
  longitude: number | null;
}

interface AppConfig {
  listen: string;
  timezone: string;
  refresh: string;
  horizon_days: number;
  show_all_day: boolean;
  highlight_red_keywords: string[];
  holiday_prefixes: string[];
  week_start?: WeekStart;
  ics: ICSConfigItem[];
  basic_auth?: BasicAuthConfig;
  weather: WeatherConfig;
  font_family: string;
  layout_json: string;
}

interface RefreshStatus {
  running: boolean;
  last_started_at?: string;
  last_ended_at?: string;
  next_allowed_at?: string;
  last_error?: string;
}

interface ICSStatus {
  id: string;
  state: "ok" | "http_error" | "network_error" | "invalid_ics" | "too_large";
  http_status?: number;
  event_count?: number;
}

function parseWeatherCoordinate(raw: string, min: number, max: number): number | null {
  const value = raw.trim();
  if (!/^-?(?:\d+(?:\.\d{0,6})?|\.\d{1,6})$/.test(value)) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= min && parsed <= max ? parsed : null;
}

function ConfigContent() {
  const { t } = useI18n();

  const [config, setConfig] = useState<AppConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [previewReloadKey, setPreviewReloadKey] = useState(0);
  const [refreshStatus, setRefreshStatus] = useState<RefreshStatus | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [clock, setClock] = useState(0);
  const [holidayText, setHolidayText] = useState("");
  const [weatherCoordinateDraft, setWeatherCoordinateDraft] = useState({ latitude: "0", longitude: "0" });
  const [icsStatuses, setICSStatuses] = useState<ICSStatus[] | null>(null);
  const [checkingICS, setCheckingICS] = useState(false);
  const [icsCheckError, setICSCheckError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let previousRunning = false;
    const poll = async () => {
      try {
        const res = await fetch("/api/refresh", { cache: "no-store" });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const status: RefreshStatus = await res.json();
        if (cancelled) return;
        if (previousRunning && !status.running && !status.last_error) {
          setPreviewReloadKey((key) => key + 1);
        }
        previousRunning = status.running;
        setRefreshStatus(status);
      } catch (e: unknown) {
        if (!cancelled) setRefreshError(e instanceof Error ? e.message : String(e));
      }
    };
    void poll();
    const clockInterval = window.setInterval(() => setClock(Date.now()), 1000);
    const pollInterval = window.setInterval(() => void poll(), 3000);
    return () => {
      cancelled = true;
      window.clearInterval(clockInterval);
      window.clearInterval(pollInterval);
    };
  }, []);

  const cooldownSeconds = refreshStatus?.next_allowed_at
    ? Math.max(0, Math.ceil((new Date(refreshStatus.next_allowed_at).getTime() - clock) / 1000))
    : 0;

  const requestRefresh = async () => {
    setRefreshError(null);
    try {
      const res = await fetch("/api/refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      });
      const status: RefreshStatus = await res.json();
      setRefreshStatus(status);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (e: unknown) {
      setRefreshError(e instanceof Error ? e.message : String(e));
    }
  };

  // /api/config 로부터 설정을 가져온다.
  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        setLoading(true);
        const res = await fetch("/api/config");
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`);
        }
        const data: AppConfig = await res.json();
        if (cancelled) return;

        // 기본값 보정
        const safeConfig: AppConfig = {
          listen: data.listen || "127.0.0.1:8080",
          timezone: data.timezone || "Asia/Seoul",
          refresh: data.refresh || "*/15 * * * *",
          horizon_days: data.horizon_days || 7,
          show_all_day:
            typeof data.show_all_day === "boolean" ? data.show_all_day : true,
          highlight_red_keywords: data.highlight_red_keywords || [],
          holiday_prefixes: data.holiday_prefixes ?? [],
          week_start: data.week_start === "sunday" ? "sunday" : "monday",
          ics: data.ics || [],
          basic_auth: data.basic_auth || {
            enabled: false,
            username: "",
            password: "",
          },
          weather: data.weather || { enabled: false, location: "", latitude: null, longitude: null },
          font_family: data.font_family || "",
          layout_json: data.layout_json || "{}",
        };

        setConfig(safeConfig);
        setWeatherCoordinateDraft({
          latitude: safeConfig.weather.latitude == null ? "" : String(safeConfig.weather.latitude),
          longitude: safeConfig.weather.longitude == null ? "" : String(safeConfig.weather.longitude),
        });
        setHolidayText(safeConfig.holiday_prefixes.join("\n"));
        setError(null);
      } catch (e: unknown) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : t("config.load_error"));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, [t]);

  const handleSave = async () => {
    if (!config) return;
    let weather = config.weather;
    if (weather.enabled) {
      const latitude = parseWeatherCoordinate(weatherCoordinateDraft.latitude, -90, 90);
      const longitude = parseWeatherCoordinate(weatherCoordinateDraft.longitude, -180, 180);
      if (latitude === null) {
        setError("위도는 -90부터 90 사이, 소수점 이하 6자리 이내로 입력하세요.");
        return;
      }
      if (longitude === null) {
        setError("경도는 -180부터 180 사이, 소수점 이하 6자리 이내로 입력하세요.");
        return;
      }
      weather = { ...weather, latitude, longitude };
    }
    setSaving(true);
    setSaveMessage(null);
    setError(null);
    try {
      const res = await fetch("/api/config", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          ...config,
          weather,
          holiday_prefixes: holidayText.split(/[,\n]/).map((item) => item.trim()).filter(Boolean),
        }),
      });
      if (!res.ok) {
        const data: { error?: string } = await res.json().catch(() => ({}));
        throw new Error(data.error ?? `HTTP ${res.status}`);
      }
      setConfig({ ...config, weather });
      setWeatherCoordinateDraft({
        latitude: weather.latitude == null ? "" : String(weather.latitude),
        longitude: weather.longitude == null ? "" : String(weather.longitude),
      });
      setSaveMessage(t("config.save_ok"));
      setICSStatuses(null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t("config.save_error"));
    } finally {
      setSaving(false);
    }
  };

  const checkICSAccess = async () => {
    setCheckingICS(true);
    setICSCheckError(null);
    try {
      const res = await fetch("/api/ics/status", { cache: "no-store" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data: { sources: ICSStatus[] } = await res.json();
      setICSStatuses(data.sources);
    } catch (e: unknown) {
      setICSCheckError(e instanceof Error ? e.message : String(e));
    } finally {
      setCheckingICS(false);
    }
  };

  const handleAddICS = () => {
    if (!config) return;
    const usedIds = new Set(config.ics.map((item) => item.id));
    let nextId = 1;
    while (usedIds.has(`calendar-${nextId}`)) nextId++;
    const next: AppConfig = {
      ...config,
      ics: [
        ...config.ics,
        { id: `calendar-${nextId}`, url: "" },
      ],
    };
    setConfig(next);
    setICSStatuses(null);
  };

  const handleRemoveICS = (index: number) => {
    if (!config) return;
    const nextList = config.ics.slice();
    nextList.splice(index, 1);
    setConfig({ ...config, ics: nextList });
    setICSStatuses(null);
  };

  const handleUpdateICS = (
    index: number,
    field: keyof ICSConfigItem,
    value: string,
  ) => {
    if (!config) return;
    const nextList = config.ics.map((item, i) =>
      i === index ? { ...item, [field]: value } : item,
    );
    setConfig({ ...config, ics: nextList });
    setICSStatuses(null);
  };

  const handleToggleAllDay = () => {
    if (!config) return;
    setConfig({ ...config, show_all_day: !config.show_all_day });
  };

  const handleWeekStartChange = (val: WeekStart) => {
    if (!config) return;
    setConfig({ ...config, week_start: val });
  };

  const handleKeywordsChange = (value: string) => {
    if (!config) return;
    const tokens = value
      .split(/[,\n]/)
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    setConfig({ ...config, highlight_red_keywords: tokens });
  };

  const handleBasicAuthEnabled = (enabled: boolean) => {
    if (!config) return;
    const nextAuth: BasicAuthConfig = {
      enabled,
      username: config.basic_auth?.username ?? "",
      password: config.basic_auth?.password ?? "",
    };
    setConfig({ ...config, basic_auth: nextAuth });
  };

  const handleBasicAuthField = (
    field: keyof BasicAuthConfig,
    value: string,
  ) => {
    if (!config) return;
    const nextAuth: BasicAuthConfig = {
      enabled: config.basic_auth?.enabled ?? false,
      username: config.basic_auth?.username ?? "",
      password: config.basic_auth?.password ?? "",
      [field]: value,
    };
    setConfig({ ...config, basic_auth: nextAuth });
  };

  const previewUrl = useMemo(
    () => `/preview.png?t=${previewReloadKey}`,
    [previewReloadKey],
  );

  return (
    <div
      className={`${nanumGothic.className} min-h-screen bg-[#eef2f1] text-slate-900 px-3 py-4 sm:px-6 lg:py-8`}
    >
      <main className="mx-auto w-full max-w-7xl rounded-[1.5rem] border border-slate-200/80 bg-[#fbfcfb] shadow-[0_18px_50px_rgba(15,23,42,0.08)] px-4 py-5 sm:px-7 sm:py-7">
		<header className="mb-7 flex flex-col gap-5 border-b border-slate-200 pb-6 lg:flex-row lg:items-end lg:justify-between [word-break:keep-all]">
          <div>
            <p className="mb-2 text-[10px] font-bold uppercase tracking-[0.2em] text-teal-700">
              EPD CONTROL CENTER
            </p>
            <h1 className="text-2xl font-bold tracking-tight text-slate-950 sm:text-4xl">
              {t("config.title")}
            </h1>
            <p className="mt-2 max-w-2xl text-xs leading-5 text-slate-500 sm:text-sm">
              {t("config.subtitle")}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-3 text-xs sm:text-sm">
            <span className="whitespace-nowrap text-[11px] font-semibold text-slate-400">{t("config.nav.label")}</span>
            <div className="inline-flex shrink-0 rounded-xl border border-slate-200 bg-white p-1 shadow-sm">
              <a
                href="/calendar"
                className="rounded-lg px-3 py-1.5 whitespace-nowrap text-slate-600 transition hover:bg-slate-100"
              >
                {t("common.goto.calendar")}
              </a>
              <a
                href="/config"
                className="rounded-lg bg-slate-900 px-3 py-1.5 whitespace-nowrap text-white shadow-sm"
              >
                {t("common.goto.config")}
              </a>
            </div>
          </div>
        </header>

        {loading && (
          <div className="mb-4 rounded-xl border border-slate-200 bg-white px-4 py-3 text-xs text-slate-600 shadow-sm">
            {t("config.loading")}
          </div>
        )}

        {error && (
          <div className="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-xs text-red-700">
            {error}
          </div>
        )}

        {saveMessage && (
          <div className="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs text-emerald-700">
            {saveMessage}
          </div>
        )}

        <section className="grid grid-cols-1 gap-7 lg:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]">
          {/* Left: Config form */}
          <div className="space-y-4">
            <h2 className="text-base font-bold text-slate-950 sm:text-lg">
              {t("config.section.settings")}
            </h2>

            {!config ? (
              <p className="text-xs text-slate-500">
                {t("config.empty_config")}
              </p>
            ) : (
              <>
                {/* General */}
                <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:p-5 space-y-4">
                  <h3 className="text-sm font-bold text-slate-900">
                    {t("config.section.general")}
                  </h3>
                  <div className="space-y-2">
                    <label className="block text-xs font-medium text-slate-600">
                      {t("config.timezone.label")}
                      <input
                        type="text"
                        value={config.timezone}
                        onChange={(e) =>
                          setConfig({ ...config, timezone: e.target.value })
                        }
                        className="mt-1.5 h-9 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs outline-none transition focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                      />
                    </label>
                    <label className="block text-xs font-medium text-slate-600">
                      {t("config.refresh.label")}
                      <input
                        type="text"
                        value={config.refresh}
                        onChange={(e) =>
                          setConfig({ ...config, refresh: e.target.value })
                        }
                        className="mt-1.5 h-9 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs outline-none transition focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                      />
                    </label>
                    <label className="block text-xs font-medium text-slate-600">
                      글꼴 패밀리 (예: 나눔고딕, Noto Sans KR)
                      <input
                        type="text"
                        value={config.font_family}
                        onChange={(e) =>
                          setConfig({ ...config, font_family: e.target.value })
                        }
                        placeholder="자동 선택"
                        className="mt-1.5 h-9 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs outline-none transition focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                      />
                      <span className="mt-1 block text-[11px] text-slate-500">
                        비워 두면 시스템에서 한글 글리프가 있는 글꼴을 자동 선택합니다. EPDCAL_FONT_REGULAR를 지정하면 해당 파일이 우선합니다.
                      </span>
                    </label>
                    <div className="flex items-center justify-between gap-2">
                      <label className="flex flex-col justify-end text-[11px] text-slate-600">
                        {t("config.week_start.label")}
                        <div className="mt-1 inline-flex rounded-full border border-slate-300 bg-slate-100 p-0.5">
                          <button
                            type="button"
                            onClick={() => handleWeekStartChange("monday")}
                            className={`px-3 py-1 rounded-full text-xs transition-colors ${
                              config.week_start === "monday"
                                ? "bg-slate-900 text-white"
                                : "text-slate-700 hover:bg-slate-200"
                            }`}
                          >
                            {t("config.week_start.monday")}
                          </button>
                          <button
                            type="button"
                            onClick={() => handleWeekStartChange("sunday")}
                            className={`px-3 py-1 rounded-full text-xs transition-colors ${
                              config.week_start === "sunday"
                                ? "bg-slate-900 text-white"
                                : "text-slate-700 hover:bg-slate-200"
                            }`}
                          >
                            {t("config.week_start.sunday")}
                          </button>
                        </div>
                      </label>
                    </div>
                    <label className="inline-flex items-center gap-2 text-[11px] text-slate-600">
                      <input
                        type="checkbox"
                        checked={config.show_all_day}
                        onChange={handleToggleAllDay}
                        className="h-3 w-3 rounded border-slate-300"
                      />
                      {t("config.show_all_day")}
                    </label>
                  </div>
                </div>

                <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:p-5 space-y-3">
                  <h3 className="text-sm font-bold text-slate-900">날씨 표시</h3>
                  <label className="flex items-center gap-2 text-xs text-slate-600">
                    <input type="checkbox" checked={config.weather.enabled} onChange={(e) => setConfig({ ...config, weather: { ...config.weather, enabled: e.target.checked } })} />
                    OpenWeather 현재 날씨 한 줄 표시
                  </label>
                  <p className="text-[11px] text-slate-500">달력 헤더에 지역 · 현재 상태 · 현재 기온만 간단히 표시하며, 달력 그리드 배치는 바뀌지 않습니다.</p>
                  <p className="text-[11px] text-slate-500">API 키는 아래 레이아웃이나 설정 파일에 저장하지 않습니다. `sudo nano /etc/default/epdcal`에서 EPDCAL_OPENWEATHER_API_KEY 값을 설정하세요.</p>
                  <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                    <input aria-label="날씨 표시 지역명" placeholder="표시 지역명 (예: Seoul)" value={config.weather.location} onChange={(e) => setConfig({ ...config, weather: { ...config.weather, location: e.target.value } })} className="h-9 rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs" />
                    <input aria-label="위도" type="text" inputMode="decimal" placeholder="위도 (-90~90)" value={weatherCoordinateDraft.latitude} onChange={(e) => setWeatherCoordinateDraft({ ...weatherCoordinateDraft, latitude: e.target.value })} className="h-9 rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs" />
                    <input aria-label="경도" type="text" inputMode="decimal" placeholder="경도 (-180~180)" value={weatherCoordinateDraft.longitude} onChange={(e) => setWeatherCoordinateDraft({ ...weatherCoordinateDraft, longitude: e.target.value })} className="h-9 rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs" />
                  </div>
                  <p className="text-[11px] text-slate-500">지역명은 화면 표시용이고, 실제 날씨 조회는 위도·경도 기준입니다. 좌표는 소수점 이하 6자리까지 입력할 수 있습니다.</p>
                </div>

                <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:p-5 space-y-3">
                  <h3 className="text-sm font-bold text-slate-900">달력 레이아웃 JSON</h3>
                  <p className="text-[11px] text-slate-500">화면 크기와 글꼴, 일정 수, 에셋 배치를 JSON으로 조정합니다. 에셋은 `/var/lib/epdcal/assets`에 PNG/JPEG로 넣고 파일명만 지정하세요.</p>
                  <textarea aria-label="달력 레이아웃 JSON" spellCheck={false} rows={14} value={config.layout_json} onChange={(e) => setConfig({ ...config, layout_json: e.target.value })} className="w-full rounded-lg border border-slate-300 bg-slate-50 p-3 font-mono text-xs" />
                </div>

                {/* ICS URLs */}
                <div className="rounded-lg border border-slate-200 p-3 space-y-3">
                  <div className="flex items-center justify-between">
                    <h3 className="text-xs font-semibold text-slate-700">
                      {t("config.ics.section_title")}
                    </h3>
                    <button
                      type="button"
                      onClick={handleAddICS}
                      className="rounded border border-slate-300 bg-slate-50 px-2 py-0.5 text-[11px] text-slate-700 hover:bg-slate-100"
                    >
                      {t("config.ics.add")}
                    </button>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 text-[11px]">
                    <button
                      type="button"
                      onClick={() => void checkICSAccess()}
                      disabled={checkingICS || saving}
                      className="rounded border border-slate-300 bg-slate-50 px-2 py-1 text-slate-700 disabled:opacity-50"
                    >
                      {checkingICS ? t("config.ics.checking") : t("config.ics.check")}
                    </button>
                    <span className="text-slate-500">{t("config.ics.check_hint")}</span>
                  </div>
                  {icsCheckError && <p className="text-[11px] text-red-700">{icsCheckError}</p>}
                  {icsStatuses && (
                    <div className="space-y-1 text-[11px]">
                      {icsStatuses.length === 0 && <p className="text-slate-500">{t("config.ics.empty")}</p>}
                      {icsStatuses.map((item) => (
                        <p key={item.id} className={item.state === "ok" ? "text-emerald-700" : "text-red-700"}>
                          {item.id}: {t(`config.ics.status.${item.state}`)}
                          {item.http_status ? ` (HTTP ${item.http_status})` : ""}
                          {item.state === "ok" ? ` · ${item.event_count ?? 0} ${t("config.ics.events")}` : ""}
                        </p>
                      ))}
                    </div>
                  )}
                  {config.ics.length === 0 ? (
                    <p className="text-[11px] text-slate-500">
                      {t("config.ics.empty")}
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {config.ics.map((item, idx) => (
                        <div
                          key={idx}
                          className="rounded-xl border border-slate-200 bg-slate-50/80 p-3 space-y-2"
                        >
                          <div className="flex items-center gap-2">
                            <label className="flex-1 text-xs font-medium text-slate-600">
                              {t("config.ics.id")}
                              <input
                                type="text"
                                value={item.id}
                                onChange={(e) =>
                                  handleUpdateICS(idx, "id", e.target.value)
                                }
                                className="mt-1 h-9 w-full rounded-lg border border-slate-300 bg-white px-3 text-xs outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                              />
                            </label>
                            <button
                              type="button"
                              onClick={() => handleRemoveICS(idx)}
                              className="mt-4 rounded-lg border border-red-200 bg-white px-2.5 py-1.5 text-[11px] font-semibold text-red-700 transition hover:bg-red-50"
                            >
                              {t("config.ics.delete")}
                            </button>
                          </div>
                          <label className="block text-xs font-medium text-slate-600">
                            {t("config.ics.url")}
                            <input
                              type="text"
                              value={item.url}
                              onChange={(e) =>
                                handleUpdateICS(idx, "url", e.target.value)
                              }
                              className="mt-1 h-9 w-full rounded-lg border border-slate-300 bg-white px-3 text-xs outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                            />
                          </label>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Highlight keywords + Basic Auth */}
                <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:p-5 space-y-4">
                  <h3 className="text-sm font-bold text-slate-900">
                    {t("config.highlight.section_title")}
                  </h3>
                  <label className="block text-[11px] text-slate-600">
                    {t("config.highlight.label")}
                    <textarea
                      rows={3}
                      value={config.highlight_red_keywords.join(", ")}
                      onChange={(e) => handleKeywordsChange(e.target.value)}
                      className="mt-1.5 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 py-2 text-xs leading-5 outline-none transition focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                    />
                  </label>
                  <label className="block text-[11px] text-slate-600">
                    {t("config.holiday_prefixes.label")}
                    <textarea
                      rows={2}
                      value={holidayText}
                      onChange={(e) => setHolidayText(e.target.value)}
                      placeholder={t("config.holiday_prefixes.placeholder")}
                      className="mt-1.5 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 py-2 text-xs leading-5 outline-none transition focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                    />
                    <span className="mt-1 block text-slate-500">{t("config.holiday_prefixes.hint")}</span>
                  </label>

                  <div className="border-t border-slate-200 pt-2 space-y-2">
                    <label className="inline-flex items-center gap-2 text-[11px] text-slate-600">
                      <input
                        type="checkbox"
                        checked={config.basic_auth?.enabled ?? false}
                        onChange={(e) =>
                          handleBasicAuthEnabled(e.target.checked)
                        }
                        className="h-4 w-4 rounded border-slate-300 accent-teal-700"
                      />
                      {t("config.basic_auth.enable")}
                    </label>
                    {config.basic_auth?.enabled && (
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                        <label className="text-[11px] text-slate-600">
                          {t("config.basic_auth.username")}
                          <input
                            type="text"
                            value={config.basic_auth.username}
                            onChange={(e) =>
                              handleBasicAuthField("username", e.target.value)
                            }
                          className="mt-1 h-9 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs outline-none focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                          />
                        </label>
                        <label className="text-[11px] text-slate-600">
                          {t("config.basic_auth.password")}
                          <input
                            type="password"
                            value={config.basic_auth.password}
                            onChange={(e) =>
                              handleBasicAuthField("password", e.target.value)
                            }
                            className="mt-1 h-9 w-full rounded-lg border border-slate-300 bg-slate-50 px-3 text-xs outline-none focus:border-teal-600 focus:bg-white focus:ring-2 focus:ring-teal-100"
                          />
                        </label>
                      </div>
                    )}
                  </div>
                </div>

                <div className="sticky bottom-3 z-10 flex justify-end rounded-2xl border border-slate-200 bg-white/90 p-2 shadow-lg backdrop-blur">
                  <button
                    type="button"
                    onClick={handleSave}
                    disabled={saving}
                    className="inline-flex items-center rounded-xl bg-teal-700 px-5 py-2.5 text-xs font-bold text-white shadow-sm transition hover:bg-teal-800 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {saving ? t("config.saving") : t("config.save")}
                  </button>
                </div>
              </>
            )}
          </div>

          {/* Right: Preview image */}
          <div className="space-y-5 lg:sticky lg:top-6 lg:self-start">
            <div className="rounded-2xl border border-teal-200 bg-[#e9f5f2] p-4 shadow-sm sm:p-5 text-xs [word-break:keep-all]">
              <div className="mb-3 flex items-start justify-between gap-3">
                <div>
                  <p className="text-[10px] font-bold uppercase tracking-[0.16em] text-teal-700">LIVE ACTION</p>
                  <h2 className="mt-1 text-sm font-bold text-slate-950">{t("config.refresh_now")}</h2>
                </div>
                <span className={`mt-1 h-2.5 w-2.5 rounded-full ${refreshStatus?.running ? "animate-pulse bg-amber-500" : "bg-emerald-500"}`} />
              </div>
              <button
                type="button"
                onClick={() => void requestRefresh()}
                disabled={!refreshStatus || refreshStatus.running || cooldownSeconds > 0}
                className="w-full rounded-xl bg-teal-700 px-3 py-2.5 font-bold text-white shadow-sm transition hover:bg-teal-800 disabled:cursor-not-allowed disabled:bg-slate-400"
              >
                {refreshStatus?.running ? t("config.refresh_running") : t("config.refresh_now")}
              </button>
              <p className="mt-3 text-slate-600">
                {refreshStatus?.running
                  ? t("config.refresh_running")
                  : cooldownSeconds > 0
                    ? `${t("config.refresh_cooldown")} ${Math.floor(cooldownSeconds / 60)}:${String(cooldownSeconds % 60).padStart(2, "0")}`
                    : t("config.refresh_ready")}
              </p>
              {refreshStatus?.last_error && <p className="mt-2 rounded-lg bg-red-50 p-2 text-red-700">{refreshStatus.last_error}</p>}
              {refreshError && <p className="mt-2 rounded-lg bg-red-50 p-2 text-red-700">{refreshError}</p>}
              <p className="mt-3 text-[11px] leading-5 text-slate-500">{t("config.refresh_hint")}</p>
            </div>
            <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
              <div className="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3 sm:px-5">
                <div>
                  <p className="text-[10px] font-bold uppercase tracking-[0.16em] text-slate-400">OUTPUT</p>
                  <h2 className="mt-1 text-sm font-bold text-slate-950 sm:text-base">
                    {t("common.preview.title")}
                  </h2>
                </div>
                <button
                  type="button"
                  onClick={() => setPreviewReloadKey((k) => k + 1)}
                  className="rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 text-[11px] font-semibold text-slate-700 transition hover:bg-slate-100"
                >
                  {t("config.preview.refresh")}
                </button>
              </div>
              <p className="px-4 pt-3 text-[11px] leading-5 text-slate-500 sm:px-5">
                {t("config.preview.hint")}
              </p>
            <div className="relative mx-4 mb-4 mt-3 rounded-xl border border-slate-200 bg-slate-100 p-2 sm:mx-5">
              <div className="bg-slate-900/90 text-white text-[10px] px-1.5 py-0.5 rounded absolute translate-y-[-120%] left-1/2 -translate-x-1/2 hidden lg:inline-flex">
                {t("config.preview.aspect_hint")}
              </div>
              <div className="relative w-full aspect-[1304/984] max-h-[480px] bg-slate-900/5 flex items-center justify-center overflow-hidden">
                <Image
                  key={previewReloadKey}
                  src={previewUrl}
                  alt="EPD preview"
                  width={984}
                  height={1304}
                  unoptimized
                  className="max-w-full max-h-full object-contain border border-slate-300 bg-white"
                  onError={() =>
                    setError(t("config.preview.error"))
                  }
                />
              </div>
            </div>
          </div>
          </div>
        </section>
      </main>
    </div>
  );
}

export default function ConfigPage() {
  return (
    <I18nProvider>
      <ConfigContent />
    </I18nProvider>
  );
}
