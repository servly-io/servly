package payments

// The market catalog behind the setup wizard: per market, the methods guests
// use most (first = most common) and the providers that serve them, the
// recommended one first. It mirrors docs/docs/payments.md; keep the two in
// step. Providers servly has no adapter for yet are listed so owners see the
// road ahead; the HTTP layer marks which ones can be connected today.

// Market is a country or region with its everyday payment methods.
type Market struct {
	Code       string            `json:"code"` // ISO country code, or EU
	Name       string            `json:"name"`
	Currencies []string          `json:"currencies"`
	Methods    []string          `json:"methods"`
	Providers  []CatalogProvider `json:"providers"`
}

// CatalogProvider is one provider's offer in a market.
type CatalogProvider struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Recommended bool     `json:"recommended"`
	Methods     []string `json:"methods"`
	Note        string   `json:"note,omitempty"`
}

var Catalog = []Market{
	{Code: "ID", Name: "Indonesia", Currencies: []string{"IDR"},
		Methods: []string{"QRIS", "GoPay, OVO, DANA, ShopeePay", "Bank transfer (virtual account)", "Cards"},
		Providers: []CatalogProvider{
			{ID: "xendit", Name: "Xendit", Recommended: true, Methods: []string{"QRIS", "OVO, DANA, ShopeePay, LinkAja, GoPay", "Virtual accounts", "Cards"}},
			{ID: "midtrans", Name: "Midtrans", Methods: []string{"QRIS", "GoPay, ShopeePay", "Virtual accounts", "Cards"}},
		}},
	{Code: "SG", Name: "Singapore", Currencies: []string{"SGD"},
		Methods: []string{"Cards with Apple Pay and Google Pay", "PayNow", "GrabPay"},
		Providers: []CatalogProvider{
			{ID: "stripe", Name: "Stripe", Recommended: true, Methods: []string{"Cards", "Apple Pay", "Google Pay", "PayNow", "GrabPay"}},
			{ID: "hitpay", Name: "HitPay", Methods: []string{"PayNow", "Cards", "GrabPay", "ShopeePay"}},
		}},
	{Code: "MY", Name: "Malaysia", Currencies: []string{"MYR"},
		Methods: []string{"DuitNow QR", "Touch 'n Go, GrabPay, ShopeePay, Boost", "Cards with Apple Pay and Google Pay", "FPX"},
		Providers: []CatalogProvider{
			{ID: "hitpay", Name: "HitPay", Recommended: true, Methods: []string{"DuitNow QR", "FPX", "Touch 'n Go, GrabPay, ShopeePay, Boost", "Cards"}},
			{ID: "stripe", Name: "Stripe", Methods: []string{"Cards", "Apple Pay", "Google Pay", "FPX", "GrabPay"}, Note: "No DuitNow QR."},
			{ID: "xendit", Name: "Xendit", Methods: []string{"DuitNow QR", "FPX", "E-wallets", "Cards"}},
		}},
	{Code: "EU", Name: "European Union", Currencies: []string{"EUR", "PLN", "SEK", "DKK", "CZK", "RON", "HUF"},
		Methods: []string{"Cards with Apple Pay and Google Pay", "iDEAL | Wero, Bancontact, BLIK and other local methods", "PayPal", "Klarna"},
		Providers: []CatalogProvider{
			{ID: "stripe", Name: "Stripe", Recommended: true, Methods: []string{"Cards", "Apple Pay", "Google Pay", "iDEAL | Wero", "Bancontact", "BLIK", "EPS", "PayPal", "Klarna"}},
			{ID: "mollie", Name: "Mollie", Methods: []string{"Cards", "iDEAL | Wero", "Bancontact", "PayPal", "Klarna"}},
			{ID: "sumup", Name: "SumUp", Methods: []string{"Card readers"}},
		}},
	{Code: "US", Name: "United States", Currencies: []string{"USD"},
		Methods: []string{"Cards with Apple Pay and Google Pay", "PayPal and Venmo", "Cash App Pay"},
		Providers: []CatalogProvider{
			{ID: "stripe", Name: "Stripe", Recommended: true, Methods: []string{"Cards", "Apple Pay", "Google Pay", "Cash App Pay", "Link"}},
			{ID: "square", Name: "Square", Methods: []string{"Cards", "Apple Pay", "Google Pay", "Cash App Pay"}},
			{ID: "paypal", Name: "PayPal", Methods: []string{"PayPal", "Venmo"}},
		}},
	{Code: "OTHER", Name: "Another country", Currencies: nil,
		Methods: []string{"Cards with Apple Pay and Google Pay"},
		Providers: []CatalogProvider{
			{ID: "stripe", Name: "Stripe", Recommended: true, Methods: []string{"Cards", "Apple Pay", "Google Pay"}, Note: "Check that Stripe serves your country at stripe.com/global."},
		}},
}
