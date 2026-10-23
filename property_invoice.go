package invoice

import (
	"context"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"
)

type MaintenanceRequest struct {
	ID, Summary          string
	ChargeCents          int64
	TenantResponsibility bool
}
type TenantDocument struct{ Name, Status string }
type InspectionReminder struct {
	Kind string
	Due  time.Time
}
type PropertyInvoice struct {
	InvoiceID, TenantName, TenantEmail, PropertyAddress string
	PeriodStart, PeriodEnd                              time.Time
	BaseRentCents                                       int64
	Maintenance                                         []MaintenanceRequest
	Documents                                           []TenantDocument
	Inspections                                         []InspectionReminder
}

type Delivery struct{ PDFID, PDFURL, PDFSHA256, MessageID string }

type InvoiceIssuer struct{ Infrai *Client }

func (i InvoiceIssuer) Issue(ctx context.Context, inv PropertyInvoice) (Delivery, error) {
	if i.Infrai == nil {
		return Delivery{}, fmt.Errorf("infrai client is required")
	}
	html, err := renderInvoice(inv)
	if err != nil {
		return Delivery{}, err
	}
	pdf, err := i.Infrai.GeneratePDF(ctx, html, inv.InvoiceID+":pdf")
	if err != nil {
		return Delivery{}, err
	}
	mail := fmt.Sprintf(`<p>Hello %s,</p><p>Your property invoice is ready.</p><p><a href="%s">Download invoice PDF</a></p><p>Reference: %s</p>`, template.HTMLEscapeString(inv.TenantName), template.HTMLEscapeString(pdf.URL), template.HTMLEscapeString(inv.InvoiceID))
	receipt, err := i.Infrai.SendEmail(ctx, inv.TenantEmail, "Property invoice "+inv.InvoiceID, mail, inv.InvoiceID+":email")
	if err != nil {
		return Delivery{}, err
	}
	return Delivery{pdf.PDFID, pdf.URL, pdf.SHA256, receipt.MessageID}, nil
}

func billableMaintenance(requests []MaintenanceRequest) []MaintenanceRequest {
	result := make([]MaintenanceRequest, 0, len(requests))
	for _, r := range requests {
		if r.TenantResponsibility && r.ChargeCents > 0 {
			result = append(result, r)
		}
	}
	return result
}

func renderInvoice(inv PropertyInvoice) (string, error) {
	const document = `<!doctype html><html><body><h1>Property invoice {{.InvoiceID}}</h1><p>{{.TenantName}} · {{.PropertyAddress}}</p><p>Period: {{date .PeriodStart}} to {{date .PeriodEnd}}</p><h2>Charges</h2><p>Base rent: {{money .BaseRentCents}}</p>{{range .Billable}}<p>Maintenance {{.ID}}: {{.Summary}} — {{money .ChargeCents}}</p>{{end}}<h2>Tenant documents</h2>{{range .Documents}}<p>{{.Name}}: {{.Status}}</p>{{else}}<p>None recorded</p>{{end}}<h2>Inspection reminders</h2>{{range .Inspections}}<p>{{.Kind}}: {{date .Due}}</p>{{else}}<p>None scheduled</p>{{end}}<p>Total: {{money .Total}}</p></body></html>`
	t, err := template.New("invoice").Funcs(template.FuncMap{"date": func(t time.Time) string { return t.Format("2006-01-02") }, "money": formatMoney}).Parse(document)
	if err != nil {
		return "", err
	}
	billable := billableMaintenance(inv.Maintenance)
	total := inv.BaseRentCents
	for _, r := range billable {
		total += r.ChargeCents
	}
	data := struct {
		PropertyInvoice
		Billable []MaintenanceRequest
		Total    int64
	}{inv, billable, total}
	var out strings.Builder
	if err := t.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func formatMoney(cents int64) string {
	whole := strconv.FormatInt(cents/100, 10)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	return fmt.Sprintf("$%s.%02d", whole, cents%100)
}
