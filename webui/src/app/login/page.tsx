"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import nanumGothic from "../fonts/nanum";

export default function LoginPage() {
  const [next, setNext] = useState("/");
  const [failed, setFailed] = useState(false);
  const [loggedOut, setLoggedOut] = useState(false);

  useEffect(() => {
    const query = new URLSearchParams(window.location.search);
    setNext(query.get("next") || "/");
    setFailed(query.get("error") === "1");
    setLoggedOut(query.get("logged_out") === "1");
  }, []);

  return (
    <div className={`${nanumGothic.className} min-h-screen bg-[#eef2f1] px-4 py-8 text-slate-900`}>
      <main className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-lg items-center justify-center">
        <section className="w-full rounded-[1.5rem] border border-slate-200 bg-[#fbfcfb] p-7 shadow-[0_18px_50px_rgba(15,23,42,0.08)] sm:p-10">
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-teal-700">EPD CONTROL CENTER</p>
          <h1 className="mt-3 text-2xl font-bold tracking-tight text-slate-950">로그인</h1>
          <p className="mt-2 text-sm leading-6 text-slate-600">설정과 캘린더에 접근하려면 인증이 필요합니다.</p>

          {failed && (
            <div className="mt-5 rounded-xl border border-red-200 bg-red-50 px-3 py-2.5 text-xs font-semibold text-red-700">
              사용자 이름 또는 비밀번호가 올바르지 않습니다.
            </div>
          )}
          {loggedOut && (
            <div className="mt-5 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-xs font-semibold text-emerald-700">
              로그아웃되었습니다.
            </div>
          )}

          <form method="post" action="/login" className="mt-6 space-y-4">
            <input type="hidden" name="next" value={next} />
            <label className="block text-xs font-semibold text-slate-600">
              사용자 이름
              <input
                name="username"
                type="text"
                autoComplete="username"
                required
                className="mt-1.5 h-10 w-full rounded-xl border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
              />
            </label>
            <label className="block text-xs font-semibold text-slate-600">
              비밀번호
              <input
                name="password"
                type="password"
                autoComplete="current-password"
                required
                className="mt-1.5 h-10 w-full rounded-xl border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
              />
            </label>
            <button type="submit" className="w-full rounded-xl bg-teal-700 px-4 py-2.5 text-sm font-bold text-white transition hover:bg-teal-800">
              로그인
            </button>
          </form>

          <p className="mt-5 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-5 text-amber-800">
            현재 HTTP로 접속 중이면 로그인 정보가 네트워크에 평문으로 전달될 수 있습니다. 신뢰할 수 있는 내부망에서 사용하거나 HTTPS/VPN을 함께 사용하세요.
          </p>
          <Link href="/" className="mt-5 inline-block text-xs font-semibold text-slate-500 transition hover:text-slate-900">
            메인으로 돌아가기
          </Link>
        </section>
      </main>
    </div>
  );
}
