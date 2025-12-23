"use client";

import { useState } from "react";
import nanumGothic from "./fonts/nanum";

export default function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const handleSubmit: React.FormEventHandler<HTMLFormElement> = (e) => {
    e.preventDefault();
    // 실제 인증은 백엔드 Basic Auth(HTTP 401)로 처리되므로,
    // 여기서는 설정 페이지로 이동만 수행한다.
    // /config 또는 /api/* 접근 시 브라우저가 Basic Auth 자격 증명을 요구하게 된다.
    window.location.href = "/config";
  };

  return (
    <div
      className={`${nanumGothic.className} min-h-screen bg-slate-100 text-slate-900 flex items-center justify-center px-4`}
    >
      <main className="w-full max-w-md rounded-xl bg-white shadow-sm px-6 py-7 sm:px-8 sm:py-8">
        <header className="mb-6 text-center">
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight">
            epdcal 로그인
          </h1>
          <p className="mt-2 text-[11px] sm:text-xs text-slate-500">
            ICS 캘린더 / 디스플레이 설정을 변경하려면 로그인 후 설정 페이지로
            이동하세요.
          </p>
        </header>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <label className="block text-[11px] text-slate-600">
              사용자 이름
              <input
                type="text"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="mt-1 w-full rounded border border-slate-300 px-2 py-1.5 text-xs focus:outline-none focus:ring-1 focus:ring-slate-500"
                placeholder="예: admin"
              />
            </label>
            <label className="block text-[11px] text-slate-600">
              비밀번호
              <input
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="mt-1 w-full rounded border border-slate-300 px-2 py-1.5 text-xs focus:outline-none focus:ring-1 focus:ring-slate-500"
                placeholder="설정된 Basic Auth 비밀번호"
              />
            </label>
          </div>

          <p className="text-[10px] sm:text-[11px] text-slate-500">
            실제 인증은 서버의 Basic Auth 설정에 의해 처리되며, 이 화면에서
            "로그인"을 누르면 설정 페이지(
            <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
              /config
            </code>
            )로 이동합니다. 보호된 리소스에 접근할 때 브라우저가 자격 증명을
            요청합니다.
          </p>

          <button
            type="submit"
            className="mt-2 w-full rounded bg-slate-900 px-4 py-2 text-xs sm:text-sm font-semibold text-white hover:bg-slate-800"
          >
            로그인 후 설정 페이지로 이동
          </button>
        </form>

        <div className="mt-5 border-t border-slate-200 pt-4 text-[11px] sm:text-xs text-slate-500 space-y-1">
          <p>
            캘린더 미리보기는 설정 페이지의 Preview 섹션에서 확인할 수 있습니다.
          </p>
          <p>
            디스플레이용 캘린더 렌더링은 다음 경로에서 확인/캡처됩니다:{" "}
            <code className="rounded bg-slate-100 px-1 py-0.5 text-[10px]">
              /calendar
            </code>
          </p>
        </div>
      </main>
    </div>
  );
}
