package app

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/dining"
	"github.com/servly-io/servly/internal/dining/diningsql"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
)

func (a *App) ListDiningLocations(ctx context.Context, p *Principal) ([]dining.Location, error) {
	return diningsql.New(a.DB).List(ctx, p.TenantID(), p.Location.ID)
}

func (a *App) GetDiningLocation(ctx context.Context, p *Principal, id uuid.UUID) (*dining.Location, error) {
	return diningsql.New(a.DB).Get(ctx, p.TenantID(), id)
}

func (a *App) CreateDiningLocation(ctx context.Context, p *Principal, name string, kind dining.Kind) (*dining.Location, error) {
	now := database.Now()
	l := &dining.Location{ID: ids.New(), TenantID: p.TenantID(), LocationID: p.Location.ID, Name: strings.TrimSpace(name),
		Kind: kind, PublicToken: ids.PublicToken(), IsEnabled: true, CreatedAt: now, UpdatedAt: now}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		if err := diningsql.New(tx).Create(ctx, l); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "dining_location.created", "dining_location", l.ID, map[string]any{"name": l.Name, "kind": l.Kind})
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// DiningLocationInput patches a dining location; nil fields are unchanged.
// RotateToken issues a new public token: printed QR codes stop working.
type DiningLocationInput struct {
	Name        *string
	IsEnabled   *bool
	RotateToken bool
}

func (a *App) UpdateDiningLocation(ctx context.Context, p *Principal, id uuid.UUID, in DiningLocationInput) (*dining.Location, error) {
	var l *dining.Location
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := diningsql.New(tx)
		var err error
		if l, err = st.Get(ctx, p.TenantID(), id); err != nil {
			return err
		}
		data := map[string]any{}
		if in.Name != nil {
			l.Name = strings.TrimSpace(*in.Name)
			data["name"] = l.Name
		}
		if in.IsEnabled != nil {
			l.IsEnabled = *in.IsEnabled
			data["is_enabled"] = l.IsEnabled
		}
		if in.RotateToken {
			l.PublicToken = ids.PublicToken()
			data["token_rotated"] = true
		}
		if err := l.Validate(); err != nil {
			return err
		}
		l.UpdatedAt = database.Now()
		if err := st.Update(ctx, l); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "dining_location.updated", "dining_location", l.ID, data)
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}
