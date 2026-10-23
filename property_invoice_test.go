package invoice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIssueSelectsTenantChargeAndHandsPDFToEmail(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing bearer auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/pdf/generate":
			html := body["html"].(string)
			for _, want := range []string{"Replacement access fob", "$1,885.00", "Lease renewal", "Annual smoke alarm"} {
				if !strings.Contains(html, want) {
					t.Errorf("invoice missing %q", want)
				}
			}
			if strings.Contains(html, "Heating inspection") {
				t.Error("owner maintenance was billed")
			}
			if r.Header.Get("Idempotency-Key") != "INV-104:pdf" {
				t.Error("wrong PDF idempotency key")
			}
			_, _ = w.Write([]byte(`{"ok":true,"data":{"url":"https://documents.example/INV-104.pdf","pdf_id":"pdf_104","sha256":"abc"},"metadata":{}}`))
		case "/v1/email/send":
			if !strings.Contains(body["html"].(string), "https://documents.example/INV-104.pdf") {
				t.Error("PDF URL was not handed to email")
			}
			if _, present := body["from"]; present {
				t.Error("default sender should be used")
			}
			if r.Header.Get("Idempotency-Key") != "INV-104:email" {
				t.Error("wrong email idempotency key")
			}
			_, _ = w.Write([]byte(`{"ok":true,"data":{"message_id":"msg_104"},"metadata":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	issuer := InvoiceIssuer{Infrai: &Client{BaseURL: server.URL, APIKey: "test-key", HTTP: server.Client()}}
	got, err := issuer.Issue(context.Background(), PropertyInvoice{InvoiceID: "INV-104", TenantName: "Mina", TenantEmail: "mina@example.com", PropertyAddress: "18 Harbor Street", PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), BaseRentCents: 185000, Maintenance: []MaintenanceRequest{{"MR-1", "Replacement access fob", 3500, true}, {"MR-2", "Heating inspection", 9900, false}}, Documents: []TenantDocument{{"Lease renewal", "received"}}, Inspections: []InspectionReminder{{"Annual smoke alarm", time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || got.MessageID != "msg_104" || got.PDFID != "pdf_104" {
		t.Fatalf("delivery = %#v, calls = %d", got, calls)
	}
}

func TestClientRetriesRateLimitWithSameIdempotencyKey(t *testing.T) {
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		if len(keys) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"rate_limited","message":"retry later"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{"message_id":"msg_retry"}}`))
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, APIKey: "test-key", HTTP: server.Client(), MaxRetries: 1, Sleep: func(context.Context, time.Duration) error { return nil }}
	got, err := c.SendEmail(context.Background(), "tenant@example.com", "Invoice", "<p>Ready</p>", "INV-9:email")
	if err != nil {
		t.Fatal(err)
	}
	if got.MessageID != "msg_retry" || len(keys) != 2 || keys[0] != keys[1] {
		t.Fatalf("message = %q, keys = %#v", got.MessageID, keys)
	}
}
