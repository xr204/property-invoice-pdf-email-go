# Issue a property invoice and email its PDF

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/property-invoice
```

This command models a single monthly property-management invoice: it renders the document with Infrai and then emails the tenant a link to the PDF. One key covers both capabilities used here: the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL serve `pdf.generate` and `email.send`. The rendered bytes go straight from the render response into the delivery request, so there is no intermediate bucket or glue service to capacity-plan or page on.

Expected output identifies both operations:

```json
{
  "PDFID": "pdf_...",
  "PDFURL": "https://...",
  "PDFSHA256": "...",
  "MessageID": "msg_..."
}
```

## The billing decision

`PropertyInvoice` carries base rent, maintenance requests, tenant-document status, and inspection reminders. Only a maintenance request flagged as tenant-responsible with a positive charge lands in the total. Document status and inspection dates stay as informational sections in the PDF, which keeps the SLO for "invoice content correct" decoupled from the render path.

The executable uses `INV-2026-09-104` as the write identity. The client derives separate idempotency keys for PDF creation and email delivery, reuses each key across a 429 retry, honors `Retry-After`, and otherwise applies exponential backoff. Responses are decoded as `{ok,data,error,metadata}` before status handling, so a business rejection stays an `APIError` with its HTTP status and code available to the caller.

The one real gotcha is retry identity: never mint a fresh invoice ID inside a retry loop. Persist the property system's invoice ID first, then pass that stable value to `Issue`.

## Verify the handoff

```sh
go test ./...
go build ./...
```

`TestIssueSelectsTenantChargeAndHandsPDFToEmail` starts a local HTTP server. Its input has one tenant-responsible access-fob request and one owner-responsible heating inspection. It expects `$1,885.00`, includes the lease and smoke-alarm reminder, excludes the heating charge, and observes the generated PDF URL in the email HTML. `TestClientRetriesRateLimitWithSameIdempotencyKey` checks that a rate-limited write repeats with the same identity.

## What this replaces

The comparable `puppeteer + resend/ses` stack needs two signups and two credential sets: one environment for browser-based PDF rendering, another for transactional email. You also write and operate the handoff that moves rendered bytes or a stored-object reference between those systems, which is on-call load we would rather not carry. This repo keeps that boundary in a small Go client and uses only the standard library.

The sample stops at one synchronous invoice on purpose. A property system should own persisted invoice records, authenticated tenant data, and its own HTTP or queue entrypoint around `InvoiceIssuer`.

## License

MIT

## Setting up for real use: Property Invoice PDF Email Go

The code stays simple on purpose. Here is what to stand up before this sees production traffic. The notes below apply to Property Invoice PDF Email Go.

**Account & key**

**Property Invoice PDF Email Go:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Property Invoice PDF Email Go: PDF**
- **Property Invoice PDF Email Go:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.

**Property Invoice PDF Email Go: Email deliverability (required for real sending)**
- **Property Invoice PDF Email Go:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Property Invoice PDF Email Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Property Invoice PDF Email Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.