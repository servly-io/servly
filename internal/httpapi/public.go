package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/dining"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/orders"
	"github.com/servly-io/servly/internal/payments"
)

// publicLimits rate-limit the guest API (MASTER_PLAN.md §12).
type publicLimits struct {
	menuPerIP, ordersPerIP, ordersPerTable, statusPerIP *limiter
}

func newPublicLimits() publicLimits {
	return publicLimits{
		menuPerIP:      newLimiter(time.Second, 60),
		ordersPerIP:    newLimiter(6*time.Second, 10),
		ordersPerTable: newLimiter(2*time.Second, 30),
		statusPerIP:    newLimiter(time.Second, 60),
	}
}

// publicRoutes registers the guest API: no session, the token in the path or
// body is the access check.
func (s *server) publicRoutes(r chi.Router) {
	r.Get("/assets/{id}", s.publicAsset)
	r.Get("/menu/{token}", s.publicMenu)
	r.Post("/orders", s.placeOrder)
	r.Post("/orders/quote", s.quoteOrder)
	r.Get("/orders/{token}", s.publicOrder)
}

type publicOptionJSON struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	PriceDelta  money.Amount `json:"price_delta_minor"`
	IsAvailable bool         `json:"available"`
}

type publicGroupJSON struct {
	ID        uuid.UUID          `json:"id"`
	Name      string             `json:"name"`
	MinSelect int                `json:"min_select"`
	MaxSelect int                `json:"max_select"`
	Options   []publicOptionJSON `json:"options"`
}

type publicItemJSON struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Price        money.Amount      `json:"price_minor"`
	ImageURL     *string           `json:"image_url"`
	Available    bool              `json:"available"`
	OptionGroups []publicGroupJSON `json:"option_groups"`
}

type publicCategoryJSON struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Items       []publicItemJSON `json:"items"`
}

type publicMenuJSON struct {
	Restaurant struct {
		Name              string         `json:"name"`
		Currency          money.Currency `json:"currency"`
		OrderNotesEnabled bool           `json:"order_notes_enabled"`
		AcceptingOrders   bool           `json:"accepting_orders"`
	} `json:"restaurant"`
	DiningLocation struct {
		Name string      `json:"name"`
		Kind dining.Kind `json:"kind"`
	} `json:"dining_location"`
	Categories []publicCategoryJSON `json:"categories"`
	// Payment: how guests here can pay online: none, before (pay to order),
	// after (order, then pay from the phone) or either.
	Payment app.GuestPayment `json:"payment"`
}

func (s *server) publicMenu(w http.ResponseWriter, r *http.Request) {
	if !s.public.menuPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.menuPerIP)
		return
	}
	pm, err := s.App.GetPublicMenu(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var v publicMenuJSON
	v.Restaurant.Name, v.Restaurant.Currency, v.Restaurant.OrderNotesEnabled = pm.Location.Name, pm.Location.Currency, pm.Location.OrderNotesEnabled
	v.Restaurant.AcceptingOrders = pm.Location.AcceptingOrders
	v.DiningLocation.Name, v.DiningLocation.Kind = pm.Dining.Name, pm.Dining.Kind
	v.Payment = pm.Payment
	v.Categories = []publicCategoryJSON{}
	for _, c := range pm.Categories {
		cv := publicCategoryJSON{ID: c.ID, Name: c.Name, Description: c.Description, Items: []publicItemJSON{}}
		for _, it := range c.Items {
			iv := publicItemJSON{ID: it.ID, Name: it.Name, Description: it.Description, Price: it.Price,
				Available: it.IsAvailable, OptionGroups: []publicGroupJSON{}}
			if it.ImageAssetID.Valid {
				u := assetURL(it.ImageAssetID.UUID)
				iv.ImageURL = &u
			}
			for _, g := range it.Groups {
				gv := publicGroupJSON{ID: g.ID, Name: g.Name, MinSelect: g.MinSelect, MaxSelect: g.MaxSelect, Options: []publicOptionJSON{}}
				for _, o := range g.Options {
					gv.Options = append(gv.Options, publicOptionJSON{ID: o.ID, Name: o.Name, PriceDelta: o.PriceDelta, IsAvailable: o.IsAvailable})
				}
				iv.OptionGroups = append(iv.OptionGroups, gv)
			}
			cv.Items = append(cv.Items, iv)
		}
		v.Categories = append(v.Categories, cv)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

type orderLineRequest struct {
	MenuItemID uuid.UUID   `json:"menu_item_id"`
	Quantity   int         `json:"quantity"`
	OptionIDs  []uuid.UUID `json:"option_ids"`
	Notes      string      `json:"notes"`
}

type placeOrderRequest struct {
	DiningLocationToken string             `json:"dining_location_token"`
	Notes               string             `json:"notes"`
	Items               []orderLineRequest `json:"items"`
}

func (s *server) placeOrder(w http.ResponseWriter, r *http.Request) {
	if !s.public.ordersPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.ordersPerIP)
		return
	}
	var req placeOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.public.ordersPerTable.allow(req.DiningLocationToken) {
		s.limited(w, r, s.public.ordersPerTable)
		return
	}
	// The decoded request re-encoded is canonical: same content, same hash.
	canon, _ := json.Marshal(req)
	sum := sha256.Sum256(canon)
	in := app.PlaceOrderInput{
		DiningToken:    req.DiningLocationToken,
		Notes:          req.Notes,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		RequestHash:    hex.EncodeToString(sum[:]),
		Lines:          req.lines(),
	}
	res, err := s.App.PlaceOrder(r.Context(), in, s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, publicOrderView(res))
}

// quoteOrder prices a cart without placing it.
func (s *server) quoteOrder(w http.ResponseWriter, r *http.Request) {
	if !s.public.menuPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.menuPerIP)
		return
	}
	var req placeOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.App.QuoteOrder(r.Context(), req.DiningLocationToken, req.Notes, req.lines())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := publicOrderView(res)
	writeJSON(w, http.StatusOK, quoteJSON{Mode: v.Mode, DiningLocation: v.DiningLocation, Currency: v.Currency,
		Subtotal: v.Subtotal, Total: v.Total, Items: v.Items})
}

type quoteJSON struct {
	Mode           orders.Mode    `json:"mode"`
	DiningLocation string         `json:"dining_location"`
	Currency       money.Currency `json:"currency"`
	Subtotal       money.Amount   `json:"subtotal_minor"`
	Total          money.Amount   `json:"total_minor"`
	Items          []lineJSON     `json:"items"`
}

func (req placeOrderRequest) lines() []orders.LineRequest {
	out := make([]orders.LineRequest, 0, len(req.Items))
	for _, l := range req.Items {
		out = append(out, orders.LineRequest{MenuItemID: l.MenuItemID, Quantity: l.Quantity, OptionIDs: l.OptionIDs, Notes: l.Notes})
	}
	return out
}

func (s *server) publicOrder(w http.ResponseWriter, r *http.Request) {
	if !s.public.statusPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.statusPerIP)
		return
	}
	res, err := s.App.GetPublicOrder(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, publicOrderView(res))
}

type lineOptionJSON struct {
	GroupName  string       `json:"group_name"`
	OptionName string       `json:"option_name"`
	PriceDelta money.Amount `json:"price_delta_minor"`
}

type lineJSON struct {
	Name         string           `json:"name"`
	Quantity     int              `json:"quantity"`
	UnitPrice    money.Amount     `json:"unit_price_minor"`
	OptionsTotal money.Amount     `json:"options_total_minor"`
	LineTotal    money.Amount     `json:"line_total_minor"`
	Notes        string           `json:"notes"`
	Options      []lineOptionJSON `json:"options"`
}

// publicOrderJSON is what a guest may see of an order: no internal ids, no
// staff, no payment details beyond the status.
type publicOrderJSON struct {
	OrderToken     string          `json:"order_token"`
	Number         int             `json:"number"`
	Status         orders.Status   `json:"status"`
	PaymentStatus  payments.Status `json:"payment_status"`
	Mode           orders.Mode     `json:"mode"`
	DiningLocation string          `json:"dining_location"`
	Currency       money.Currency  `json:"currency"`
	Subtotal       money.Amount    `json:"subtotal_minor"`
	Total          money.Amount    `json:"total_minor"`
	Notes          string          `json:"notes"`
	Items          []lineJSON      `json:"items"`
	CreatedAt      time.Time       `json:"created_at"`
	// PayOnline: the guest may pay this order online now.
	PayOnline bool `json:"pay_online"`
}

func linesView(ls []orders.Line) []lineJSON {
	out := make([]lineJSON, 0, len(ls))
	for _, l := range ls {
		lv := lineJSON{Name: l.Name, Quantity: l.Quantity, UnitPrice: l.UnitPrice, OptionsTotal: l.OptionsTotal,
			LineTotal: l.LineTotal, Notes: l.Notes, Options: []lineOptionJSON{}}
		for _, o := range l.Options {
			lv.Options = append(lv.Options, lineOptionJSON{GroupName: o.GroupName, OptionName: o.OptionName, PriceDelta: o.PriceDelta})
		}
		out = append(out, lv)
	}
	return out
}

func publicOrderView(p *app.PlacedOrder) publicOrderJSON {
	o := p.Order
	return publicOrderJSON{OrderToken: o.PublicToken, Number: o.Number, Status: o.Status, PaymentStatus: o.PaymentStatus,
		Mode: o.Mode, DiningLocation: p.DiningName, Currency: o.Currency, Subtotal: o.Subtotal, Total: o.Total,
		Notes: o.Notes, Items: linesView(o.Lines), CreatedAt: o.CreatedAt.Time, PayOnline: p.PayOnline}
}
