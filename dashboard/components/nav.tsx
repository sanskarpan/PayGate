"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import type { DashboardViewer } from "../lib/types";

const links = [
  ["/overview", "Overview"],
  ["/orders", "Orders"],
  ["/compliance", "Compliance"],
  ["/webhooks", "Webhooks"],
  ["/settlements", "Settlements"],
  ["/payouts", "Payouts"],
  ["/reports", "Reports"],
  ["/recon", "Reconciliation"],
  ["/control-plane", "Control Plane"],
  ["/api-keys", "API Keys"],
  ["/risk", "Risk"],
  ["/disputes", "Disputes"],
  ["/gateway", "Gateway"],
  ["/observability", "Observability"],
  ["/audit", "Audit Log"],
  ["/team", "Team"],
] as const;

function isActive(pathname: string, href: string) {
  if (href === "/overview") {
    return pathname === "/overview";
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export default function Nav({
  viewer,
  logoutAction,
}: {
  viewer: DashboardViewer | null;
  logoutAction: string;
}) {
  const pathname = usePathname();

  return (
    <nav className="site-nav">
      <div className="nav-brand-block">
        <Link className="brand" href={viewer ? "/overview" : "/"}>
          <span className="brand-mark">PG</span>
          <span>
            PayGate
            <span className="brand-subtitle">Control</span>
          </span>
        </Link>
        {viewer ? <span className="brand-context">Merchant operations console</span> : null}
      </div>

      <div className="nav-right">
        <div className="nav-links">
          {viewer
            ? links.map(([href, label]) => (
                <Link
                  key={href}
                  href={href}
                  className={`nav-link${isActive(pathname, href) ? " active" : ""}`}
                >
                  {label}
                </Link>
              ))
            : null}
        </div>

        <div className="nav-user">
          {viewer ? (
            <>
              <div className="identity-card">
                <div className="identity-email">
                  {viewer.email.length > 30 ? viewer.email.slice(0, 28) + "…" : viewer.email}
                </div>
                <div className="identity-meta">
                  <span className="badge-neutral">{viewer.role}</span>
                  <span className="badge-info">{viewer.auth_type}</span>
                  <span>{viewer.merchant_id.slice(-10)}</span>
                </div>
              </div>
              <form action={logoutAction} method="POST">
                <button className="ghost-button" type="submit">
                  Sign Out
                </button>
              </form>
            </>
          ) : (
            <Link className="ghost-button" href="/">
              Login
            </Link>
          )}
        </div>
      </div>
    </nav>
  );
}
