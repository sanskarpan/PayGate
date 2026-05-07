import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  vus: 1,
  duration: "30s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
  },
};

const BASE_URL = __ENV.BASE_URL || "http://localhost:8090";
const API_KEY = __ENV.API_KEY || "";
const API_SECRET = __ENV.API_SECRET || "";

function authHeader() {
  return { Authorization: `Basic ${btoa(`${API_KEY}:${API_SECRET}`)}` };
}

export default function () {
  // Health check
  const health = http.get(`${BASE_URL}/healthz`);
  check(health, { "healthz 200": (r) => r.status === 200 });

  // Readiness check
  const ready = http.get(`${BASE_URL}/readyz`);
  check(ready, { "readyz 200": (r) => r.status === 200 });

  if (API_KEY) {
    // List orders
    const orders = http.get(`${BASE_URL}/v1/orders`, { headers: authHeader() });
    check(orders, { "orders 200": (r) => r.status === 200 });
  }

  sleep(1);
}
