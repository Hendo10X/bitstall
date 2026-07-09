import type { Metadata } from "next";
import "./globals.css";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import { GeistPixelSquare } from "geist/font/pixel";
import { cn } from "@/lib/utils";

export const metadata: Metadata = {
  title: "Bitstall · mesh market",
  description:
    "Local-first decentralized marketplace · dashboard for the Bitstall daemon",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html
      lang="en"
      className={cn(
        "dark font-sans",
        GeistSans.variable,
        GeistMono.variable,
        GeistPixelSquare.variable
      )}
    >
      <body>{children}</body>
    </html>
  );
}
