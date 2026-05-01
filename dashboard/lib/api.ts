import { cookies } from "next/headers";
import { notFound, redirect } from "next/navigation";

import type {
  APIKeyItem,
  DeliveryAttemptItem,
  DashboardViewer,
  OrderItem,
  PaymentItem,
  ReconMismatch,
  RefundItem,
  SettlementItem,
  SettlementLineItem,
  WebhookItem,
} from "./types";

type CollectionResponse<T> = {
  items: T[];
  count: number;
  has_more?: boolean;
  next_cursor?: string;
};

export function getApiBaseUrl() {
  return process.env.API_BASE_URL || "http://localhost:8080";
}

export function getAppBaseUrl() {
  return process.env.APP_BASE_URL || "http://localhost:3000";
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
  return fetch(`${getApiBaseUrl()}${path}`, {
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

export async function getOrders() {
  await requireViewer();
  const response = await apiFetch("/v1/orders?count=25");
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
  return (await response.json()) as SettlementItem & { items: SettlementLineItem[] };
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
