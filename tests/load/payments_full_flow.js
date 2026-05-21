import http from "k6/http";
import encoding from "k6/encoding";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

export const options = {
  stages: [
    { duration: "2m", target: 20 },   // ramp up slowly
    { duration: "5m", target: 20 },   // hold at 20 VUs
    { duration: "1m", target: 0 },    // ramp down
  ],
  thresholds: {
    http_req_failed: ["rate<0.05"],
    "payment_captured": ["rate>0.90"],
    "capture_duration_ms": ["p(95)<500"],
  },
};

const BASE_URL = __ENV.BASE_URL || "http://localhost:8090";
const API_KEY = __ENV.API_KEY || "rzp_test_demo";
const API_SECRET = __ENV.API_SECRET || "secret_demo";

const paymentCaptured = new Rate("payment_captured");
const captureDuration = new Trend("capture_duration_ms");

function headers(idempotencyKey) {
  const h = {
    Authorization: `Basic ${encoding.b64encode(`${API_KEY}:${API_SECRET}`)}`,
    "Content-Type": "application/json",
  };
  if (idempotencyKey) h["Idempotency-Key"] = idempotencyKey;
  return h;
}

export default function () {
  // Step 1: Create order
  const amount = Math.floor(Math.random() * 50000) + 10000;
  const orderRes = http.post(
    `${BASE_URL}/v1/orders`,
    JSON.stringify({ amount, currency: "INR", receipt: `rcpt_${__VU}_${__ITER}` }),
    { headers: headers(`idem-order-${__VU}-${__ITER}`) }
  );
  if (!check(orderRes, { "order created": (r) => r.status === 201 })) {
    sleep(1);
    return;
  }
  const order = JSON.parse(orderRes.body);

  sleep(0.1);

  // Step 2: Authorize payment (via checkout endpoint)
  const payRes = http.post(
    `${BASE_URL}/checkout/pay`,
    JSON.stringify({
      order_id: order.id,
      amount: order.amount,
      currency: order.currency,
      method: "card",
      card_number: "4111111111111111",
      card_expiry: "12/26",
      card_cvv: "123",
    }),
    { headers: { "Content-Type": "application/json" } }
  );
  if (!check(payRes, { "payment authorized": (r) => r.status === 200 || r.status === 201 })) {
    sleep(1);
    return;
  }

  let paymentId;
  try {
    const body = JSON.parse(payRes.body);
    paymentId = body.payment_id || body.id;
  } catch (err) {
    sleep(1);
    return;
  }

  if (!paymentId) { sleep(1); return; }

  sleep(0.2);

  // Step 3: Capture
  const captureStart = Date.now();
  const captureRes = http.post(
    `${BASE_URL}/v1/payments/${paymentId}/capture`,
    JSON.stringify({ amount, currency: "INR" }),
    { headers: headers(`idem-capture-${__VU}-${__ITER}`) }
  );
  captureDuration.add(Date.now() - captureStart);

  const captured = check(captureRes, {
    "payment captured": (r) => r.status === 200,
    "status is captured": (r) => {
      try { return JSON.parse(r.body).status === "captured"; } catch (err) { return false; }
    },
  });
  paymentCaptured.add(captured);

  sleep(0.5);
}
