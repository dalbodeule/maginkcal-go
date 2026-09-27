"use client";

import { useCallback, useEffect, useState } from "react";
import nanumGothic from "./fonts/nanum";
import { I18nProvider, useI18n } from "@/app/core/i18n";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faBatteryEmpty, faBatteryFull, faBatteryHalf, faBatteryQuarter, faBatteryThreeQuarters } from "@fortawesome/free-solid-svg-icons";

type HealthStatus = "idle" | "ok" | "error";

function HomeContent() {
  const { t } = useI18n();
  const [healthStatus, setHealthStatus] = useState<HealthStatus>("idle");
  const [healthMessage, setHealthMessage] = useState<string>("");
  const [checking, setChecking] = useState(false);
  const [batteryPercent, setBatteryPercent] = useState<number | null>(null);
  const batteryIcon = batteryPercent == null || batteryPercent < 40
    ? batteryPercent != null && batteryPercent < 20 ? faBatteryEmpty : faBatteryQuarter
    : batteryPercent < 60 ? faBatteryHalf
    : batteryPercent < 80 ? faBatteryThreeQuarters : faBatteryFull;

  useEffect(() => {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 5000);
    void fetch("/api/battery", { signal: controller.signal, cache: "no-store" })
      .then((res) => res.json())
      .then((status: { available?: boolean; percent?: number | null }) => {
        if (status.available && typeof status.percent === "number" &&
            Number.isInteger(status.percent) && status.percent >= 0 && status.percent <= 100) {
          setBatteryPercent(status.percent);
        }
      })
      .catch(() => {})
      .finally(() => window.clearTimeout(timeout));
    return () => { controller.abort(); window.clearTimeout(timeout); };
  }, []);

  const checkHealth = useCallback(async () => {
    try {
      setChecking(true);
      setHealthMessage("");
      const start = performance.now();
      const res = await fetch("/health", { cache: "no-store" });
      const elapsed = Math.round(performance.now() - start);
      if (!res.ok) {
        setHealthStatus("error");
        setHealthMessage(`HTTP ${res.status} (${elapsed}ms)`);
        return;
      }
      const text = (await res.text()).trim();
      setHealthStatus("ok");
      setHealthMessage(`${text || "OK"} (${elapsed}ms)`);
    } catch (e: unknown) {
      setHealthStatus("error");
      setHealthMessage(e instanceof Error ? e.message : t("home.health.request_failed"));
    } finally {
      setChecking(false);
    }
  }, [t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void checkHealth();
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [checkHealth]);

  return (
    <div
      className={`${nanumGothic.className} min-h-screen bg-[#eef2f1] text-slate-900 flex items-center justify-center px-3 py-6 sm:px-6`}
    >
      <main className="w-full max-w-4xl rounded-[1.5rem] border border-slate-200/80 bg-[#fbfcfb] px-5 py-6 shadow-[0_18px_50px_rgba(15,23,42,0.08)] sm:px-9 sm:py-9">
        <header className="mb-8 flex flex-col gap-4 border-b border-slate-200 pb-6 [word-break:keep-all]">
          <div>
            <p className="mb-2 text-[10px] font-bold uppercase tracking-[0.2em] text-teal-700">EPD CONTROL CENTER</p>
            <div className="flex items-center justify-between gap-3">
              <h1 className="text-3xl font-bold tracking-tight text-slate-950 sm:text-4xl">{t("home.title")}</h1>
              <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-slate-200 bg-white px-2.5 py-1 text-xs font-bold text-slate-700 shadow-sm" aria-label={`Battery ${batteryPercent == null ? "unknown" : `${batteryPercent}%`}`}>
                <FontAwesomeIcon icon={batteryIcon} />
                {batteryPercent == null ? "??%" : `${batteryPercent}%`}
              </span>
            </div>
            <p className="mt-1 text-[11px] sm:text-xs text-slate-500">
              {t("home.subtitle")}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-3 text-xs">
            <span className="whitespace-nowrap text-[11px] font-semibold text-slate-400">{t("home.quick_links")}</span>
            <div className="inline-flex shrink-0 rounded-xl border border-slate-200 bg-white p-1 shadow-sm">
              <a
                href="/calendar"
                className="rounded-lg px-3 py-1.5 whitespace-nowrap text-slate-600 transition hover:bg-slate-100"
              >
                {t("common.goto.calendar")}
              </a>
              <a
                href="/config"
                className="rounded-lg bg-slate-900 px-3 py-1.5 whitespace-nowrap text-white shadow-sm transition hover:bg-slate-800"
              >
                {t("common.goto.config")}
              </a>
            </div>
          </div>
        </header>

        <section className="grid grid-cols-1 gap-4 text-xs text-slate-700 md:grid-cols-2">
          <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm space-y-3">
            <h2 className="text-base font-bold text-slate-950">
              {t("home.section.howto")}
            </h2>
            <ol className="list-decimal list-inside space-y-1">
              <li>{t("home.howto.step1")}</li>
              <li>{t("home.howto.step2")}</li>
              <li>{t("home.howto.step3")}</li>
              <li>{t("home.howto.step4")}</li>
            </ol>
          </div>

          <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm space-y-3">
            <h2 className="text-base font-bold text-slate-950">
              {t("home.section.auth")}
            </h2>
            <p>{t("home.auth.description")}</p>
            <p className="mt-1 text-slate-500">
              {t("home.auth.description2")}
            </p>
          </div>
        </section>

        <section className="mt-5 rounded-2xl border border-slate-200 bg-slate-50/70 p-4 text-[11px] text-slate-500 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span>{t("common.health.check")}:</span>
            <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
              /health
            </code>
            <span className="inline-flex items-center gap-1">
              {healthStatus === "idle" && (
                <span className="text-slate-400">
                  {t("common.health.waiting")}
                </span>
              )}
              {healthStatus === "ok" && (
                <span className="inline-flex items-center gap-1 text-emerald-700">
                  <span className="h-2 w-2 rounded-full bg-emerald-500" />
                  <span>{healthMessage || t("common.health.ok")}</span>
                </span>
              )}
              {healthStatus === "error" && (
                <span className="inline-flex items-center gap-1 text-red-700">
                  <span className="h-2 w-2 rounded-full bg-red-500" />
                  <span>{healthMessage || t("common.health.error")}</span>
                </span>
              )}
            </span>
            <button
              type="button"
              onClick={() => void checkHealth()}
              disabled={checking}
              className="ml-auto inline-flex items-center rounded border border-slate-300 bg-slate-50 px-2 py-0.5 text-[10px] text-slate-700 hover:bg-slate-100 disabled:opacity-60"
            >
              {checking
                ? t("common.health.checking")
                : t("common.health.again")}
            </button>
          </div>
          <p>{t("home.footer.description")}</p>
        </section>
      </main>
    </div>
  );
}

export default function HomePage() {
  return (
    <I18nProvider>
      <HomeContent />
    </I18nProvider>
  );
}
