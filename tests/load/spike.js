import http from "k6/http";
import encoding from "k6/encoding";
import { check, sleep } from "k6";

export const options = {
  stages: [
    { duration: "2m", target: 10 },    // warm up
    { duration: "30s", target: 50 },   // spike to 5x
    { duration: "5m", target: 50 },    // hold spike
    { duration: "1m", target: 10 },    // back to normal
    { duration: "2m", target: 0 },     // ramp down
  ],
  thresholds: {
    http_req_failed: ["rate<0.10"],     // allow higher error rate during spike
    http_req_duration: ["p(95)<2000"],  // allow slower responses during spike
  },
};

const BASE_URL = __ENV.BASE_URL || "http://localhost:8090";
const API_KEY = __ENV.API_KEY || "rzp_test_demo";
const API_SECRET = __ENV.API_SECRET || "secret_demo";

function authHeader() {
  return {
    Authorization: `Basic ${encoding.b64encode(`${API_KEY}:${API_SECRET}`)}`,
    "Content-Type": "application/json",
    "Idempotency-Key": `spike-${__VU}-${__ITER}-${Date.now()}`,
  };
}

export default function () {
  // Mix of read and write operations
  const op = Math.random();

  if (op < 0.5) {
    // 50%: create orders (write)
    const res = http.post(
      `${BASE_URL}/v1/orders`,
      JSON.stringify({ amount: 50000, currency: "INR", receipt: `spike_${__VU}_${__ITER}` }),
      { headers: authHeader() }
    );
    check(res, { "order ok": (r) => r.status === 201 || r.status === 200 });
  } else if (op < 0.8) {
    // 30%: list orders (read)
    const res = http.get(`${BASE_URL}/v1/orders?count=10`, {
      headers: { Authorization: `Basic ${encoding.b64encode(`${API_KEY}:${API_SECRET}`)}` },
    });
    check(res, { "list ok": (r) => r.status === 200 });
  } else {
    // 20%: health check
    const res = http.get(`${BASE_URL}/healthz`);
    check(res, { "health ok": (r) => r.status === 200 });
  }

  sleep(0.1);
}
