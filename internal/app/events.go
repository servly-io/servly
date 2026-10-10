package app

import "github.com/servly-io/servly/internal/notifications"

const (
	EventOrderCreated = "order.created"
	EventOrderUpdated = "order.updated"
)

// OrderEvent is published after an order change commits.
type OrderEvent = notifications.Event

// publish notifies staff screens; only ever called after commit.
func (a *App) publish(ev OrderEvent) {
	if a.Events != nil {
		a.Events.Publish(ev)
	}
}
