import type { Metadata } from "next";
import Script from "next/script";
import "./globals.css";
import React from "react";
import nanumGothic from "./fonts/nanum";
import "@/app/core/config/fontawesome";

export const metadata: Metadata = {
  title: "epdcal",
  description: "E-paper calendar",
  icons: "/favicon.png",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className={`${nanumGothic.variable} antialiased`}>
        <Script src="/app-config.js" strategy="beforeInteractive" />
        {children}
      </body>
    </html>
  );
}
