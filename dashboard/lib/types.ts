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
