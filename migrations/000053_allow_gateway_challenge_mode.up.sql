ALTER TABLE paygate_gateway.gateway_scenarios
    DROP CONSTRAINT IF EXISTS gateway_scenarios_mode_check;

ALTER TABLE paygate_gateway.gateway_scenarios
    ADD CONSTRAINT gateway_scenarios_mode_check
    CHECK (mode IN ('success', 'slow', 'flaky', 'timeout', 'decline', 'late_callback', 'challenge'));
