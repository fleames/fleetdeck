import type { Metadata } from "next";
import { IBM_Plex_Sans, IBM_Plex_Mono } from "next/font/google";
import { AppShell } from "@/components/app-shell";
import "./globals.css";

const sans = IBM_Plex_Sans({
  variable: "--font-sans-family",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const mono = IBM_Plex_Mono({
  variable: "--font-mono-family",
  subsets: ["latin"],
  weight: ["400", "500"],
});

export const metadata: Metadata = {
  title: "FleetDeck",
  description: "Local-first server and Docker monitoring command center",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" data-theme="dark" className={`${sans.variable} ${mono.variable} h-full`}>
      <body className="min-h-full antialiased">
        <AppShell>{children}</AppShell>
      </body>
    </html>
  );
}
