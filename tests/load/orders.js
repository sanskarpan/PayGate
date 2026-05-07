import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

export const options = {
  stages: [
    { duration: "1m", target: 50 },   // ramp up
    { duration: "3m", target: 50 },   // hold
    { duration: "30s", target: 0 },   // ramp down
  ],
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(99)<200", "p(95)<100"],
    "order_creation_success": ["rate>0.99"],
  },
};

const BASE_URL = __ENV.BASE_URL || "http://localhost:8090";
const API_KEY = __ENV.API_KEY || "rzp_test_demo";
const API_SECRET = __ENV.API_SECRET || "secret_demo";

const orderCreationSuccess = new Rate("order_creation_success");
const orderCreationDuration = new Trend("order_creation_duration_ms");

function authHeader() {
  return {
    Authorization: `Basic ${btoa(`${API_KEY}:${API_SECRET}`)}`,
    "Content-Type": "application/json",
    "Idempotency-Key": `load-test-order-${__VU}-${__ITER}`,
  };
}

export default function () {
  const amount = Math.floor(Math.random() * 100000) + 10000; // 100 - 1000 INR in paise
  const currencies = ["INR"];
  const currency = currencies[Math.floor(Math.random() * currencies.length)];

  const start = Date.now();
  const res = http.post(
    `${BASE_URL}/v1/orders`,
    JSON.stringify({
      amount,
      currency,
      receipt: `rcpt_load_${__VU}_${__ITER}`,
      notes: { load_test: true },
    }),
    { headers: authHeader() }
  );
  const duration = Date.now() - start;

  orderCreationDuration.add(duration);
  const ok = check(res, {
    "order created": (r) => r.status === 201,
    "order has id": (r) => {
      try { return JSON.parse(r.body).id?.startsWith("order_"); } catch { return false; }
    },
  });
  orderCreationSuccess.add(ok);

  sleep(0.1);
}
