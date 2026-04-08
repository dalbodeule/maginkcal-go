import { Suspense } from "react";
import CalendarPageClient from "./calendar-page-client";

export default function CalendarPage() {
  return (
    <Suspense fallback={null}>
      <CalendarPageClient />
    </Suspense>
  );
}
