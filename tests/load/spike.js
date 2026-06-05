import http from "k6/http";
import encoding from "k6/encoding";
import { check, sleep } from "k6";

export const options = {
  stages: [
    { duration: "45s", target: 4 },
    { duration: "45s", target: 12 },
    { duration: "2m", target: 12 },
    { duration: "45s", target: 4 },
    { duration: "30s", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.02"],
    http_req_duration: ["p(95)<2000"],
  },
};

const BASE_URL = __ENV.BASE_URL || "http://localhost:8090";
const API_KEY = __ENV.API_KEY || "rzp_test_demo";
const API_SECRET = __ENV.API_SECRET || "secret_demo";
const API_KEYS = (__ENV.API_KEYS || "").split(",").filter(Boolean);
const API_SECRETS = (__ENV.API_SECRETS || "").split(",").filter(Boolean);

function credentials() {
  if (API_KEYS.length > 0 && API_KEYS.length === API_SECRETS.length) {
    const idx = ((__VU - 1) + __ITER) % API_KEYS.length;
    return { key: API_KEYS[idx], secret: API_SECRETS[idx] };
  }
  return { key: API_KEY, secret: API_SECRET };
}

function authHeader() {
  const { key, secret } = credentials();
  return {
    Authorization: `Basic ${encoding.b64encode(`${key}:${secret}`)}`,
    "Content-Type": "application/json",
    "Idempotency-Key": `spike-${__VU}-${__ITER}-${Date.now()}`,
  };
}

function authOnlyHeader() {
  const { key, secret } = credentials();
  return {
    Authorization: `Basic ${encoding.b64encode(`${key}:${secret}`)}`,
  };
}

export function setup() {
  const ready = http.get(`${BASE_URL}/readyz`);
  check(ready, { "readyz 200": (r) => r.status === 200 });
}

export default function () {
  const op = Math.random();

  if (op < 0.65) {
    const res = http.post(
      `${BASE_URL}/v1/orders`,
      JSON.stringify({ amount: 50000, currency: "INR", receipt: `spike_${__VU}_${__ITER}` }),
      { headers: authHeader() }
    );
    check(res, { "order ok": (r) => r.status === 201 || r.status === 200 });
  } else {
    const res = http.get(`${BASE_URL}/v1/orders?count=10`, {
      headers: authOnlyHeader(),
    });
    check(res, { "list ok": (r) => r.status === 200 });
  }

  sleep(0.35);
}
