import http from "k6/http";
import encoding from "k6/encoding";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

const orderCreationSuccess = new Rate("order_creation_success");
const orderCreationDuration = new Trend("order_creation_duration_ms");

const BASE_URL = __ENV.BASE_URL || "http://127.0.0.1:8090";
const API_KEY = __ENV.API_KEY || "";
const API_SECRET = __ENV.API_SECRET || "";

export const options = {
  stages: [
    { duration: "10s", target: 3 },
    { duration: "20s", target: 5 },
    { duration: "10s", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<750", "p(99)<1200"],
    order_creation_success: ["rate>0.99"],
    order_creation_duration_ms: ["p(95)<600"],
  },
};

function authHeader(idempotencyKey) {
  return {
    Authorization: `Basic ${encoding.b64encode(`${API_KEY}:${API_SECRET}`)}`,
    "Content-Type": "application/json",
    "Idempotency-Key": idempotencyKey,
  };
}

export default function () {
  const ready = http.get(`${BASE_URL}/readyz`);
  check(ready, { "readyz 200": (r) => r.status === 200 });

  const payload = JSON.stringify({
    amount: 10000 + ((__VU + __ITER) % 9) * 1000,
    currency: "INR",
    receipt: `ci-smoke-${__VU}-${__ITER}`,
    notes: { suite: "k6-ci-smoke" },
  });

  const startedAt = Date.now();
  const response = http.post(
    `${BASE_URL}/v1/orders`,
    payload,
    { headers: authHeader(`ci-smoke-order-${__VU}-${__ITER}`), timeout: "10s" },
  );
  orderCreationDuration.add(Date.now() - startedAt);

  const ok = check(response, {
    "order created": (r) => r.status === 201,
    "order entity": (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.entity === "order" && typeof body.id === "string" && body.id.startsWith("order_");
      } catch (err) {
        return false;
      }
    },
  });
  orderCreationSuccess.add(ok);

  sleep(0.2);
}
