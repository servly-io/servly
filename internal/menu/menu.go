// Package menu holds the menu model: categories, items and their option
// groups and options.
//
// Contract: docs/docs/domain-model.md.
package menu

import (
	"context"
	"strings"
	"unicode/utf8"

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
	ID         uuid.UUID
	TenantID   uuid.UUID
	ItemID     uuid.UUID
	Name       string
	MinSelect  int
	MaxSelect  int
	Position   int
	Options    []Option
	ArchivedAt database.Timestamp // removed from the item; kept for past orders
	CreatedAt  database.Timestamp
	UpdatedAt  database.Timestamp
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
	ArchivedAt    database.Timestamp // removed from the item; kept for past orders
	CreatedAt     database.Timestamp
	UpdatedAt     database.Timestamp
}

// Text limits; the schema does not enforce them, the domain does.
const (
	MaxNameLen        = 120
	MaxDescriptionLen = 1000
	MaxGroupsPerItem  = 20
	MaxOptionsPerItem = 100
)

var (
	ErrNotFound         = apperr.NotFound("menu_not_found", "menu entry not found")
	ErrInvalidPrice     = apperr.Invalid("invalid_price", "price must be between 0 and the maximum amount")
	ErrInvalidSelection = apperr.Invalid("invalid_option_group", "option group needs 0 <= min_select <= max_select and max_select >= 1")
	ErrNameRequired     = apperr.Invalid("name_required", "name must not be empty")
	ErrTextTooLong      = apperr.Invalid("text_too_long", "name or description is too long")
	ErrTooManyOptions   = apperr.Invalid("too_many_options", "an item has at most 20 option groups and 100 options")
	ErrCategoryRequired = apperr.Invalid("category_required", "the item needs an existing category")
)

func checkText(name, description string) error {
	if strings.TrimSpace(name) == "" {
		return ErrNameRequired
	}
	if utf8.RuneCountInString(name) > MaxNameLen || utf8.RuneCountInString(description) > MaxDescriptionLen {
		return ErrTextTooLong
	}
	return nil
}

// Validate checks a category.
func (c *Category) Validate() error { return checkText(c.Name, c.Description) }

// Validate checks the rules of a group that the schema also enforces, so
// they fail early with a useful error.
func (g *OptionGroup) Validate() error {
	if err := checkText(g.Name, ""); err != nil {
		return err
	}
	if g.MinSelect < 0 || g.MaxSelect < 1 || g.MaxSelect < g.MinSelect {
		return ErrInvalidSelection
	}
	for k := range g.Options {
		o := &g.Options[k]
		if err := checkText(o.Name, ""); err != nil {
			return err
		}
		if o.PriceDelta < -money.MaxAmount || o.PriceDelta > money.MaxAmount {
			return ErrInvalidPrice
		}
	}
	return nil
}

// Validate checks an item and its groups.
func (i *Item) Validate() error {
	if err := checkText(i.Name, i.Description); err != nil {
		return err
	}
	if i.Price < 0 || i.Price > money.MaxAmount {
		return ErrInvalidPrice
	}
	n := 0
	for _, g := range i.Groups {
		n += len(g.Options)
	}
	if len(i.Groups) > MaxGroupsPerItem || n > MaxOptionsPerItem {
		return ErrTooManyOptions
	}
	for k := range i.Groups {
		if err := i.Groups[k].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Repository persists the menu. Every method takes the tenant id; a record of
// another tenant is reported as ErrNotFound. Archived items, groups and
// options are never listed; GetItem returns an archived item with its
// ArchivedAt set.
type Repository interface {
	ListCategories(ctx context.Context, tenantID, locationID uuid.UUID) ([]Category, error)
	GetCategory(ctx context.Context, tenantID, id uuid.UUID) (*Category, error)
	CreateCategory(ctx context.Context, c *Category) error
	UpdateCategory(ctx context.Context, c *Category) error
	// SetCategoryPositions numbers the given categories 0, 1, 2… in order.
	SetCategoryPositions(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) error

	// ListItems returns the location's items with their groups and options,
	// by category position and item position.
	ListItems(ctx context.Context, tenantID, locationID uuid.UUID) ([]Item, error)
	GetItem(ctx context.Context, tenantID, id uuid.UUID) (*Item, error)
	// CreateItem writes the item with its groups and options; every id is set.
	CreateItem(ctx context.Context, i *Item) error
	// UpdateItem writes the item's own fields.
	UpdateItem(ctx context.Context, i *Item) error
	// SaveGroups makes the item's groups and options match groups: entries
	// with a known id are updated, entries with a nil id are inserted (and
	// get an id), and existing entries that are absent are archived. An id
	// that belongs to another item is ErrNotFound.
	SaveGroups(ctx context.Context, tenantID, itemID uuid.UUID, groups []OptionGroup, now database.Timestamp) error
	// SetItemPositions numbers the given items 0, 1, 2… in order.
	SetItemPositions(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) error
	// ItemReferenced reports whether any order line points at the item.
	ItemReferenced(ctx context.Context, tenantID, id uuid.UUID) (bool, error)
	// DeleteItem removes an unreferenced item with its groups and options.
	DeleteItem(ctx context.Context, tenantID, id uuid.UUID) error
	// ArchiveItem hides a referenced item for good.
	ArchiveItem(ctx context.Context, tenantID, id uuid.UUID, now database.Timestamp) error
}
