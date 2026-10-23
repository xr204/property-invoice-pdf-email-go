package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	invoice "example.com/property-invoice"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	inv := invoice.PropertyInvoice{
		InvoiceID: "INV-2026-09-104", TenantName: "Mina Chen", TenantEmail: "chenhua@changba.com", PropertyAddress: "18 Harbor Street, Unit 4B",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), BaseRentCents: 185000,
		Maintenance: []invoice.MaintenanceRequest{{ID: "MR-441", Summary: "Replacement access fob", ChargeCents: 3500, TenantResponsibility: true}, {ID: "MR-442", Summary: "Heating inspection", ChargeCents: 0, TenantResponsibility: false}},
		Documents:   []invoice.TenantDocument{{Name: "Lease renewal", Status: "received"}},
		Inspections: []invoice.InspectionReminder{{Kind: "Annual smoke alarm", Due: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}},
	}
	result, err := (invoice.InvoiceIssuer{Infrai: &invoice.Client{BaseURL: invoice.DefaultBaseURL, APIKey: key, MaxRetries: 3}}).Issue(context.Background(), inv)
	if err != nil {
		log.Fatal(err)
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
}
