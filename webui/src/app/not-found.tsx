import Link from "next/link";
import nanumGothic from "./fonts/nanum";

export default function NotFound() {
  return (
    <div className={`${nanumGothic.className} min-h-screen bg-[#eef2f1] px-4 py-8 text-slate-900`}>
      <main className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-lg items-center justify-center">
        <section className="w-full rounded-[1.5rem] border border-slate-200 bg-[#fbfcfb] p-7 text-center shadow-[0_18px_50px_rgba(15,23,42,0.08)] sm:p-10">
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-slate-400">PAGE STATUS</p>
          <div className="mx-auto mt-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-slate-100 text-2xl font-extrabold text-slate-600">
            404
          </div>
          <h1 className="mt-6 text-2xl font-bold tracking-tight text-slate-950">페이지를 찾을 수 없습니다</h1>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            요청하신 주소가 존재하지 않거나 이동되었을 수 있습니다.
            <br />
            아래 버튼을 눌러 서비스 첫 화면으로 이동하세요.
          </p>
          <div className="mt-7 flex justify-center">
          <Link
            href="/"
            className="inline-flex items-center justify-center rounded-xl bg-teal-700 px-4 py-2.5 text-sm font-bold text-white transition hover:bg-teal-800"
          >
            메인으로
          </Link>
          </div>
        </section>
      </main>
    </div>
  );
}
