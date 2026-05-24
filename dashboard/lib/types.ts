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
  method_state?: string;
  method_state_reason?: string;
  captured: boolean;
  captured_at: number;
  authorized_at: number;
  created_at: number;
};

export type UPIIntentItem = PaymentItem & {
  vpa: string;
  provider_status: string;
  gateway_reference: string;
  expires_at: number;
  completed_at?: number;
  last_polled_at?: number;
  failure_code?: string;
  failure_description?: string;
  next_action?: {
    type: string;
    deep_link: string;
    intent_uri: string;
    expires_at: number;
  };
};

export type APIKeyItem = {
  id: string;
  mode: string;
  scope: string;
  status: string;
  allowed_ips?: string[];
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
  source_import_id?: string;
  status?: string;
  assigned_to?: string;
  assigned_at?: number | null;
  resolved_at?: number | null;
  resolved_by?: string;
  resolution_code?: string;
  resolution_notes?: string;
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
  device_fingerprint_hash?: string;
  browser_language?: string;
  user_agent?: string;
  card_bin?: string;
  card_network?: string;
  issuer_country?: string;
  card_country?: string;
  funding_type?: string;
  review_status?: string;
  assigned_to?: string;
  assigned_at?: number | null;
  review_notes?: string;
  manual_decision?: string;
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

export type DisputeItem = {
  id: string;
  merchant_id: string;
  payment_id: string;
  settlement_id: string | null;
  status: string;
  reason: string;
  amount: number;
  currency: string;
  evidence: Record<string, unknown> | null;
  evidence_submitted_at: number | null;
  due_by: number | null;
  resolved_at: number | null;
  notes: string;
  created_at: number;
};

export type PayoutItem = {
  id: string;
  merchant_id: string;
  settlement_id: string;
  beneficiary_id?: string;
  saga_id?: string;
  status: string;
  approval_status?: string;
  amount: number;
  currency: string;
  bank_reference: string;
  rail_reference?: string;
  failure_reason: string;
  return_reason?: string;
  initiated_at: number | null;
  completed_at: number | null;
  failed_at?: number | null;
  returned_at?: number | null;
  reversed_at?: number | null;
  cancelled_at?: number | null;
  cancel_reason?: string;
  created_at: number;
};

export type BeneficiaryItem = {
  id: string;
  destination_type: string;
  account_holder_name: string;
  bank_account_last4?: string;
  bank_ifsc?: string;
  vpa?: string;
  status: string;
  verification_fresh_until?: number | null;
  approved_at?: number | null;
  approval_notes?: string;
};

export type OnboardingApplicationItem = {
  id: string;
  merchant_id: string;
  legal_name: string;
  business_classification: string;
  registration_number: string;
  tax_identifier: string;
  country_code: string;
  state: string;
  reviewer_notes: string;
  submitted_at?: number | null;
  reviewed_at?: number | null;
  approved_at?: number | null;
  rejected_at?: number | null;
  created_at: number;
  updated_at: number;
};

export type OnboardingPartyItem = {
  id: string;
  party_type: string;
  full_name: string;
  title: string;
  email: string;
  phone: string;
  ownership_bps: number;
  verification_status: string;
  evidence_notes: string;
  revision: number;
};

export type OnboardingDocumentItem = {
  id: string;
  document_type: string;
  file_name: string;
  content_type: string;
  storage_key: string;
  request_reason: string;
  review_notes: string;
  status: string;
  expires_at?: string | null;
};

export type ScreeningCaseItem = {
  id: string;
  screening_type: string;
  provider: string;
  provider_reference: string;
  subject_name: string;
  status: string;
  result_payload: Record<string, unknown>;
  screened_at: number;
};

export type CapabilityItem = {
  id: string;
  capability_code: string;
  status: string;
  reason: string;
};

export type ReservePolicyItem = {
  id: string;
  policy_type: string;
  percentage_bps: number;
  hold_days: number;
  threshold_amount: number;
  notes: string;
};

export type ReserveEscalationItem = {
  id: string;
  merchant_id: string;
  risk_event_id: string;
  trigger_score: number;
  triggered_rules: string[];
  status: string;
  suggested_policy_type: string;
  suggested_percentage_bps: number;
  suggested_hold_days: number;
  suggested_threshold_amount: number;
  rationale: string;
  review_notes: string;
  reviewed_by: string;
  reviewed_at?: number | null;
  created_at: number;
  updated_at: number;
};

export type ReportCatalogItem = {
  report_type: string;
  label: string;
  description: string;
  supports_api: boolean;
};

export type ExportJobItem = {
  id: string;
  report_type: string;
  format: string;
  status: string;
  file_name: string;
  content_type: string;
  file_size_bytes: number;
  filters: Record<string, unknown>;
  download_url: string;
  download_expires_at: number;
  error_message: string;
  created_at: number;
  completed_at?: number | null;
};

export type TaxProfileItem = {
  merchant_id: string;
  legal_name: string;
  gstin: string;
  business_state_code: string;
  place_of_supply: string;
  default_tax_rate_bps: number;
  created_at: number;
  updated_at: number;
};

export type SagaStepItem = {
  id: string;
  step_index: number;
  step_name: string;
  step_kind: string;
  status: string;
  command_name: string;
  command_id: string;
  error_code?: string;
  error_message?: string;
  attempt_count: number;
  max_attempts: number;
  leased_by?: string;
  leased_at?: number | null;
  completed_at?: number | null;
  created_at: number;
};

export type SagaItem = {
  id: string;
  merchant_id: string;
  saga_type: string;
  status: string;
  correlation_id: string;
  causation_id: string;
  current_step_index: number;
  failure_code?: string;
  failure_reason?: string;
  replay_count: number;
  deadline_at?: number | null;
  timeout_at?: number | null;
  started_at: number;
  completed_at?: number | null;
  created_at: number;
  updated_at: number;
  steps: SagaStepItem[];
};

export type EventSchemaItem = {
  id: string;
  subject: string;
  event_type: string;
  topic_name: string;
  owner: string;
  review_link: string;
  created_at: number;
  updated_at: number;
};

export type LedgerHoldItem = {
  id: string;
  merchant_id: string;
  account_code: string;
  source_type: string;
  source_id: string;
  reason: string;
  currency: string;
  amount: number;
  status: string;
  idempotency_key?: string;
  target_account_code?: string;
  description?: string;
  expires_at?: number | null;
  released_at?: number | null;
  committed_at?: number | null;
  created_at: number;
};

export type GatewayScenario = {
  id: string;
  merchant_id: string;
  mode: string;
  failure_rate: number;
  delay_ms: number;
  decline_code: string;
  active: boolean;
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

export function formatCompactNumber(value: number | null | undefined) {
  if (value === null || value === undefined) return "0";
  return new Intl.NumberFormat("en-IN", {
    notation: "compact",
    maximumFractionDigits: value >= 1000 ? 1 : 0,
  }).format(value);
}

export function truncateMiddle(value: string, head = 8, tail = 6) {
  if (!value) return "";
  if (value.length <= head + tail + 3) return value;
  return `${value.slice(0, head)}...${value.slice(-tail)}`;
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
