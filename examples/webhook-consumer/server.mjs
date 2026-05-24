import http from "node:http";
import { verifyWebhookSignature } from "../../sdk/js/paygate.js";

const port = Number(process.env.PORT || 3200);
const webhookSecret = process.env.PAYGATE_WEBHOOK_SECRET || "secret";

const server = http.createServer(async (req, res) => {
  if (req.method !== "POST" || req.url !== "/webhooks/paygate") {
    res.writeHead(404, { "Content-Type": "text/plain" });
    res.end("Not found");
    return;
  }

  const chunks = [];
  for await (const chunk of req) {
    chunks.push(chunk);
  }
  const bodyText = Buffer.concat(chunks).toString("utf8");
  const signature = req.headers["x-paygate-signature-v1"] || "";
  const valid = await verifyWebhookSignature({
    secret: webhookSecret,
    bodyText,
    signature: Array.isArray(signature) ? signature[0] : signature,
  });

  if (!valid) {
    res.writeHead(400, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ ok: false, error: "invalid_signature" }));
    return;
  }

  console.log("received webhook", bodyText);
  res.writeHead(200, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ ok: true }));
});

server.listen(port, () => {
  console.log(`Webhook consumer listening on http://127.0.0.1:${port}/webhooks/paygate`);
});
