// Package notifications is the process-local broadcaster behind the staff
// SSE stream. It is a notification mechanism, not a queue: events are not
// stored, a slow subscriber drops events, and clients refetch state from the
// database. See MASTER_PLAN.md §11.
package notifications

import (
	"sync"

	"github.com/google/uuid"
)

// Event says that an order changed; it carries ids only.
type Event struct {
	Kind       string // "order.created" | "order.updated"
	TenantID   uuid.UUID
	LocationID uuid.UUID
	OrderID    uuid.UUID
	Version    int64
}

// Broker fans events out to subscribers of one tenant location.
type Broker struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

func NewBroker() *Broker { return &Broker{subs: map[*Subscription]struct{}{}} }

// Subscription receives the events of one tenant location.
type Subscription struct {
	C          chan Event
	tenantID   uuid.UUID
	locationID uuid.UUID
	// Dropped is set when an event could not be delivered; the client must
	// refetch everything.
	dropped bool
}

const buffer = 64

// Subscribe registers a subscriber for a tenant location. Call Unsubscribe
// when done.
func (b *Broker) Subscribe(tenantID, locationID uuid.UUID) *Subscription {
	s := &Subscription{C: make(chan Event, buffer), tenantID: tenantID, locationID: locationID}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

func (b *Broker) Unsubscribe(s *Subscription) {
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
}

// Publish delivers ev to the subscribers of its tenant location without
// blocking; a full subscriber misses the event and is marked dropped.
func (b *Broker) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if s.tenantID != ev.TenantID || s.locationID != ev.LocationID {
			continue
		}
		select {
		case s.C <- ev:
		default:
			s.dropped = true
		}
	}
}

// TakeDropped reports and clears whether events were missed.
func (b *Broker) TakeDropped(s *Subscription) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := s.dropped
	s.dropped = false
	return d
}

// Count is the number of subscribers, for tests and metrics.
func (b *Broker) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
