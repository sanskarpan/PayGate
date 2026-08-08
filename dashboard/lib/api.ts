import { cookies } from "next/headers";
import { notFound, redirect } from "next/navigation";

import type {
  APIKeyItem,
  AuditLogItem,
  BeneficiaryItem,
  CapabilityItem,
  DeliveryAttemptItem,
  DashboardViewer,
  DisputeItem,
  EventSchemaItem,
  ExportJobItem,
  GatewayScenario,
  InvitationItem,
  LedgerHoldItem,
  OnboardingApplicationItem,
  OnboardingDocumentItem,
  OnboardingPartyItem,
  OrderItem,
  PaymentItem,
  PayoutItem,
  ReconMismatch,
  ReportCatalogItem,
  ReserveEscalationItem,
  ReservePolicyItem,
  RefundItem,
  RiskEventItem,
  SagaItem,
  ScreeningCaseItem,
  SettlementItem,
  SettlementLineItem,
  TaxProfileItem,
  UPIMandateEventItem,
  UPIMandateItem,
  UPIIntentItem,
  WebhookItem,
  PrometheusSeriesPoint,
} from "./types";

type CollectionResponse<T> = {
  items: T[];
  count: number;
  has_more?: boolean;
  next_cursor?: string;
};

// Browser-facing API origin. Used for form actions, redirects and anything
// handed to a client component, so it must be reachable from the user's browser.
export function getApiBaseUrl() {
  return process.env.API_BASE_URL || "http://localhost:8090";
}

// API origin for calls made by the dashboard server itself. When the dashboard
// runs in a container or pod, the browser-facing URL is often unreachable from
// inside it (localhost is the container, and hairpinning through the ingress is
// wasteful), so allow an internal address. Defaults to the public one, which is
// correct for a single-URL deployment.
export function getInternalApiBaseUrl() {
  return process.env.API_INTERNAL_BASE_URL || getApiBaseUrl();
}

export function getAppBaseUrl() {
  return process.env.APP_BASE_URL || "http://localhost:3001";
}

function cookieHeader() {
  return cookies()
    .getAll()
    .map(({ name, value }) => `${name}=${value}`)
    .join("; ");
}

async function apiFetch(path: string, init?: RequestInit) {
  const headers = new Headers(init?.headers || {});
  const cookie = cookieHeader();
  if (cookie) {
    headers.set("cookie", cookie);
  }
  if (!headers.has("content-type") && init?.body) {
    headers.set("content-type", "application/json");
  }
  return fetch(`${getInternalApiBaseUrl()}${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });
}

export async function getViewerOptional(): Promise<DashboardViewer | null> {
  const response = await apiFetch("/v1/dashboard/me");
  if (response.status === 401) {
    return null;
  }
  if (!response.ok) {
    throw new Error(`dashboard viewer fetch failed: ${response.status}`);
  }
  return (await response.json()) as DashboardViewer;
}

export async function requireViewer() {
  const viewer = await getViewerOptional();
  if (!viewer) {
    redirect("/");
  }
  return viewer;
}

export async function getOrders(cursor?: string) {
  await requireViewer();
  const qs = cursor ? `?count=25&cursor=${encodeURIComponent(cursor)}` : "?count=25";
  const response = await apiFetch(`/v1/orders${qs}`);
  if (!response.ok) {
    throw new Error(`orders fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<OrderItem>;
}

export async function getOrder(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/orders/${id}`);
  if (response.status === 404) {
    notFound();
  }
  if (!response.ok) {
    throw new Error(`order fetch failed: ${response.status}`);
  }
  return (await response.json()) as OrderItem;
}

export async function getPayments(orderID?: string) {
  await requireViewer();
  const suffix = orderID ? `?order_id=${encodeURIComponent(orderID)}` : "";
  const response = await apiFetch(`/v1/payments${suffix}`);
  if (!response.ok) {
    throw new Error(`payments fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<PaymentItem>;
}

export async function getPayment(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/payments/${id}`);
  if (response.status === 404) {
    notFound();
  }
  if (!response.ok) {
    throw new Error(`payment fetch failed: ${response.status}`);
  }
  return (await response.json()) as PaymentItem;
}

export async function getUPIIntent(paymentID: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/payments/${paymentID}/upi-intent`);
  if (response.status === 404) {
    return null;
  }
  if (!response.ok) {
    throw new Error(`upi intent fetch failed: ${response.status}`);
  }
  return (await response.json()) as UPIIntentItem;
}

export async function getUPIMandates() {
  await requireViewer();
  const response = await apiFetch("/v1/upi-mandates");
  if (!response.ok) {
    throw new Error(`upi mandates fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<UPIMandateItem>;
}

export async function getUPIMandate(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/upi-mandates/${id}`);
  if (response.status === 404) {
    notFound();
  }
  if (!response.ok) {
    throw new Error(`upi mandate fetch failed: ${response.status}`);
  }
  return (await response.json()) as UPIMandateItem;
}

export async function getUPIMandateEvents(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/upi-mandates/${id}/events`);
  if (!response.ok) {
    throw new Error(`upi mandate events fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<UPIMandateEventItem>;
}

export async function getAPIKeys() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/api-keys");
  if (!response.ok) {
    throw new Error(`api keys fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<APIKeyItem>;
}

export async function getRefunds(paymentID: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/payments/${paymentID}/refunds`);
  if (!response.ok) {
    throw new Error(`refunds fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<RefundItem>;
}

export async function getWebhooks() {
  await requireViewer();
  const response = await apiFetch("/v1/webhooks");
  if (!response.ok) {
    throw new Error(`webhooks fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<WebhookItem>;
}

export async function getWebhook(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/webhooks/${id}`);
  if (response.status === 404) {
    notFound();
  }
  if (!response.ok) {
    throw new Error(`webhook fetch failed: ${response.status}`);
  }
  return (await response.json()) as WebhookItem;
}

export async function getWebhookDeliveries(webhookID: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/webhooks/${webhookID}/deliveries`);
  if (!response.ok) {
    throw new Error(`deliveries fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<DeliveryAttemptItem>;
}

export async function getSettlements() {
  await requireViewer();
  const response = await apiFetch("/v1/settlements");
  if (!response.ok) {
    throw new Error(`settlements fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<SettlementItem>;
}

export async function getSettlement(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/settlements/${id}`);
  if (response.status === 404) {
    notFound();
  }
  if (!response.ok) {
    throw new Error(`settlement fetch failed: ${response.status}`);
  }
  return (await response.json()) as SettlementItem & { items?: SettlementLineItem[] };
}

export async function getReconMismatches() {
  await requireViewer();
  const response = await apiFetch("/v1/recon/mismatches");
  if (!response.ok) {
    // Recon endpoint may not exist yet — return empty list gracefully.
    return { items: [] as ReconMismatch[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<ReconMismatch>;
}

export async function getRiskEvents() {
  await requireViewer();
  const response = await apiFetch("/v1/risk/events");
  if (!response.ok) {
    throw new Error(`risk events fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<RiskEventItem>;
}

export async function getOnboarding() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/onboarding");
  if (!response.ok) {
    throw new Error(`onboarding fetch failed: ${response.status}`);
  }
  return (await response.json()) as OnboardingApplicationItem;
}

export async function getOnboardingParties() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/onboarding/parties");
  if (!response.ok) {
    throw new Error(`onboarding parties fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<OnboardingPartyItem>;
}

export async function getOnboardingDocuments() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/onboarding/documents");
  if (!response.ok) {
    throw new Error(`onboarding documents fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<OnboardingDocumentItem>;
}

export async function getScreeningCases() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/onboarding/screenings");
  if (!response.ok) {
    throw new Error(`screening cases fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<ScreeningCaseItem>;
}

export async function getCapabilities() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/capabilities");
  if (!response.ok) {
    throw new Error(`capabilities fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<CapabilityItem>;
}

export async function getReservePolicy() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/reserve-policy");
  if (!response.ok) {
    throw new Error(`reserve policy fetch failed: ${response.status}`);
  }
  return (await response.json()) as ReservePolicyItem;
}

export async function getReserveEscalations() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/reserve-escalations");
  if (!response.ok) {
    throw new Error(`reserve escalations fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<ReserveEscalationItem>;
}

export async function getReportCatalog() {
  await requireViewer();
  const response = await apiFetch("/v1/reports/catalog");
  if (!response.ok) {
    throw new Error(`report catalog fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<ReportCatalogItem>;
}

export async function getReportExports() {
  await requireViewer();
  const response = await apiFetch("/v1/reports/exports");
  if (!response.ok) {
    throw new Error(`report exports fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<ExportJobItem>;
}

export async function getTaxProfile() {
  await requireViewer();
  const response = await apiFetch("/v1/reports/tax-profile");
  if (!response.ok) {
    throw new Error(`tax profile fetch failed: ${response.status}`);
  }
  return (await response.json()) as TaxProfileItem;
}

export async function resolveRiskEvent(id: string, resolvedBy: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/risk/events/${id}/resolve`, {
    method: "POST",
    body: JSON.stringify({ resolved_by: resolvedBy }),
  });
  if (!response.ok) {
    throw new Error(`resolve risk event failed: ${response.status}`);
  }
  return (await response.json()) as RiskEventItem;
}

export async function getAuditLogs(params?: {
  resource_type?: string;
  resource_id?: string;
  actor_id?: string;
}) {
  await requireViewer();
  const qs = params
    ? "?" +
      Object.entries(params)
        .filter(([, v]) => v)
        .map(([k, v]) => `${k}=${encodeURIComponent(v!)}`)
        .join("&")
    : "";
  const response = await apiFetch(`/v1/audit-logs${qs}`);
  if (!response.ok) {
    throw new Error(`audit logs fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<AuditLogItem>;
}

export async function getInvitations() {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/invitations");
  if (!response.ok) {
    throw new Error(`invitations fetch failed: ${response.status}`);
  }
  return (await response.json()) as CollectionResponse<InvitationItem>;
}

export async function inviteTeamMember(email: string, role: string) {
  await requireViewer();
  const response = await apiFetch("/v1/merchants/me/invitations", {
    method: "POST",
    body: JSON.stringify({ email, role }),
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error((body as { error?: { description?: string } }).error?.description ?? "invite failed");
  }
  return (await response.json()) as InvitationItem;
}

export async function revokeInvitation(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/merchants/me/invitations/${id}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    throw new Error(`revoke invitation failed: ${response.status}`);
  }
}

export async function getDisputes() {
  await requireViewer();
  const response = await apiFetch("/v1/disputes");
  if (!response.ok) {
    return { items: [] as DisputeItem[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<DisputeItem>;
}

export async function getDispute(id: string) {
  await requireViewer();
  const response = await apiFetch(`/v1/disputes/${id}`);
  if (response.status === 404) {
    notFound();
  }
  if (!response.ok) {
    throw new Error(`dispute fetch failed: ${response.status}`);
  }
  return (await response.json()) as DisputeItem;
}

export async function getPayouts() {
  await requireViewer();
  const response = await apiFetch("/v1/payouts");
  if (!response.ok) {
    return { items: [] as PayoutItem[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<PayoutItem>;
}

export async function getBeneficiaries() {
  await requireViewer();
  const response = await apiFetch("/v1/beneficiaries");
  if (!response.ok) {
    return { items: [] as BeneficiaryItem[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<BeneficiaryItem>;
}

export async function getSagas() {
  await requireViewer();
  const response = await apiFetch("/v1/sagas?count=20");
  if (!response.ok) {
    return { items: [] as SagaItem[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<SagaItem>;
}

export async function getEventSchemas() {
  await requireViewer();
  const response = await apiFetch("/v1/event-schemas");
  if (!response.ok) {
    return { items: [] as EventSchemaItem[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<EventSchemaItem>;
}

export async function getLedgerHolds() {
  await requireViewer();
  const response = await apiFetch("/v1/ledger/holds?count=20");
  if (!response.ok) {
    return { items: [] as LedgerHoldItem[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<LedgerHoldItem>;
}

export async function getGatewayScenarios() {
  const response = await apiFetch("/v1/gateway/scenarios");
  if (!response.ok) {
    return { items: [] as GatewayScenario[], count: 0 };
  }
  return (await response.json()) as CollectionResponse<GatewayScenario>;
}

export async function getActiveGatewayScenario() {
  const response = await apiFetch("/v1/gateway/scenarios/active");
  if (!response.ok) {
    return null;
  }
  return (await response.json()) as GatewayScenario;
}

/**
 * Fetches Prometheus metrics from the API gateway and parses a specific
 * metric's instantaneous value (sum across all labels).
 * Returns null if the metric is not found or the endpoint is unreachable.
 */
export async function getPrometheusMetricSum(metricName: string): Promise<number | null> {
  try {
    const res = await fetch(`${getInternalApiBaseUrl()}/metrics`, {
      cache: 'no-store',
    });
    if (!res.ok) return null;
    const text = await res.text();
    const lines = text.split('\n');
    let sum = 0;
    let found = false;
    for (const line of lines) {
      if (line.startsWith('#') || line.trim() === '') continue;
      const parts = line.split(' ');
      if (parts.length < 2) continue;
      const nameWithLabels = parts[0];
      const value = parseFloat(parts[1]);
      if (isNaN(value)) continue;
      // Match metric name (with or without labels)
      const name = nameWithLabels.includes('{')
        ? nameWithLabels.slice(0, nameWithLabels.indexOf('{'))
        : nameWithLabels;
      if (name === metricName) {
        sum += value;
        found = true;
      }
    }
    return found ? sum : null;
  } catch {
    return null;
  }
}

export async function getPrometheusMetricSeries(metricName: string): Promise<PrometheusSeriesPoint[]> {
  try {
    const res = await fetch(`${getInternalApiBaseUrl()}/metrics`, { cache: "no-store" });
    if (!res.ok) return [];
    const text = await res.text();
    const lines = text.split("\n");
    const points: PrometheusSeriesPoint[] = [];
    for (const line of lines) {
      if (line.startsWith("#") || line.trim() === "") continue;
      const parts = line.trim().split(/\s+/);
      if (parts.length < 2) continue;
      const metric = parts[0];
      const value = Number(parts[1]);
      if (Number.isNaN(value)) continue;
      const braceIdx = metric.indexOf("{");
      const name = braceIdx === -1 ? metric : metric.slice(0, braceIdx);
      if (name !== metricName) continue;
      const labels: Record<string, string> = {};
      if (braceIdx !== -1 && metric.endsWith("}")) {
        const body = metric.slice(braceIdx + 1, -1);
        for (const pair of body.split(",")) {
          if (!pair) continue;
          const eqIdx = pair.indexOf("=");
          if (eqIdx === -1) continue;
          const key = pair.slice(0, eqIdx);
          const raw = pair.slice(eqIdx + 1).trim();
          labels[key] = raw.startsWith("\"") && raw.endsWith("\"") ? raw.slice(1, -1) : raw;
        }
      }
      points.push({ labels, value });
    }
    return points;
  } catch {
    return [];
  }
}
