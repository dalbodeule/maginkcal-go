import Link from "next/link";
import nanumGothic from "./fonts/nanum";

export default function NotFound() {
  return (
    <div
      className={`${nanumGothic.className} min-h-screen flex flex-col items-center justify-center bg-slate-100 text-slate-900 px-4`}
    >
      <div className="max-w-md w-full text-center">
        <p className="text-sm font-medium text-slate-500 mb-2">404 NOT FOUND</p>
        <h1 className="text-3xl md:text-4xl font-bold mb-4">
          페이지를 찾을 수 없습니다
        </h1>
        <p className="text-sm md:text-base text-slate-600 mb-8 leading-relaxed">
          요청하신 페이지가 존재하지 않거나 이동되었을 수 있습니다.
          <br />
          아래 버튼을 눌러 메인 화면으로 이동하세요.
        </p>
        <div className="flex flex-col sm:flex-row gap-3 justify-center items-center">
          <Link
            href="/"
            className="inline-flex items-center justify-center rounded-md border border-slate-300 px-4 py-2.5 text-sm font-semibold text-slate-700 bg-white hover:bg-slate-50 transition-colors"
          >
            메인으로
          </Link>
        </div>
      </div>
    </div>
  );
}