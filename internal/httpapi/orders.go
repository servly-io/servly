package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/orders"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/tenant"
)

func (s *server) orderRoutes(r chi.Router) {
	view := func(h http.HandlerFunc) http.HandlerFunc { return s.can(tenant.PermViewOrders, h) }
	record := func(h http.HandlerFunc) http.HandlerFunc { return s.can(tenant.PermRecordPayment, h) }
	r.Get("/orders", view(s.listOrders))
	r.Get("/orders/{id}", view(s.getOrder))
	// The finer cancel permission is checked by the domain rule.
	r.Patch("/orders/{id}/status", s.can(tenant.PermUpdateOrderStatus, s.changeStatus))
	r.Post("/orders/{id}/payments", record(s.recordPayment))
	r.Patch("/payments/{id}", record(s.settlePayment))
	r.Post("/payments/{id}/refund", s.can(tenant.PermRefundPayment, s.refundPayment))
	r.Get("/events", view(s.events))
}

type diningRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type staffOrderJSON struct {
	ID             uuid.UUID       `json:"id"`
	Number         int             `json:"number"`
	BusinessDate   string          `json:"business_date"`
	Mode           orders.Mode     `json:"mode"`
	DiningLocation *diningRef      `json:"dining_location"`
	Status         orders.Status   `json:"status"`
	PaymentStatus  payments.Status `json:"payment_status"`
	Version        int64           `json:"version"`
	Currency       money.Currency  `json:"currency"`
	Subtotal       money.Amount    `json:"subtotal_minor"`
	Total          money.Amount    `json:"total_minor"`
	Notes          string          `json:"notes"`
	Items          []lineJSON      `json:"items"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func staffOrderView(o *orders.Order, names map[uuid.UUID]string) staffOrderJSON {
	v := staffOrderJSON{ID: o.ID, Number: o.Number, BusinessDate: o.BusinessDate, Mode: o.Mode, Status: o.Status,
		PaymentStatus: o.PaymentStatus, Version: o.Version, Currency: o.Currency, Subtotal: o.Subtotal, Total: o.Total,
		Notes: o.Notes, Items: linesView(o.Lines), CreatedAt: o.CreatedAt.Time, UpdatedAt: o.UpdatedAt.Time}
	if o.DiningLocationID.Valid {
		v.DiningLocation = &diningRef{ID: o.DiningLocationID.UUID, Name: names[o.DiningLocationID.UUID]}
	}
	return v
}

type actorJSON struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type eventJSON struct {
	From      *orders.Status `json:"from"`
	To        orders.Status  `json:"to"`
	Actor     *actorJSON     `json:"actor"`
	Reason    string         `json:"reason"`
	CreatedAt time.Time      `json:"created_at"`
}

type paymentJSON struct {
	ID         uuid.UUID       `json:"id"`
	Method     payments.Method `json:"method"`
	Status     payments.Status `json:"status"`
	Amount     money.Amount    `json:"amount_minor"`
	Refunded   money.Amount    `json:"refunded_minor"`
	Currency   money.Currency  `json:"currency"`
	Reference  string          `json:"reference"`
	RecordedBy *actorJSON      `json:"recorded_by"`
	CreatedAt  time.Time       `json:"created_at"`
}

type orderDetailJSON struct {
	staffOrderJSON
	Outstanding money.Amount  `json:"outstanding_minor"`
	Events      []eventJSON   `json:"events"`
	Payments    []paymentJSON `json:"payments"`
}

func actor(id uuid.NullUUID, names map[uuid.UUID]string) *actorJSON {
	if !id.Valid {
		return nil
	}
	return &actorJSON{ID: id.UUID, Name: names[id.UUID]}
}

func (s *server) detailView(r *http.Request, d *app.OrderDetail) (orderDetailJSON, error) {
	names, err := s.App.DiningNames(r.Context(), principal(r))
	if err != nil {
		return orderDetailJSON{}, err
	}
	v := orderDetailJSON{staffOrderJSON: staffOrderView(d.Order, names), Outstanding: d.Outstanding,
		Events: []eventJSON{}, Payments: []paymentJSON{}}
	for _, e := range d.Events {
		ev := eventJSON{To: e.To, Actor: actor(e.ActorUserID, d.Actors), Reason: e.Reason, CreatedAt: e.CreatedAt.Time}
		if e.From != "" {
			from := e.From
			ev.From = &from
		}
		v.Events = append(v.Events, ev)
	}
	for _, p := range d.Payments {
		v.Payments = append(v.Payments, paymentJSON{ID: p.ID, Method: p.Method, Status: p.Status, Amount: p.Amount,
			Refunded: p.Refunded, Currency: p.Currency, Reference: p.Reference, RecordedBy: actor(p.RecordedByUserID, d.Actors),
			CreatedAt: p.CreatedAt.Time})
	}
	return v, nil
}

func (s *server) writeDetail(w http.ResponseWriter, r *http.Request, d *app.OrderDetail, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.detailView(r, d)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) listOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := orders.ListFilter{Mode: orders.Mode(q.Get("mode")), Cursor: q.Get("cursor")}
	for _, st := range q["status"] {
		f.Statuses = append(f.Statuses, orders.Status(st))
	}
	if f.Mode != "" && f.Mode != orders.ModeDineIn && f.Mode != orders.ModePickup {
		s.fail(w, r, errMalformedJSON.WithMessage("mode must be dine_in or pickup"))
		return
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 200 {
			s.fail(w, r, errMalformedJSON.WithMessage("limit must be 1-200"))
			return
		}
		f.Limit = n
	}
	list, next, err := s.App.ListOrders(r.Context(), principal(r), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.App.DiningNames(r.Context(), principal(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]staffOrderJSON, 0, len(list))
	for i := range list {
		items = append(items, staffOrderView(&list[i], names))
	}
	var nc *string
	if next != "" {
		nc = &next
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nc})
}

func (s *server) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.App.GetOrder(r.Context(), principal(r), id)
	s.writeDetail(w, r, d, err)
}

func (s *server) changeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Status  orders.Status `json:"status"`
		Version int64         `json:"version"`
		Reason  string        `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.App.ChangeOrderStatus(r.Context(), principal(r), id, req.Status, req.Version, req.Reason)
	s.writeDetail(w, r, d, err)
}

func (s *server) recordPayment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Method    payments.Method `json:"method"`
		Status    payments.Status `json:"status"`
		Amount    money.Amount    `json:"amount_minor"`
		Reference string          `json:"reference"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.App.RecordPayment(r.Context(), principal(r), id, app.PaymentInput{Method: req.Method, Status: req.Status, Amount: req.Amount, Reference: req.Reference})
	s.writeDetail(w, r, d, err)
}

func (s *server) settlePayment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Status payments.Status `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.App.SettlePayment(r.Context(), principal(r), id, req.Status)
	s.writeDetail(w, r, d, err)
}

func (s *server) refundPayment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Amount money.Amount `json:"amount_minor"`
		Reason string       `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.App.RefundPayment(r.Context(), principal(r), id, req.Amount, req.Reason)
	s.writeDetail(w, r, d, err)
}

// sseKeepalive keeps proxies and the browser from closing an idle stream.
var sseKeepalive = 25 * time.Second

// events streams order notifications for the active tenant and location.
// Events carry ids only; clients fetch state. After "ready", and after a
// "resync", clients refetch everything.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, fmt.Errorf("streaming unsupported"))
		return
	}
	p := principal(r)
	sub := s.App.Events.Subscribe(p.TenantID(), p.Location.ID)
	defer s.App.Events.Unsubscribe(sub)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 3000\nevent: ready\ndata: {}\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(sseKeepalive)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.Stop:
			return
		case ev := <-sub.C:
			if s.App.Events.TakeDropped(sub) {
				_, _ = fmt.Fprint(w, "event: resync\ndata: {}\n\n")
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: {\"order_id\":%q,\"version\":%d}\n\n", ev.Kind, ev.OrderID, ev.Version)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
