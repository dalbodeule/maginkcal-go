"use client";

import { useEffect, useState } from "react";
import nanumGothic from "./fonts/nanum";

type HealthStatus = "idle" | "ok" | "error";

export default function HomePage() {
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
      setHealthMessage(e?.message ?? "요청 실패");
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
              epdcal
            </h1>
            <p className="mt-1 text-[11px] sm:text-xs text-slate-500">
              Raspberry Pi + Waveshare 12.48" e-paper 에 ICS 캘린더를
              표시하는 작은 서비스입니다.
            </p>
          </div>
          <div className="flex items-center gap-2 text-[11px] sm:text-xs">
            <span className="text-slate-500">바로가기</span>
            <div className="inline-flex rounded-full border border-slate-300 bg-slate-100 p-0.5">
              <a
                href="/calendar"
                className="px-3 py-1 rounded-full text-slate-700 hover:bg-slate-200"
              >
                캘린더 보기
              </a>
              <a
                href="/config"
                className="px-3 py-1 rounded-full bg-slate-900 text-white hover:bg-slate-800"
              >
                설정 / Preview
              </a>
            </div>
          </div>
        </header>

        <section className="grid grid-cols-1 md:grid-cols-2 gap-5 text-[11px] sm:text-xs text-slate-700">
          <div className="space-y-2">
            <h2 className="text-sm sm:text-base font-semibold text-slate-900">
              사용 방법
            </h2>
            <ol className="list-decimal list-inside space-y-1">
              <li>
                <a
                  href="/config"
                  className="font-semibold text-slate-900 underline underline-offset-2 decoration-slate-300"
                >
                  설정 / Preview
                </a>{" "}
                페이지에서 ICS URL, 타임존, 스케줄 등을 설정합니다.
              </li>
              <li>
                설정이 저장되면 백엔드가 주기적으로 ICS 를 fetch/parse/expand
                하고, /calendar 를 렌더링해 EPD 로 출력합니다.
              </li>
              <li>
                <a
                  href="/calendar"
                  className="font-semibold text-slate-900 underline underline-offset-2 decoration-slate-300"
                >
                  /calendar
                </a>{" "}
                는 실제 디스플레이용 화면이며, 캡처 파이프라인도 이 경로를
                사용합니다.
              </li>
              <li>
                최신 캡처 이미지는{" "}
                <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
                  /preview.png
                </code>{" "}
                에서 확인할 수 있습니다.
              </li>
            </ol>
          </div>

          <div className="space-y-2">
            <h2 className="text-sm sm:text-base font-semibold text-slate-900">
              인증 / 보안
            </h2>
            <p>
              HTTP Basic Auth 가 설정된 경우, 이 메인 페이지와 설정/캘린더
              페이지에 처음 접근할 때 브라우저가 사용자 이름/비밀번호를
              요청합니다. 한 번 로그인하면 같은 브라우저 세션 동안은 자동으로
              인증이 유지됩니다.
            </p>
            <p className="mt-1 text-slate-500">
              Basic Auth 는{" "}
              <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
                config.yaml
              </code>{" "}
              또는 백엔드 설정 API 를 통해{" "}
              <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
                basic_auth.username / basic_auth.password
              </code>{" "}
              로 구성합니다.
            </p>
          </div>
        </section>

        <section className="mt-6 border-t border-slate-200 pt-4 text-[10px] sm:text-[11px] text-slate-500 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span>백엔드 상태 확인:</span>
            <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
              /health
            </code>
            <span className="inline-flex items-center gap-1">
              {healthStatus === "idle" && (
                <span className="text-slate-400">확인 대기중...</span>
              )}
              {healthStatus === "ok" && (
                <span className="inline-flex items-center gap-1 text-emerald-700">
                  <span className="h-2 w-2 rounded-full bg-emerald-500" />
                  <span>{healthMessage || "OK"}</span>
                </span>
              )}
              {healthStatus === "error" && (
                <span className="inline-flex items-center gap-1 text-red-700">
                  <span className="h-2 w-2 rounded-full bg-red-500" />
                  <span>{healthMessage || "에러"}</span>
                </span>
              )}
            </span>
            <button
              type="button"
              onClick={() => void checkHealth()}
              disabled={checking}
              className="ml-auto inline-flex items-center rounded border border-slate-300 bg-slate-50 px-2 py-0.5 text-[10px] text-slate-700 hover:bg-slate-100 disabled:opacity-60"
            >
              {checking ? "확인 중..." : "다시 확인"}
            </button>
          </div>
          <p>
            이 페이지는 단순 안내용 메인 화면이며, 실제 캘린더 렌더링/설정은
            상단의 "캘린더 보기" 및 "설정 / Preview" 링크를 통해 진행합니다.
          </p>
        </section>
      </main>
    </div>
  );
}
