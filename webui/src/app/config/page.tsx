"use client";

import { useEffect, useMemo, useState } from "react";
import nanumGothic from "../fonts/nanum";

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

interface AppConfig {
  listen: string;
  timezone: string;
  refresh: string;
  horizon_days: number;
  show_all_day: boolean;
  highlight_red_keywords: string[];
  week_start?: WeekStart;
  ics: ICSConfigItem[];
  basic_auth?: BasicAuthConfig;
}

export default function ConfigPage() {
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [previewReloadKey, setPreviewReloadKey] = useState(0);

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
          week_start: data.week_start === "sunday" ? "sunday" : "monday",
          ics: data.ics || [],
          basic_auth: data.basic_auth || {
            enabled: false,
            username: "",
            password: "",
          },
        };

        setConfig(safeConfig);
        setError(null);
      } catch (e: any) {
        if (!cancelled) {
          // /api/config 가 아직 구현되지 않았거나, 404/500 이면 여기로 들어온다.
          setError(
            e?.message ??
              "설정을 불러오는 중 오류가 발생했습니다. 백엔드 /api/config 구현 상태를 확인하세요.",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();

    return () => {
      cancelled = true;
    };
  }, []);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setSaveMessage(null);
    setError(null);
    try {
      const res = await fetch("/api/config", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(config),
      });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      setSaveMessage("설정이 저장되었습니다.");
    } catch (e: any) {
      setError(
        e?.message ??
          "설정을 저장하는 동안 오류가 발생했습니다. 백엔드 /api/config 구현 상태를 확인하세요.",
      );
    } finally {
      setSaving(false);
    }
  };

  const handleAddICS = () => {
    if (!config) return;
    const next: AppConfig = {
      ...config,
      ics: [
        ...config.ics,
        { id: `calendar-${config.ics.length + 1}`, url: "" },
      ],
    };
    setConfig(next);
  };

  const handleRemoveICS = (index: number) => {
    if (!config) return;
    const nextList = config.ics.slice();
    nextList.splice(index, 1);
    setConfig({ ...config, ics: nextList });
  };

  const handleUpdateICS = (index: number, field: keyof ICSConfigItem, value: string) => {
    if (!config) return;
    const nextList = config.ics.map((item, i) =>
      i === index ? { ...item, [field]: value } : item,
    );
    setConfig({ ...config, ics: nextList });
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

  const handleBasicAuthField = (field: keyof BasicAuthConfig, value: string) => {
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
      className={`${nanumGothic.className} min-h-screen bg-slate-100 text-slate-900 flex flex-col items-center py-4 px-2 sm:px-4`}
    >
      <main className="w-full max-w-6xl rounded-xl bg-white shadow-sm px-4 py-5 sm:px-6 sm:py-6">
        <header className="mb-4 border-b border-slate-200 pb-3 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl sm:text-3xl font-bold tracking-tight">
              epdcal 설정
            </h1>
            <p className="mt-1 text-xs sm:text-sm text-slate-500">
              ICS 구독 / 타임존 / 스케줄 / 표시 옵션을 설정하고, 우측에서 현재
              Preview 이미지를 확인할 수 있습니다.
            </p>
          </div>
          <div className="flex items-center gap-2 text-xs sm:text-sm">
            <span className="text-slate-500">페이지</span>
            <div className="inline-flex rounded-full border border-slate-300 bg-slate-100 p-0.5">
              <a
                href="/calendar"
                className="px-3 py-1 rounded-full text-slate-700 hover:bg-slate-200"
              >
                캘린더 보기
              </a>
              <a
                href="/config"
                className="px-3 py-1 rounded-full bg-slate-900 text-white"
              >
                설정 / Preview
              </a>
            </div>
          </div>
        </header>

        {loading && (
          <div className="mb-3 rounded-md bg-slate-50 px-3 py-2 text-xs text-slate-600">
            설정을 불러오는 중입니다...
          </div>
        )}

        {error && (
          <div className="mb-3 rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
            {error}
          </div>
        )}

        {saveMessage && (
          <div className="mb-3 rounded-md bg-emerald-50 px-3 py-2 text-xs text-emerald-700">
            {saveMessage}
          </div>
        )}

        <section className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left: Config form */}
          <div className="space-y-4">
            <h2 className="text-sm sm:text-base font-semibold text-slate-800">
              설정
            </h2>

            {!config ? (
              <p className="text-xs text-slate-500">
                설정 정보가 아직 없습니다. 백엔드 /api/config 구현 후 이 화면에서
                수정할 수 있습니다.
              </p>
            ) : (
              <>
                {/* General */}
                <div className="rounded-lg border border-slate-200 p-3 space-y-3">
                  <h3 className="text-xs font-semibold text-slate-700">
                    일반
                  </h3>
                  <div className="space-y-2">
                    <label className="block text-[11px] text-slate-600">
                      타임존 (IANA 이름, 예: Asia/Seoul)
                      <input
                        type="text"
                        value={config.timezone}
                        onChange={(e) =>
                          setConfig({ ...config, timezone: e.target.value })
                        }
                        className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                      />
                    </label>
                    <label className="block text-[11px] text-slate-600">
                      Refresh 스케줄 (cron string, 예: */15 * * * *)
                      <input
                        type="text"
                        value={config.refresh}
                        onChange={(e) =>
                          setConfig({ ...config, refresh: e.target.value })
                        }
                        className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                      />
                    </label>
                    <div className="flex items-center justify-between gap-2">
                      <label className="flex-1 text-[11px] text-slate-600">
                        Horizon (앞으로 표시할 일 수)
                        <input
                          type="number"
                          min={1}
                          max={30}
                          value={config.horizon_days}
                          onChange={(e) =>
                            setConfig({
                              ...config,
                              horizon_days: Number(e.target.value || 7),
                            })
                          }
                          className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                        />
                      </label>
                      <label className="flex flex-col justify-end text-[11px] text-slate-600">
                        주 시작 요일
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
                            월요일
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
                            일요일
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
                      All-day 섹션 표시
                    </label>
                  </div>
                </div>

                {/* ICS URLs */}
                <div className="rounded-lg border border-slate-200 p-3 space-y-3">
                  <div className="flex items-center justify-between">
                    <h3 className="text-xs font-semibold text-slate-700">
                      ICS 구독
                    </h3>
                    <button
                      type="button"
                      onClick={handleAddICS}
                      className="rounded border border-slate-300 bg-slate-50 px-2 py-0.5 text-[11px] text-slate-700 hover:bg-slate-100"
                    >
                      + 추가
                    </button>
                  </div>
                  {config.ics.length === 0 ? (
                    <p className="text-[11px] text-slate-500">
                      아직 등록된 ICS URL 이 없습니다. "+ 추가" 버튼을 눌러
                      캘린더를 등록하세요.
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {config.ics.map((item, idx) => (
                        <div
                          key={idx}
                          className="rounded border border-slate-200 bg-slate-50 px-2 py-2 space-y-1"
                        >
                          <div className="flex items-center gap-2">
                            <label className="flex-1 text-[11px] text-slate-600">
                              ID
                              <input
                                type="text"
                                value={item.id}
                                onChange={(e) =>
                                  handleUpdateICS(idx, "id", e.target.value)
                                }
                                className="mt-0.5 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                              />
                            </label>
                            <button
                              type="button"
                              onClick={() => handleRemoveICS(idx)}
                              className="mt-4 rounded border border-red-200 bg-red-50 px-2 py-0.5 text-[11px] text-red-700 hover:bg-red-100"
                            >
                              삭제
                            </button>
                          </div>
                          <label className="block text-[11px] text-slate-600">
                            URL
                            <input
                              type="text"
                              value={item.url}
                              onChange={(e) =>
                                handleUpdateICS(idx, "url", e.target.value)
                              }
                              className="mt-0.5 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                            />
                          </label>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Highlight keywords + Basic Auth */}
                <div className="rounded-lg border border-slate-200 p-3 space-y-3">
                  <h3 className="text-xs font-semibold text-slate-700">
                    표시 옵션 / 보안
                  </h3>
                  <label className="block text-[11px] text-slate-600">
                    Red highlight keywords (쉼표 또는 줄바꿈으로 구분)
                    <textarea
                      rows={3}
                      value={config.highlight_red_keywords.join(", ")}
                      onChange={(e) => handleKeywordsChange(e.target.value)}
                      className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                    />
                  </label>

                  <div className="border-t border-slate-200 pt-2 space-y-2">
                    <label className="inline-flex items-center gap-2 text-[11px] text-slate-600">
                      <input
                        type="checkbox"
                        checked={config.basic_auth?.enabled ?? false}
                        onChange={(e) => handleBasicAuthEnabled(e.target.checked)}
                        className="h-3 w-3 rounded border-slate-300"
                      />
                      Basic Auth 활성화 (백엔드에서 /health 를 제외한 모든
                      엔드포인트 보호)
                    </label>
                    {config.basic_auth?.enabled && (
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                        <label className="text-[11px] text-slate-600">
                          Username
                          <input
                            type="text"
                            value={config.basic_auth.username}
                            onChange={(e) =>
                              handleBasicAuthField("username", e.target.value)
                            }
                            className="mt-0.5 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                          />
                        </label>
                        <label className="text-[11px] text-slate-600">
                          Password
                          <input
                            type="password"
                            value={config.basic_auth.password}
                            onChange={(e) =>
                              handleBasicAuthField("password", e.target.value)
                            }
                            className="mt-0.5 w-full rounded border border-slate-300 px-2 py-1 text-xs"
                          />
                        </label>
                      </div>
                    )}
                  </div>
                </div>

                <div className="flex justify-end">
                  <button
                    type="button"
                    onClick={handleSave}
                    disabled={saving}
                    className="inline-flex items-center rounded bg-slate-900 px-4 py-1.5 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-60"
                  >
                    {saving ? "저장 중..." : "설정 저장"}
                  </button>
                </div>
              </>
            )}
          </div>

          {/* Right: Preview image */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="text-sm sm:text-base font-semibold text-slate-800">
                Preview (/preview.png)
              </h2>
              <button
                type="button"
                onClick={() => setPreviewReloadKey((k) => k + 1)}
                className="rounded border border-slate-300 bg-slate-50 px-2 py-0.5 text-[11px] text-slate-700 hover:bg-slate-100"
              >
                Preview 새로고침
              </button>
            </div>
            <p className="text-[11px] text-slate-500">
              최신 캡처 결과를 확인하려면 "Preview 새로고침" 버튼을 누르거나
              브라우저 캐시를 무시하고 다시 불러오십시오. 이 이미지는 Go 서버의{" "}
              <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
                /preview.png
              </code>{" "}
              엔드포인트에서 제공됩니다.
            </p>
            <div className="rounded-lg border border-slate-200 bg-slate-50 p-2 flex items-center justify-center">
              <div className="bg-slate-900/90 text-white text-[10px] px-1.5 py-0.5 rounded absolute translate-y-[-120%] left-1/2 -translate-x-1/2 hidden lg:inline-flex">
                1304 × 984 EPD 비율에 가깝게 표시됩니다.
              </div>
              <div className="relative w-full aspect-[1304/984] max-h-[480px] bg-slate-900/5 flex items-center justify-center overflow-hidden">
                <img
                  key={previewReloadKey}
                  src={previewUrl}
                  alt="EPD preview"
                  className="max-w-full max-h-full object-contain border border-slate-300 bg-white"
                  onError={() =>
                    setError(
                      "Preview 이미지를 불러오는 데 실패했습니다. Go 서버에서 /preview.png 가 제공되는지 확인하세요.",
                    )
                  }
                />
              </div>
            </div>
          </div>
        </section>
      </main>
    </div>
  );
}