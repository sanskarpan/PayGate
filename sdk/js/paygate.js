export const VERSION = "0.1.0";

function encodeBase64(value) {
  if (typeof Buffer !== "undefined") {
    return Buffer.from(value, "utf8").toString("base64");
  }
  if (typeof btoa !== "undefined") {
    return btoa(value);
  }
  throw new Error("No base64 encoder available in this runtime");
}

async function sha256Hex(secret, bodyText) {
  if (typeof globalThis.crypto?.subtle !== "undefined") {
    const enc = new TextEncoder();
    const key = await globalThis.crypto.subtle.importKey(
      "raw",
      enc.encode(secret),
      { name: "HMAC", hash: "SHA-256" },
      false,
      ["sign"],
    );
    const sig = await globalThis.crypto.subtle.sign("HMAC", key, enc.encode(bodyText));
    return Array.from(new Uint8Array(sig))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }
  if (typeof Buffer !== "undefined") {
    const { createHmac } = await import("node:crypto");
    return createHmac("sha256", secret).update(bodyText).digest("hex");
  }
  throw new Error("No HMAC implementation available in this runtime");
}

export function createClient({ baseUrl, keyId, keySecret, fetchImpl = fetch }) {
  const authHeader = "Basic " + encodeBase64(`${keyId}:${keySecret}`);

  async function request(path, init = {}) {
    const headers = new Headers(init.headers || {});
    headers.set("Authorization", authHeader);
    if (!headers.has("Content-Type") && init.body) {
      headers.set("Content-Type", "application/json");
    }
    const response = await fetchImpl(`${baseUrl}${path}`, { ...init, headers });
    if (!response.ok) {
      throw new Error(`PayGate request failed: ${response.status}`);
    }
    return response.json();
  }

  return {
    version: VERSION,
    createOrder(payload, idempotencyKey) {
      return request("/v1/orders", {
        method: "POST",
        headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {},
        body: JSON.stringify(payload),
      });
    },
    createPayment(payload, idempotencyKey) {
      return request("/v1/payments/authorize", {
        method: "POST",
        headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {},
        body: JSON.stringify(payload),
      });
    },
    capturePayment(paymentId, payload = {}, idempotencyKey) {
      return request(`/v1/payments/${paymentId}/capture`, {
        method: "POST",
        headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {},
        body: JSON.stringify(payload),
      });
    },
    createRefund(paymentId, payload, idempotencyKey) {
      return request(`/v1/payments/${paymentId}/refunds`, {
        method: "POST",
        headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {},
        body: JSON.stringify(payload),
      });
    },
  };
}

export async function verifyWebhookSignature({ secret, bodyText, signature }) {
  if (!secret || !signature) {
    return false;
  }
  const expected = await sha256Hex(secret, bodyText);
  return expected === signature;
}
