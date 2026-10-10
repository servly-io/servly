// Package audit records privileged and manual changes: who did what to which
// record, and when. Audit rows are written in the same transaction as the
// change they describe.
package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
)

// Event is one audit row.
type Event struct {
	TenantID    uuid.NullUUID // empty for platform-level events
	ActorUserID uuid.NullUUID // empty for the system or an anonymous customer
	Action      string        // dotted, e.g. "menu.item.updated", "payment.recorded"
	EntityType  string
	EntityID    uuid.UUID
	Data        map[string]any
}

// Record writes ev with q, typically a transaction.
func Record(ctx context.Context, q database.Querier, ev Event) error {
	data := []byte("{}")
	if len(ev.Data) > 0 {
		var err error
		if data, err = json.Marshal(ev.Data); err != nil {
			return err
		}
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO audit_events (id, tenant_id, actor_user_id, action, entity_type, entity_id, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ids.New(), ev.TenantID, ev.ActorUserID, ev.Action, ev.EntityType, ev.EntityID, string(data), database.Now())
	return err
}

// Tenant is a helper for the common case of a tenant-scoped event.
func Tenant(tenantID uuid.UUID, actor uuid.UUID) (uuid.NullUUID, uuid.NullUUID) {
	return uuid.NullUUID{UUID: tenantID, Valid: true}, uuid.NullUUID{UUID: actor, Valid: actor != uuid.Nil}
}
