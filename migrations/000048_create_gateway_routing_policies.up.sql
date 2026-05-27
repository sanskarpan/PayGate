CREATE TABLE IF NOT EXISTS paygate_gateway.routing_policies (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL,
    method TEXT NOT NULL,
    primary_provider TEXT NOT NULL DEFAULT '',
    fallback_provider TEXT NOT NULL DEFAULT '',
    force_provider TEXT NOT NULL DEFAULT '',
    failover_on_decline BOOLEAN NOT NULL DEFAULT true,
    failover_on_error BOOLEAN NOT NULL DEFAULT true,
    cost_weight INTEGER NOT NULL DEFAULT 50,
    success_weight INTEGER NOT NULL DEFAULT 50,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, method)
);

CREATE INDEX IF NOT EXISTS idx_gateway_routing_policies_merchant_method
    ON paygate_gateway.routing_policies(merchant_id, method);
