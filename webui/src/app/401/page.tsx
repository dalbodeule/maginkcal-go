import Link from "next/link";
import nanumGothic from "../fonts/nanum";

export default function UnauthorizedPage() {
  return (
    <div className={`${nanumGothic.className} min-h-screen bg-[#eef2f1] px-4 py-8 text-slate-900`}>
      <main className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-lg items-center justify-center">
        <section className="w-full rounded-[1.5rem] border border-slate-200 bg-[#fbfcfb] p-7 text-center shadow-[0_18px_50px_rgba(15,23,42,0.08)] sm:p-10">
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-amber-700">ACCESS CHECK</p>
          <div className="mx-auto mt-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-amber-100 text-2xl font-extrabold text-amber-700">
            401
          </div>
          <h1 className="mt-6 text-2xl font-bold tracking-tight text-slate-950">인증이 필요합니다</h1>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            이 화면을 보려면 설정된 사용자 이름과 비밀번호로 인증해야 합니다.
            <br />
            인증 창이 다시 표시되지 않으면 페이지를 새로고침하세요.
          </p>
          <div className="mt-7 flex flex-col justify-center gap-2 sm:flex-row">
            <Link
              href="/"
              className="rounded-xl bg-teal-700 px-4 py-2.5 text-sm font-bold text-white transition hover:bg-teal-800"
            >
              다시 인증하기
            </Link>
            <Link
              href="/"
              className="rounded-xl border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-100"
            >
              메인으로
            </Link>
          </div>
        </section>
      </main>
    </div>
  );
}
