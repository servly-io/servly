// Package menu holds the menu model: categories, items and their option
// groups and options.
//
// Contract: docs/docs/domain-model.md.
package menu

import (
	"context"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
)

type Category struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	LocationID  uuid.UUID
	Name        string
	Description string
	Position    int
	IsEnabled   bool
	CreatedAt   database.Timestamp
	UpdatedAt   database.Timestamp
}

// Item is a product. Its price is in the currency of its location.
type Item struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	CategoryID   uuid.UUID
	Name         string
	Description  string
	Price        money.Amount
	ImageAssetID uuid.NullUUID
	Position     int
	IsEnabled    bool // set by an administrator; disabled items are hidden
	IsAvailable  bool // temporary, such as "sold out today"; shown but not orderable
	// ArchivedAt is set instead of deleting an item that orders reference.
	ArchivedAt database.Timestamp
	Groups     []OptionGroup
	CreatedAt  database.Timestamp
	UpdatedAt  database.Timestamp
}

// Orderable reports whether a customer may order the item now.
func (i *Item) Orderable() bool {
	return i.IsEnabled && i.IsAvailable && i.ArchivedAt.IsZero()
}

// OptionGroup is a choice on an item, such as "Size" (pick exactly 1) or
// "Extras" (pick 0..3).
type OptionGroup struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	ItemID    uuid.UUID
	Name      string
	MinSelect int
	MaxSelect int
	Position  int
	Options   []Option
	CreatedAt database.Timestamp
	UpdatedAt database.Timestamp
}

// Option is one choice within a group. PriceDelta may be negative; the unit
// price of a line must still not be negative.
type Option struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	OptionGroupID uuid.UUID
	Name          string
	PriceDelta    money.Amount
	Position      int
	IsAvailable   bool
	CreatedAt     database.Timestamp
	UpdatedAt     database.Timestamp
}

var (
	ErrNotFound         = apperr.NotFound("menu_not_found", "menu entry not found")
	ErrInvalidPrice     = apperr.Invalid("invalid_price", "price must be between 0 and the maximum amount")
	ErrInvalidSelection = apperr.Invalid("invalid_option_group", "option group needs 0 <= min_select <= max_select and max_select >= 1")
	ErrNameRequired     = apperr.Invalid("name_required", "name must not be empty")
)

// Validate checks the rules of a group that the schema also enforces, so
// they fail early with a useful error.
func (g *OptionGroup) Validate() error {
	if g.Name == "" {
		return ErrNameRequired
	}
	if g.MinSelect < 0 || g.MaxSelect < 1 || g.MaxSelect < g.MinSelect {
		return ErrInvalidSelection
	}
	return nil
}

// Validate checks an item and its groups.
func (i *Item) Validate() error {
	if i.Name == "" {
		return ErrNameRequired
	}
	if i.Price < 0 || i.Price > money.MaxAmount {
		return ErrInvalidPrice
	}
	for k := range i.Groups {
		if err := i.Groups[k].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Repository persists the menu. Every method takes the tenant id; a record of
// another tenant is reported as ErrNotFound.
type Repository interface {
	ListCategories(ctx context.Context, tenantID, locationID uuid.UUID) ([]Category, error)
	CreateCategory(ctx context.Context, c *Category) error
	UpdateCategory(ctx context.Context, c *Category) error

	// ListItems returns items with their option groups and options.
	ListItems(ctx context.Context, tenantID, locationID uuid.UUID) ([]Item, error)
	GetItem(ctx context.Context, tenantID, id uuid.UUID) (*Item, error)
	// CreateItem and UpdateItem write the item together with its groups and
	// options, replacing the previous groups on update.
	CreateItem(ctx context.Context, i *Item) error
	UpdateItem(ctx context.Context, i *Item) error
	// DeleteItem removes an item no order references and archives one that
	// is referenced. It reports which it did.
	DeleteItem(ctx context.Context, tenantID, id uuid.UUID) (archived bool, err error)
}
