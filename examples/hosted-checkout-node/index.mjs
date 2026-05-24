import http from "node:http";
import { createClient } from "../../sdk/js/paygate.js";

const baseUrl = process.env.PAYGATE_BASE_URL || "http://127.0.0.1:8090";
const keyId = process.env.PAYGATE_KEY_ID;
const keySecret = process.env.PAYGATE_KEY_SECRET;
const port = Number(process.env.PORT || 3100);

if (!keyId || !keySecret) {
  console.error("Set PAYGATE_KEY_ID and PAYGATE_KEY_SECRET");
  process.exit(1);
}

const client = createClient({ baseUrl, keyId, keySecret });

const server = http.createServer(async (req, res) => {
  if (req.method === "GET" && req.url === "/checkout") {
    const order = await client.createOrder({
      amount: 4200,
      currency: "INR",
      receipt: `node-checkout-${Date.now()}`,
    }, `node-checkout-${Date.now()}`);

    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(`<!doctype html>
<html><body>
  <h1>PayGate Hosted Checkout Example</h1>
  <p>Order created: <code>${order.id}</code></p>
  <p>Use this order id with the dashboard or API to continue the sandbox flow.</p>
</body></html>`);
    return;
  }

  res.writeHead(404, { "Content-Type": "text/plain" });
  res.end("Not found");
});

server.listen(port, () => {
  console.log(`Hosted checkout example listening on http://127.0.0.1:${port}/checkout`);
});
