import type { Metadata } from "next";
import "./globals.css";
import Nav from "../components/nav";
import { getAppBaseUrl, getApiBaseUrl, getViewerOptional } from "../lib/api";

export const metadata: Metadata = {
  title: "PayGate Control",
  description: "Operations, money movement, risk, and reliability control center for PayGate.",
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const viewer = await getViewerOptional();
  const logoutAction = `${getApiBaseUrl()}/v1/dashboard/logout?redirect_to=${encodeURIComponent(
    `${getAppBaseUrl()}/`,
  )}`;
  return (
    <html lang="en">
      <body>
        <div className="page-shell">
          <Nav logoutAction={logoutAction} viewer={viewer} />
          <main className="page-main">{children}</main>
        </div>
      </body>
    </html>
  );
}
