"use client";

import { useEffect, useState } from "react";
import nanumGothic from "./fonts/nanum";
import { I18nProvider, useI18n } from "@/app/core/i18n";

type HealthStatus = "idle" | "ok" | "error";

function HomeContent() {
  const { t } = useI18n();
  const [healthStatus, setHealthStatus] = useState<HealthStatus>("idle");
  const [healthMessage, setHealthMessage] = useState<string>("");
  const [checking, setChecking] = useState(false);

  const checkHealth = async () => {
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
    } catch (e: any) {
      setHealthStatus("error");
      setHealthMessage(e?.message ?? t("home.health.request_failed"));
    } finally {
      setChecking(false);
    }
  };

  useEffect(() => {
    void checkHealth();
  }, []);

  return (
    <div
      className={`${nanumGothic.className} min-h-screen bg-slate-100 text-slate-900 flex items-center justify-center px-4`}
    >
      <main className="w-full max-w-3xl rounded-xl bg-white shadow-sm px-6 py-7 sm:px-8 sm:py-8 border border-slate-200">
        <header className="mb-6 border-b border-slate-200 pb-4 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl sm:text-3xl font-bold tracking-tight">
              {t("home.title")}
            </h1>
            <p className="mt-1 text-[11px] sm:text-xs text-slate-500">
              {t("home.subtitle")}
            </p>
          </div>
          <div className="flex items-center gap-2 text-[11px] sm:text-xs">
            <span className="text-slate-500">{t("home.quick_links")}</span>
            <div className="inline-flex rounded-full border border-slate-300 bg-slate-100 p-0.5">
              <a
                href="/calendar"
                className="px-3 py-1 rounded-full text-slate-700 hover:bg-slate-200"
              >
                {t("common.goto.calendar")}
              </a>
              <a
                href="/config"
                className="px-3 py-1 rounded-full bg-slate-900 text-white hover:bg-slate-800"
              >
                {t("common.goto.config")}
              </a>
            </div>
          </div>
        </header>

        <section className="grid grid-cols-1 md:grid-cols-2 gap-5 text-[11px] sm:text-xs text-slate-700">
          <div className="space-y-2">
            <h2 className="text-sm sm:text-base font-semibold text-slate-900">
              {t("home.section.howto")}
            </h2>
            <ol className="list-decimal list-inside space-y-1">
              <li>{t("home.howto.step1")}</li>
              <li>{t("home.howto.step2")}</li>
              <li>{t("home.howto.step3")}</li>
              <li>{t("home.howto.step4")}</li>
            </ol>
          </div>

          <div className="space-y-2">
            <h2 className="text-sm sm:text-base font-semibold text-slate-900">
              {t("home.section.auth")}
            </h2>
            <p>{t("home.auth.description")}</p>
            <p className="mt-1 text-slate-500">
              {t("home.auth.description2")}
            </p>
          </div>
        </section>

        <section className="mt-6 border-t border-slate-200 pt-4 text-[10px] sm:text-[11px] text-slate-500 space-y-2">
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
