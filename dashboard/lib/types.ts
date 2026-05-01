export type DashboardViewer = {
  merchant_id: string;
  user_id: string;
  email: string;
  role: string;
  scope: string;
  auth_type: string;
};

export type OrderItem = {
  id: string;
  amount: number;
  amount_paid: number;
  amount_due: number;
  currency: string;
  receipt: string;
  status: string;
  partial_payment: boolean;
  notes: Record<string, unknown>;
  created_at: number;
};

export type PaymentItem = {
  id: string;
  amount: number;
  currency: string;
  status: string;
  order_id: string;
  method: string;
  captured: boolean;
  captured_at: number;
  authorized_at: number;
  created_at: number;
};

export type APIKeyItem = {
  id: string;
  mode: string;
  scope: string;
  status: string;
  last_used_at: number;
  revoked_at: number;
  created_at: number;
};

export type RefundItem = {
  id: string;
  payment_id: string;
  amount: number;
  currency: string;
  reason: string;
  status: string;
  processed_at: number;
  created_at: number;
};

export type WebhookItem = {
  id: string;
  url: string;
  events: string[];
  status: string;
  created_at: number;
  updated_at: number;
};

export type DeliveryAttemptItem = {
  id: string;
  event_id: string;
  subscription_id: string;
  status: string;
  request_url: string;
  response_code: number;
  response_body: string;
  error: string;
  attempt_number: number;
  next_retry_at: number | null;
  created_at: number;
};

export type SettlementItem = {
  id: string;
  status: string;
  period_start: number;
  period_end: number;
  total_amount: number;
  total_fees: number;
  total_refunds: number;
  net_amount: number;
  payment_count: number;
  currency: string;
  processed_at: number | null;
  created_at: number;
};

export type SettlementLineItem = {
  id: string;
  payment_id: string;
  amount: number;
  fee: number;
  refunds: number;
  net: number;
  currency: string;
};

export type ReconMismatch = {
  id: string;
  batch_id: string;
  mismatch_type: string;
  entity_type: string;
  entity_id: string;
  expected_value: string;
  actual_value: string;
  description: string;
  resolved: boolean;
  created_at: number;
};

export type RiskEventItem = {
  id: string;
  merchant_id: string;
  payment_id: string;
  score: number;
  action: string;
  triggered_rules: string[];
  resolved: boolean;
  resolved_by: string | null;
  resolved_at: number | null;
  created_at: number;
};

export type AuditLogItem = {
  id: string;
  merchant_id: string;
  actor_id: string;
  actor_email: string;
  actor_type: string;
  action: string;
  resource_type: string;
  resource_id: string;
  ip_address: string;
  correlation_id: string;
  created_at: number;
};

export type InvitationItem = {
  id: string;
  merchant_id: string;
  email: string;
  role: string;
  status: string;
  invited_by: string;
  expires_at: number;
  accepted_at: number | null;
  created_at: number;
};

export function formatMoney(amount: number, currency: string) {
  return new Intl.NumberFormat("en-IN", {
    style: "currency",
    currency,
    maximumFractionDigits: 2,
    minimumFractionDigits: 2,
  }).format(amount / 100);
}

export function formatTime(unixSeconds: number) {
  if (!unixSeconds) {
    return "Not available";
  }
  return new Date(unixSeconds * 1000).toLocaleString("en-IN", {
    dateStyle: "medium",
    timeStyle: "short",
  });
}
