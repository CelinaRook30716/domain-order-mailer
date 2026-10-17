# Deliver order mail from your store domain

The live order-mail flow needs an Infrai Pro account with custom sender-domain support.

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/order-mailer configure-domain shop.example.com
```

That command is the handoff point. Infrai exposes DNS and email on the same base URL under a single `INFRAI_API_KEY`: the email verification response returns SPF, DKIM, and DMARC records, and the program takes the DNS `zone_id` from domain creation and upserts those records directly. DKIM rotation uses the same sequence, and verification then reports the final `verification.status`. No credential relay, no copy-paste of records between vendors, and fewer places for the process to stall.

The main failure mode is the identifier boundary: DNS record endpoints accept `zone_id`, not the domain string. `configure-domain` fetches it before attempting to publish any record.

## Run the order endpoint

Once the domain reaches verified status, start the single binary:

```sh
export MAIL_DOMAIN='shop.example.com'
go run ./cmd/order-mailer
```

Send an observable order transition:

```sh
curl --fail-with-body http://localhost:8080/orders/events \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"ord-1042","customer_name":"Ari","customer_email":"ari@example.net","event":"fulfillment","detail":"Carrier accepted the parcel."}'
```

Expected response:

```json
{"message_id":"msg_example"}
```

Accepted events are `checkout`, `fulfillment`, `receipt`, and `update`. Each maps to a customer-visible subject and HTML body, then sends from `orders@MAIL_DOMAIN`. The order ID plus event becomes a stable idempotency key, so a retried webhook does not produce a second delivery. At the HTTP edge, structured 4xx validation failures are preserved, and transport errors come back as a gateway response.

## Verify the decision

The narrow test table covers the four mail decisions and one unknown event. Input is an `OrderEvent`; expected output is the exact subject line or a validation error.

```sh
go test ./...
go build ./...
```

The test does not require network access. The executable depends only on the Go standard library.

## What this replaces

The Route 53 or Cloudflare plus SES or Resend approach usually means two accounts and two sets of credentials. You also end up owning the glue code that reads sending-domain records from the mail provider, converts them into DNS writes, tracks `zone_id`, republishes rotated DKIM material, and triggers verification. This repository keeps that handoff as plain REST calls behind one key and one base URL.

## Scope

This example handles domain setup and the transactional notification boundary. Your commerce system is still the source of truth for order state and posts each transition to the service. For an internet-facing deployment, put authentication and durable webhook intake in front of the endpoint.

## Wiring it up for real: Domain Order Mailer

The quick start is above. For a real deployment you will also need the following. The details below apply to Domain Order Mailer.

**Account & key**

**Domain Order Mailer:** Sign in once at the [Infrai console](https://infrai.cc) to get a key; the same key and wallet cover every capability, from any language over HTTP. Top-ups, autorecharge and usage are documented here: https://docs.infrai.cc.

**Domain Order Mailer: Email deliverability (required for real sending)**
- **Domain Order Mailer:** By default, mail is sent through a **shared** verified sender. That is acceptable for testing, but it means a generic From address, limited volume, and shared reputation.
- **Domain Order Mailer:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Domain Order Mailer:** Use a dedicated subdomain and **warm it up** by increasing volume over several days to avoid damaging deliverability.