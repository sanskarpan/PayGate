"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import type { DashboardViewer } from "../lib/types";
import { BrandEmblem, RouteIcon } from "./system-icons";

const linkGroups = [
  {
    label: "Command",
    links: [
      ["/overview", "Overview", "overview"],
      ["/control-plane", "Control Plane", "control-plane"],
      ["/observability", "Observability", "observability"],
      ["/recon", "Reconciliation", "recon"],
    ],
  },
  {
    label: "Commercial",
    links: [
      ["/orders", "Orders", "orders"],
      ["/settlements", "Settlements", "settlements"],
      ["/payouts", "Payouts", "payouts"],
      ["/reports", "Reports", "reports"],
      ["/mandates", "Mandates", "mandates"],
    ],
  },
  {
    label: "Governance",
    links: [
      ["/compliance", "Compliance", "compliance"],
      ["/risk", "Risk", "risk"],
      ["/disputes", "Disputes", "disputes"],
      ["/webhooks", "Webhooks", "webhooks"],
      ["/gateway", "Gateway", "gateway"],
      ["/api-keys", "API Keys", "api-keys"],
      ["/audit", "Audit Log", "audit"],
      ["/team", "Team", "team"],
    ],
  },
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
    <nav className={`site-nav${viewer ? " is-rail" : " is-public"}`}>
      <div className="nav-brand-block">
        <Link className="brand" href={viewer ? "/overview" : "/"}>
          <span className="brand-mark" aria-hidden="true">
            <BrandEmblem size={28} />
          </span>
          <span>
            PayGate
            <span className="brand-subtitle">Merchant Operating System</span>
          </span>
        </Link>
        <span className="brand-context">
          {viewer ? "Live commercial control picture" : "Premium operator access surface"}
        </span>
      </div>

      {viewer ? (
        <>
          <div className="nav-status-band">
            <div className="status-cell">
              <span className="status-label">Access Profile</span>
              <span className="status-value">
                {viewer.role} / {viewer.auth_type}
              </span>
            </div>
            <div className="status-cell">
              <span className="status-label">Tenant</span>
              <span className="status-value">{viewer.merchant_id}</span>
            </div>
            <div className="status-cell">
              <span className="status-label">Domain</span>
              <span className="status-value">Money / Risk / Runtime / Treasury</span>
            </div>
          </div>

          <div className="nav-groups">
            {linkGroups.map((group) => (
              <div className="nav-group" key={group.label}>
                <div className="nav-group-label">{group.label}</div>
                <div className="nav-links">
                  {group.links.map(([href, label, icon]) => (
                    <Link
                      key={href}
                      href={href}
                      className={`nav-link${isActive(pathname, href) ? " active" : ""}`}
                    >
                      <RouteIcon name={icon} size={16} />
                      {label}
                    </Link>
                  ))}
                </div>
              </div>
            ))}
          </div>

          <div className="nav-user">
            <div className="identity-card">
              <div className="identity-email">
                {viewer.email.length > 30 ? viewer.email.slice(0, 28) + "…" : viewer.email}
              </div>
              <div className="identity-meta">
                <span className="badge-neutral">{viewer.role}</span>
                <span className="badge-info">{viewer.auth_type}</span>
                <span className="mono">{viewer.merchant_id.slice(-12)}</span>
              </div>
            </div>
            <form action={logoutAction} method="POST">
              <button className="ghost-button" type="submit">
                Sign Out
              </button>
            </form>
          </div>
        </>
      ) : (
        <div className="nav-public-actions nav-public-pills">
          <a className="ghost-button nav-public-pill" href="#operator-lanes">
            System Tour
          </a>
          <a className="ghost-button nav-public-pill" href="#operator-sign-in">
            Talk to PayGate
          </a>
          <Link className="primary-button nav-public-pill" href="#operator-sign-in">
            Dashboard Access
          </Link>
        </div>
      )}
    </nav>
  );
}
