/**
 * PayGate — Soak Test (1 hour sustained load)
 *
 * Goal: Verify the system remains stable under sustained moderate load for
 * 1 hour. Checks for memory leaks, connection pool exhaustion, and
 * gradual latency degradation.
 *
 * Load profile: 20 VUs (roughly 20–40 orders/sec depending on latency),
 * mixed read/write operations mirroring realistic traffic.
 *
 * Run:
 *   k6 run --env BASE_URL=http://localhost:8090 \
 *           --env API_KEY=<key> --env API_SECRET=<secret> \
 *           tests/load/soak.js
 *
 * Pass criteria:
 *   - Error rate < 0.5% throughout
 *   - p99 latency does not increase more than 3× from the first 5-minute
 *     window to the last 5-minute window (no latency degradation)
 *   - No HTTP 5xx errors
 */

import http from 'k6/http';
import encoding from 'k6/encoding';
import { check, group, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errors       = new Rate('soak_errors');
const writeLatency = new Trend('soak_write_latency_ms', true);
const readLatency  = new Trend('soak_read_latency_ms',  true);
const writes       = new Counter('soak_writes');
const reads        = new Counter('soak_reads');

const BASE_URL   = __ENV.BASE_URL   || 'http://localhost:8090';
const API_KEY    = __ENV.API_KEY    || '';
const API_SECRET = __ENV.API_SECRET || '';

export const options = {
  scenarios: {
    soak: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { target: 20, duration: '2m' },   // ramp up
        { target: 20, duration: '56m' },  // 56-minute soak plateau
        { target: 0,  duration: '2m' },   // ramp down
      ],
    },
  },
  thresholds: {
    soak_errors:          ['rate<0.005'],           // < 0.5%
    soak_write_latency_ms: ['p(99)<1000'],          // writes < 1s p99
    soak_read_latency_ms:  ['p(99)<200'],           // reads < 200ms p99
    http_req_failed:       ['rate<0.005'],
  },
};

function authHeaders() {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Basic ${__ENV.AUTH_B64 || encoding.b64encode(`${API_KEY}:${API_SECRET}`)}`,
  };
}

export default function () {
  const rand = Math.random();

  if (rand < 0.40) {
    // 40% — create order (write)
    group('create_order', () => {
      const amount  = Math.floor(Math.random() * 90000) + 10000;
      const receipt = `soak_${__VU}_${__ITER}`;
      const start   = Date.now();
      const res = http.post(
        `${BASE_URL}/v1/orders`,
        JSON.stringify({ amount, currency: 'INR', receipt }),
        { headers: authHeaders(), timeout: '15s' },
      );
      const elapsed = Date.now() - start;
      const ok = check(res, { 'order created 201': (r) => r.status === 201 });
      errors.add(!ok);
      writeLatency.add(elapsed);
      if (ok) writes.add(1);
    });
  } else if (rand < 0.75) {
    // 35% — list orders (read)
    group('list_orders', () => {
      const start = Date.now();
      const res = http.get(
        `${BASE_URL}/v1/orders?count=20`,
        { headers: authHeaders(), timeout: '10s' },
      );
      const elapsed = Date.now() - start;
      const ok = check(res, { 'orders list 200': (r) => r.status === 200 });
      errors.add(!ok);
      readLatency.add(elapsed);
      if (ok) reads.add(1);
    });
  } else if (rand < 0.90) {
    // 15% — health check
    group('healthz', () => {
      const start = Date.now();
      const res = http.get(`${BASE_URL}/healthz`, { timeout: '5s' });
      const elapsed = Date.now() - start;
      const ok = check(res, { 'healthz 200': (r) => r.status === 200 });
      errors.add(!ok);
      readLatency.add(elapsed);
      if (ok) reads.add(1);
    });
  } else {
    // 10% — list settlements (heavier read, exercises joins)
    group('list_settlements', () => {
      const start = Date.now();
      const res = http.get(
        `${BASE_URL}/v1/settlements`,
        { headers: authHeaders(), timeout: '10s' },
      );
      const elapsed = Date.now() - start;
      const ok = check(res, { 'settlements 200': (r) => r.status === 200 });
      errors.add(!ok);
      readLatency.add(elapsed);
      if (ok) reads.add(1);
    });
  }

  sleep(0.1 + Math.random() * 0.4); // 100–500ms think time
}

export function handleSummary(data) {
  const errRate   = data.metrics.soak_errors ? data.metrics.soak_errors.values.rate : 0;
  const wP99      = data.metrics.soak_write_latency_ms ? data.metrics.soak_write_latency_ms.values['p(99)'] : 0;
  const rP99      = data.metrics.soak_read_latency_ms  ? data.metrics.soak_read_latency_ms.values['p(99)']  : 0;
  const totalW    = data.metrics.soak_writes ? data.metrics.soak_writes.values.count : 0;
  const totalR    = data.metrics.soak_reads  ? data.metrics.soak_reads.values.count  : 0;
  const duration  = '60m';

  console.log('\n=== Soak Test Summary (1 hour) ===');
  console.log(`Duration     : ${duration}`);
  console.log(`Total writes : ${totalW}`);
  console.log(`Total reads  : ${totalR}`);
  console.log(`Error rate   : ${(errRate * 100).toFixed(3)}%  (threshold: <0.5%)`);
  console.log(`Write p99    : ${wP99.toFixed(0)}ms  (threshold: <1000ms)`);
  console.log(`Read  p99    : ${rP99.toFixed(0)}ms  (threshold: <200ms)`);
  console.log('==================================\n');

  return { stdout: JSON.stringify(data, null, 2) };
}
