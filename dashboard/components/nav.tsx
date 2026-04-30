import Link from "next/link";

import { getApiBaseUrl, getAppBaseUrl } from "../lib/api";
import type { DashboardViewer } from "../lib/types";

const links = [
  ["/orders", "Orders"],
  ["/api-keys", "API Keys"],
];

export default function Nav({ viewer }: { viewer: DashboardViewer | null }) {
  const logoutAction = `${getApiBaseUrl()}/v1/dashboard/logout?redirect_to=${encodeURIComponent(
    `${getAppBaseUrl()}/`,
  )}`;
  return (
    <nav className="site-nav">
      <Link className="brand" href={viewer ? "/orders" : "/"}>
        PayGate
      </Link>
      <div className="nav-links">
        {viewer
          ? links.map(([href, label]) => (
              <Link key={href} href={href}>
                {label}
              </Link>
            ))
          : null}
      </div>
      <div className="nav-user">
        {viewer ? (
          <>
            <div>
              <div className="row-title">{viewer.email}</div>
              <div className="row-meta">
                <span>{viewer.role}</span>
                <span>{viewer.merchant_id}</span>
              </div>
            </div>
            <form action={logoutAction} method="POST">
              <button className="ghost-button" type="submit">
                Sign Out
              </button>
            </form>
          </>
        ) : (
          <Link href="/">Login</Link>
        )}
      </div>
    </nav>
  );
}
