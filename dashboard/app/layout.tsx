import type { Metadata } from "next";
import { IBM_Plex_Mono, Instrument_Sans, Syne } from "next/font/google";
import "./globals.css";
import Nav from "../components/nav";
import { getAppBaseUrl, getApiBaseUrl, getViewerOptional } from "../lib/api";

const displayFont = Syne({
  subsets: ["latin"],
  weight: ["500", "600", "700", "800"],
  variable: "--font-display",
});

const bodyFont = Instrument_Sans({
  subsets: ["latin"],
  variable: "--font-body",
});

const monoFont = IBM_Plex_Mono({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  variable: "--font-mono",
});

export const metadata: Metadata = {
  title: "PayGate Command Fabric",
  description: "Operational command surface for money movement, control policy, and system visibility.",
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  let viewer = null;
  try {
    viewer = await getViewerOptional();
  } catch {
    viewer = null;
  }
  const logoutAction = `${getApiBaseUrl()}/v1/dashboard/logout?redirect_to=${encodeURIComponent(
    `${getAppBaseUrl()}/`,
  )}`;
  return (
    <html lang="en" className={`${displayFont.variable} ${bodyFont.variable} ${monoFont.variable}`}>
      <body>
        <div className={`page-shell${viewer ? " viewer-shell" : ""}`}>
          <div className="page-atmosphere" />
          <Nav logoutAction={logoutAction} viewer={viewer} />
          <div className="page-main-wrap">
            <main className="page-main">{children}</main>
          </div>
        </div>
      </body>
    </html>
  );
}
