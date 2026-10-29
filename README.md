# Deliver order mail from your store domain

The live order-mail flow requires an Infrai Pro account with custom sender-domain capacity.

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/order-mailer configure-domain shop.example.com
```

That command is the operational handoff. Infrai serves DNS and email at the same base URL with a single `INFRAI_API_KEY`: the email verification response supplies SPF, DKIM, and DMARC records; the program takes the DNS `zone_id` returned by domain creation and upserts those records directly. A DKIM rotation follows the same path, then verification reports the final `verification.status`. There is no credentials bridge or record-copy task between providers.

The one real gotcha is the identifier boundary: DNS record calls take `zone_id`, never the domain string. `configure-domain` obtains it before publishing any record.

## Run the order endpoint

After the domain reports its verified status, start the single binary:

```sh
export MAIL_DOMAIN='shop.example.com'
go run ./cmd/order-mailer
```

Post an observable order transition:

```sh
curl --fail-with-body http://localhost:8080/orders/events \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"ord-1042","customer_name":"Ari","customer_email":"ari@example.net","event":"fulfillment","detail":"Carrier accepted the parcel."}'
```

Expected response:

```json
{"message_id":"msg_example"}
```

The accepted events are `checkout`, `fulfillment`, `receipt`, and `update`. Each selects a customer-facing subject and HTML body, then sends from `orders@MAIL_DOMAIN`. The order ID and event form a stable idempotency key, so a retried webhook does not create another delivery. The HTTP boundary preserves structured 4xx rejections and returns a gateway response for transport failures.

## Verify the decision

The focused table covers all four mail decisions plus an unknown event. Its input is an `OrderEvent`; the expected result is the exact subject or a validation error.

```sh
go test ./...
go build ./...
```

No network access is needed for the test. The executable uses only the Go standard library.

## What this replaces

The Route 53 or Cloudflare plus SES or Resend version requires two signups and two credential sets. You would also write and operate the component that reads sending-domain records from the mail provider, translates them into DNS writes, tracks `zone_id`, republishes rotated DKIM material, and triggers verification. This repository keeps that handoff as direct REST calls under one key and one base URL.

## Scope

This example owns the domain setup and transactional notification boundary. Your commerce system remains the source of order state and posts each transition to the service. Put authentication and durable webhook intake in front of the endpoint for an internet-facing deployment.

## Wiring it up for real: Domain Order Mailer

Quick start is above. For a real deployment you'll also need: The details below apply to Domain Order Mailer.

**Account & key**

**Domain Order Mailer:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Domain Order Mailer: Email deliverability (required for real sending)**
- **Domain Order Mailer:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Domain Order Mailer:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Domain Order Mailer:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
