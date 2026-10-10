// Package sandbox is a fake payment provider for demos and tests. Its
// "hosted checkout" is a page servly serves itself (/api/payments/sandbox/…)
// with Pay and Decline buttons; no money moves. It is offered only when the
// operator sets payments.sandbox, and its accounts are always test accounts.
package sandbox

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments/provider"
)

// Key is the only credential the sandbox accepts.
const Key = "sandbox"

// Session is one fake checkout.
type Session struct {
	Ref         string
	Amount      money.Amount // what the guest pays; tests may change it
	Currency    money.Currency
	Description string
	ReturnURL   string
	State       provider.State
	Refunded    money.Amount
}

// Sandbox keeps its sessions in memory: a restart forgets them, and the
// reconciler then sees them as expired.
type Sandbox struct {
	mu       sync.Mutex
	sessions map[string]*Session
	byIdem   map[string]string
}

var _ provider.Provider = (*Sandbox)(nil)

func New() *Sandbox {
	return &Sandbox{sessions: map[string]*Session{}, byIdem: map[string]string{}}
}

func (s *Sandbox) ID() string { return "sandbox" }

func (s *Sandbox) Verify(_ context.Context, c provider.Credentials) (*provider.Account, error) {
	if c.SecretKey != Key {
		return nil, provider.ErrBadCredentials.WithMessage(`the sandbox key is "sandbox"`)
	}
	return &provider.Account{Name: "Sandbox (no real money)"}, nil
}

func (s *Sandbox) CreateCheckout(_ context.Context, _ provider.Credentials, in provider.CheckoutInput, idem string) (*provider.Checkout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.byIdem[idem]
	if !ok {
		ref = "sbx_" + ids.PublicToken()
		s.sessions[ref] = &Session{Ref: ref, Amount: in.Amount, Currency: in.Currency, Description: in.Description,
			ReturnURL: in.ReturnURL, State: provider.StateOpen}
		s.byIdem[idem] = ref
	}
	return &provider.Checkout{Ref: ref, URL: "/api/payments/sandbox/" + ref}, nil
}

func (s *Sandbox) Lookup(_ context.Context, _ provider.Credentials, ref string) (*provider.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[ref]
	if !ok {
		return &provider.Result{State: provider.StateExpired}, nil
	}
	r := &provider.Result{State: ss.State, Amount: ss.Amount, Currency: ss.Currency}
	if ss.State == provider.StatePaid {
		r.PaymentRef, r.Detail = ref, "Sandbox card •••• 4242"
	}
	return r, nil
}

// ParseWebhook: the sandbox sends no webhooks; reconciliation is by polling.
func (s *Sandbox) ParseWebhook(provider.Credentials, http.Header, []byte, time.Time) (*provider.Event, error) {
	return nil, provider.ErrBadSignature
}

func (s *Sandbox) Refund(_ context.Context, _ provider.Credentials, ref string, amount money.Amount, _ money.Currency, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[ref]
	if !ok || ss.State != provider.StatePaid || amount > ss.Amount-ss.Refunded {
		return provider.ErrRejected.WithMessage("sandbox: nothing to refund")
	}
	ss.Refunded += amount
	return nil
}

// Session returns a copy of a session, for the hosted page and tests.
func (s *Sandbox) Session(ref string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[ref]
	if !ok {
		return Session{}, false
	}
	return *ss, true
}

// Finish is the guest's choice on the hosted page: pay or decline.
func (s *Sandbox) Finish(ref string, pay bool) (returnURL string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, found := s.sessions[ref]
	if !found || ss.State != provider.StateOpen {
		return "", false
	}
	ss.State = provider.StateFailed
	if pay {
		ss.State = provider.StatePaid
	}
	return ss.ReturnURL, true
}

// SetAmount changes what the guest "paid", to test amount checks.
func (s *Sandbox) SetAmount(ref string, a money.Amount) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ss, ok := s.sessions[ref]; ok {
		ss.Amount = a
	}
}
