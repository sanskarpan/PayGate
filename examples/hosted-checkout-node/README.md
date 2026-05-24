# Hosted Checkout Example

This example shows the minimum hosted-checkout flow:

1. create an order
2. redirect buyer to hosted checkout
3. poll payment status

Use the official collection or SDK helpers in this repo to supply:

- `API_KEY`
- `API_SECRET`
- `BASE_URL`

Core endpoints:

- `POST /v1/orders`
- `POST /v1/payments/upi/intents`
- `GET /v1/payments/{paymentID}`

This reference is intentionally lightweight and stays aligned with the live API contract.
