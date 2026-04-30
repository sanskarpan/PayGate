CREATE TABLE IF NOT EXISTS public.outbox (
    id              TEXT PRIMARY KEY,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    merchant_id     TEXT NOT NULL,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON public.outbox(created_at)
    WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_outbox_cleanup ON public.outbox(published_at)
    WHERE published_at IS NOT NULL;
