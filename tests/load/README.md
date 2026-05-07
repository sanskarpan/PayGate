# PayGate Load Tests

k6-based load tests for all critical endpoints.

## Prerequisites

Install k6: https://grafana.com/docs/k6/latest/get-started/installation/

## Running tests

Set environment variables:
```bash
export BASE_URL=http://localhost:8090
export API_KEY=rzp_test_your_key_id
export API_SECRET=your_key_secret
```

### Smoke test (quick sanity check)
```bash
k6 run tests/load/smoke.js
```

### Order creation stress test (p99 < 200ms target)
```bash
k6 run tests/load/orders.js
```

### Full payment flow (order → authorize → capture)
```bash
k6 run tests/load/payments_full_flow.js
```

### Spike test (5x normal load for 5 minutes)
```bash
k6 run tests/load/spike.js
```

### With output to InfluxDB (Grafana)
```bash
k6 run --out influxdb=http://localhost:8086/k6 tests/load/orders.js
```

## Targets

| Test | VUs | p99 target | Error rate |
|------|-----|-----------|------------|
| Smoke | 1 | <500ms | <1% |
| Orders | 50 | <200ms | <1% |
| Full flow | 20 | <500ms capture | <10% |
| Spike | 50 | <2000ms | <10% |
