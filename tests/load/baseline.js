/**
 * PayGate — Baseline Performance Test
 *
 * Goal: Verify the system sustains 1000 orders/second under steady-state load.
 * Model: Open arrival rate (constant RPS regardless of response time).
 * Duration: 5-minute warmup ramp + 10-minute sustained plateau.
 *
 * Run:
 *   k6 run --env BASE_URL=http://localhost:8090 \
 *           --env API_KEY=<key> --env API_SECRET=<secret> \
 *           tests/load/baseline.js
 *
 * Pass criteria:
 *   - p99 order creation latency < 500ms
 *   - Error rate < 1%
 *   - Achieved RPS ≥ 900 (≥ 90% of target) during plateau
 */

import http from 'k6/http';
import encoding from 'k6/encoding';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const orderErrors    = new Rate('order_errors');
const orderLatency   = new Trend('order_latency_ms', true);
const ordersCreated  = new Counter('orders_created');

const BASE_URL   = __ENV.BASE_URL   || 'http://localhost:8090';
const API_KEY    = __ENV.API_KEY    || '';
const API_SECRET = __ENV.API_SECRET || '';

export const options = {
  scenarios: {
    baseline: {
      executor: 'ramping-arrival-rate',
      startRate: 0,
      timeUnit: '1s',
      preAllocatedVUs: 200,
      maxVUs: 500,
      stages: [
        { target: 100, duration: '1m' },   // warm up to 100 RPS
        { target: 500, duration: '2m' },   // ramp to 500 RPS
        { target: 1000, duration: '2m' },  // ramp to 1000 RPS
        { target: 1000, duration: '10m' }, // hold 1000 RPS for 10 minutes
        { target: 0,    duration: '1m' },  // ramp down
      ],
    },
  },
  thresholds: {
    order_errors:     ['rate<0.01'],              // < 1% error rate
    order_latency_ms: ['p(99)<500', 'p(95)<250'], // p99 < 500ms, p95 < 250ms
    http_req_failed:  ['rate<0.01'],
  },
};

function authHeaders() {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Basic ${__ENV.AUTH_B64 || encoding.b64encode(`${API_KEY}:${API_SECRET}`)}`,
  };
}

export default function () {
  const amount   = Math.floor(Math.random() * 90000) + 10000; // ₹100–₹1000
  const receipt  = `baseline_${__VU}_${__ITER}`;
  const payload  = JSON.stringify({ amount, currency: 'INR', receipt });

  const start = Date.now();
  const res = http.post(
    `${BASE_URL}/v1/orders`,
    payload,
    { headers: authHeaders(), timeout: '10s' },
  );
  const elapsed = Date.now() - start;

  const ok = check(res, {
    'status 201': (r) => r.status === 201,
    'has order id': (r) => {
      try { return !!JSON.parse(r.body).id; } catch (err) { return false; }
    },
  });

  orderErrors.add(!ok);
  orderLatency.add(elapsed);
  if (ok) ordersCreated.add(1);
}

export function handleSummary(data) {
  const rps     = data.metrics.iterations ? data.metrics.iterations.values.rate : 0;
  const p99     = data.metrics.order_latency_ms ? data.metrics.order_latency_ms.values['p(99)'] : 0;
  const errRate = data.metrics.order_errors ? data.metrics.order_errors.values.rate : 0;

  console.log('\n=== Baseline Performance Summary ===');
  console.log(`Target RPS  : 1000`);
  console.log(`Achieved RPS: ${rps.toFixed(1)}`);
  console.log(`p99 latency : ${p99.toFixed(0)}ms  (threshold: <500ms)`);
  console.log(`Error rate  : ${(errRate * 100).toFixed(2)}%  (threshold: <1%)`);
  console.log(`Orders OK   : ${data.metrics.orders_created ? data.metrics.orders_created.values.count : 0}`);
  console.log('=====================================\n');

  return {
    stdout: JSON.stringify(data, null, 2),
  };
}
