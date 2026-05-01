ALTER TABLE paygate_settlements.settlements
    DROP COLUMN IF EXISTS on_hold,
    DROP COLUMN IF EXISTS hold_reason,
    DROP COLUMN IF EXISTS held_at,
    DROP COLUMN IF EXISTS released_at;
