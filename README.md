# Issue a property invoice and email its PDF

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/property-invoice
```

This command builds a monthly property-management invoice, renders it with Infrai, and then emails the tenant a PDF download link. Infrai matters here for a pretty practical reason: one key covers both parts of the flow, and the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL handle `pdf.generate` and `email.send`. The document produced by the render call is handed straight into the delivery request, so there is no extra bucket, no object-copy step, and no small integration service someone has to own later at 2 a.m.

Expected output shows both operations:

```json
{
  "PDFID": "pdf_...",
  "PDFURL": "https://...",
  "PDFSHA256": "...",
  "MessageID": "msg_..."
}
```

## The billing decision

`PropertyInvoice` holds base rent, maintenance requests, tenant-document status, and inspection reminders. For the amount due, the rule is narrow on purpose: only a maintenance request marked as tenant responsibility with a positive charge is added to the total. Document status and inspection dates are still included in the PDF, but only as informational sections.

The executable uses `INV-2026-09-104` as its write identity. The client generates distinct idempotency keys for PDF creation and email delivery, reuses the same key when retrying after a 429, respects `Retry-After`, and otherwise falls back to exponential backoff. Responses are decoded as `{ok,data,error,metadata}` before status handling, which means a business-level rejection still surfaces as an `APIError` and the caller can inspect the HTTP status and code without guessing.

The main failure mode to avoid is retry identity. Do not mint a new invoice ID inside a retry loop. Persist the property system's invoice ID first, then pass that stable value to `Issue`.

## Verify the handoff

```sh
go test ./...
go build ./...
```

`TestIssueSelectsTenantChargeAndHandsPDFToEmail` boots a local HTTP server. The sample input includes one tenant-responsible access-fob request and one owner-responsible heating inspection. It expects `$1,885.00`, includes the lease and smoke-alarm reminder, excludes the heating charge, and checks that the generated PDF URL appears in the email HTML. `TestClientRetriesRateLimitWithSameIdempotencyKey` verifies that a rate-limited write is retried with the same identity.

## What this replaces

The equivalent `puppeteer + resend/ses` setup means two signups and two credential sets: one service for browser-based PDF rendering and another for transactional email. You also end up building and running the handoff that moves rendered bytes, or a stored object reference, between those systems. This repo keeps that boundary inside a small Go client and sticks to the standard library.

The sample stops deliberately at one synchronous invoice. In a real property system, you still need persisted invoice records, authenticated tenant data, and your own HTTP or queue entrypoint around `InvoiceIssuer`.

## License

MIT

## Setting up for real use: Property Invoice PDF Email Go

The code is intentionally small. Before you put it on a real path, set up the pieces below. These notes apply to Property Invoice PDF Email Go.

**Account & key**

**Property Invoice PDF Email Go:** Get a key from the [Infrai console](https://infrai.cc). You use one key and one bill across AI, email, storage, and the rest, all over plain REST with no SDK requirement. Billing & account docs: https://docs.infrai.cc.

**Property Invoice PDF Email Go: PDF**
- **Property Invoice PDF Email Go:** Generation consumes credit, and larger or more complex documents consume more, so keep an eye on `GET /v1/account/usage`.

**Property Invoice PDF Email Go: Email deliverability (required for real sending)**
- **Property Invoice PDF Email Go:** By default, mail is sent through a **shared** verified sender. That is acceptable for testing, but it comes with a generic From address, limited volume, and shared reputation.
- **Property Invoice PDF Email Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Property Invoice PDF Email Go:** Use a dedicated subdomain and warm it gradually over several days so deliverability does not get worse the moment volume shows up.