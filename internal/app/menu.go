package app

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/audit"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/menu"
	"github.com/servly-io/servly/internal/menu/menusql"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
)

// CategoryInput creates or patches a category; nil fields are unchanged.
type CategoryInput struct {
	Name        *string
	Description *string
	IsEnabled   *bool
	Position    *int // 0-based target position; the others shift
}

func (a *App) ListCategories(ctx context.Context, p *Principal) ([]menu.Category, error) {
	return menusql.New(a.DB).ListCategories(ctx, p.TenantID(), p.Location.ID)
}

func (a *App) CreateCategory(ctx context.Context, p *Principal, in CategoryInput) (*menu.Category, error) {
	now := database.Now()
	c := &menu.Category{ID: ids.New(), TenantID: p.TenantID(), LocationID: p.Location.ID, IsEnabled: true, CreatedAt: now, UpdatedAt: now}
	applyCategory(c, in)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := menusql.New(tx)
		all, err := st.ListCategories(ctx, c.TenantID, c.LocationID)
		if err != nil {
			return err
		}
		c.Position = len(all)
		if err := st.CreateCategory(ctx, c); err != nil {
			return err
		}
		if in.Position != nil {
			if err := moveCategory(ctx, st, c, append(all, *c), *in.Position); err != nil {
				return err
			}
		}
		return a.audit(ctx, tx, p, "menu.category.created", "menu_category", c.ID, map[string]any{"name": c.Name})
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (a *App) UpdateCategory(ctx context.Context, p *Principal, id uuid.UUID, in CategoryInput) (*menu.Category, error) {
	var c *menu.Category
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := menusql.New(tx)
		var err error
		if c, err = st.GetCategory(ctx, p.TenantID(), id); err != nil {
			return err
		}
		applyCategory(c, in)
		if err := c.Validate(); err != nil {
			return err
		}
		c.UpdatedAt = database.Now()
		if err := st.UpdateCategory(ctx, c); err != nil {
			return err
		}
		if in.Position != nil {
			all, err := st.ListCategories(ctx, c.TenantID, c.LocationID)
			if err != nil {
				return err
			}
			if err := moveCategory(ctx, st, c, all, *in.Position); err != nil {
				return err
			}
		}
		return a.audit(ctx, tx, p, "menu.category.updated", "menu_category", c.ID, nil)
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func applyCategory(c *menu.Category, in CategoryInput) {
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		c.Description = strings.TrimSpace(*in.Description)
	}
	if in.IsEnabled != nil {
		c.IsEnabled = *in.IsEnabled
	}
}

func moveCategory(ctx context.Context, st *menusql.Store, c *menu.Category, all []menu.Category, to int) error {
	order := make([]uuid.UUID, 0, len(all))
	for _, x := range all {
		order = append(order, x.ID)
	}
	order = moveTo(order, c.ID, to)
	c.Position = slices.Index(order, c.ID)
	return st.SetCategoryPositions(ctx, c.TenantID, order)
}

// moveTo moves id to position to (clamped) and shifts the others.
func moveTo(order []uuid.UUID, id uuid.UUID, to int) []uuid.UUID {
	i := slices.Index(order, id)
	if i < 0 {
		return order
	}
	order = slices.Delete(order, i, i+1)
	to = max(0, min(to, len(order)))
	return slices.Insert(order, to, id)
}

// OptionInput is one option of a group; a nil ID adds a new option.
type OptionInput struct {
	ID          uuid.UUID
	Name        string
	PriceDelta  money.Amount
	IsAvailable bool
}

// GroupInput is one option group; a nil ID adds a new group.
type GroupInput struct {
	ID        uuid.UUID
	Name      string
	MinSelect int
	MaxSelect int
	Options   []OptionInput
}

// ItemInput creates or patches an item; nil fields are unchanged. Groups,
// when set, is the complete new list: absent groups and options are removed
// (archived, as past orders may reference them).
type ItemInput struct {
	CategoryID   *uuid.UUID
	Name         *string
	Description  *string
	Price        *money.Amount
	ImageAssetID *uuid.NullUUID
	IsEnabled    *bool
	IsAvailable  *bool
	Position     *int
	Groups       *[]GroupInput
}

func (a *App) ListItems(ctx context.Context, p *Principal, categoryID uuid.NullUUID) ([]menu.Item, error) {
	items, err := menusql.New(a.DB).ListItems(ctx, p.TenantID(), p.Location.ID)
	if err != nil || !categoryID.Valid {
		return items, err
	}
	return slices.DeleteFunc(items, func(i menu.Item) bool { return i.CategoryID != categoryID.UUID }), nil
}

func (a *App) GetItem(ctx context.Context, p *Principal, id uuid.UUID) (*menu.Item, error) {
	it, err := menusql.New(a.DB).GetItem(ctx, p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	if !it.ArchivedAt.IsZero() {
		return nil, menu.ErrNotFound
	}
	return it, nil
}

func (a *App) CreateItem(ctx context.Context, p *Principal, in ItemInput) (*menu.Item, error) {
	if in.CategoryID == nil {
		return nil, menu.ErrCategoryRequired
	}
	now := database.Now()
	it := &menu.Item{ID: ids.New(), TenantID: p.TenantID(), IsEnabled: true, IsAvailable: true, CreatedAt: now, UpdatedAt: now}
	applyItem(it, in)
	if err := it.Validate(); err != nil {
		return nil, err
	}
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := menusql.New(tx)
		if _, err := a.categoryHere(ctx, st, p, it.CategoryID); err != nil {
			return err
		}
		siblings, err := a.siblings(ctx, st, p, it.CategoryID)
		if err != nil {
			return err
		}
		it.Position = len(siblings)
		if err := st.CreateItem(ctx, it); err != nil {
			return err
		}
		if in.Position != nil {
			if err := moveItem(ctx, st, it, append(siblings, it.ID), *in.Position); err != nil {
				return err
			}
		}
		return a.audit(ctx, tx, p, "menu.item.created", "menu_item", it.ID, map[string]any{"name": it.Name, "price_minor": it.Price})
	})
	if err != nil {
		return nil, err
	}
	return it, nil
}

func (a *App) UpdateItem(ctx context.Context, p *Principal, id uuid.UUID, in ItemInput) (*menu.Item, error) {
	var it *menu.Item
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := menusql.New(tx)
		var err error
		if it, err = st.GetItem(ctx, p.TenantID(), id); err != nil {
			return err
		}
		if !it.ArchivedAt.IsZero() {
			return menu.ErrNotFound
		}
		before := *it
		applyItem(it, in)
		if err := it.Validate(); err != nil {
			return err
		}
		moved := it.CategoryID != before.CategoryID
		if moved {
			if _, err := a.categoryHere(ctx, st, p, it.CategoryID); err != nil {
				return err
			}
			siblings, err := a.siblings(ctx, st, p, it.CategoryID)
			if err != nil {
				return err
			}
			it.Position = len(siblings)
		}
		it.UpdatedAt = database.Now()
		if err := st.UpdateItem(ctx, it); err != nil {
			return err
		}
		if in.Groups != nil {
			if err := st.SaveGroups(ctx, it.TenantID, it.ID, it.Groups, it.UpdatedAt); err != nil {
				return err
			}
		}
		if in.Position != nil {
			siblings, err := a.siblings(ctx, st, p, it.CategoryID)
			if err != nil {
				return err
			}
			if err := moveItem(ctx, st, it, siblings, *in.Position); err != nil {
				return err
			}
		}
		data := map[string]any{}
		if before.Price != it.Price {
			data["price_minor"] = map[string]any{"from": before.Price, "to": it.Price}
		}
		if before.IsAvailable != it.IsAvailable {
			data["is_available"] = it.IsAvailable
		}
		if err := a.audit(ctx, tx, p, "menu.item.updated", "menu_item", it.ID, data); err != nil {
			return err
		}
		it, err = st.GetItem(ctx, it.TenantID, it.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return it, nil
}

// DuplicateItem copies an item with its groups and options. The copy is
// disabled so it does not appear on the menu before it is edited.
func (a *App) DuplicateItem(ctx context.Context, p *Principal, id uuid.UUID) (*menu.Item, error) {
	src, err := a.GetItem(ctx, p, id)
	if err != nil {
		return nil, err
	}
	in := ItemInput{
		CategoryID:   &src.CategoryID,
		Name:         ptr(copyName(src.Name)),
		Description:  &src.Description,
		Price:        &src.Price,
		ImageAssetID: &src.ImageAssetID,
		IsEnabled:    ptr(false),
		IsAvailable:  &src.IsAvailable,
		Position:     ptr(src.Position + 1),
		Groups:       ptr(groupInputs(src.Groups, true)),
	}
	return a.CreateItem(ctx, p, in)
}

func copyName(name string) string {
	const suffix = " (copy)"
	if r := []rune(name); len(r)+len([]rune(suffix)) > menu.MaxNameLen {
		name = string(r[:menu.MaxNameLen-len([]rune(suffix))])
	}
	return name + suffix
}

// DeleteItem deletes an item no order references, and archives one that is
// referenced. It reports whether it archived.
func (a *App) DeleteItem(ctx context.Context, p *Principal, id uuid.UUID) (archived bool, err error) {
	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := menusql.New(tx)
		it, err := st.GetItem(ctx, p.TenantID(), id)
		if err != nil {
			return err
		}
		if !it.ArchivedAt.IsZero() {
			return menu.ErrNotFound
		}
		if archived, err = st.ItemReferenced(ctx, it.TenantID, it.ID); err != nil {
			return err
		}
		action := "menu.item.deleted"
		if archived {
			action = "menu.item.archived"
			err = st.ArchiveItem(ctx, it.TenantID, it.ID, database.Now())
		} else {
			err = st.DeleteItem(ctx, it.TenantID, it.ID)
		}
		if err != nil {
			return err
		}
		return a.audit(ctx, tx, p, action, "menu_item", it.ID, map[string]any{"name": it.Name})
	})
	return archived, err
}

func applyItem(it *menu.Item, in ItemInput) {
	if in.CategoryID != nil {
		it.CategoryID = *in.CategoryID
	}
	if in.Name != nil {
		it.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		it.Description = strings.TrimSpace(*in.Description)
	}
	if in.Price != nil {
		it.Price = *in.Price
	}
	if in.ImageAssetID != nil {
		it.ImageAssetID = *in.ImageAssetID
	}
	if in.IsEnabled != nil {
		it.IsEnabled = *in.IsEnabled
	}
	if in.IsAvailable != nil {
		it.IsAvailable = *in.IsAvailable
	}
	if in.Groups != nil {
		it.Groups = make([]menu.OptionGroup, len(*in.Groups))
		for gi, g := range *in.Groups {
			mg := menu.OptionGroup{ID: g.ID, Name: strings.TrimSpace(g.Name), MinSelect: g.MinSelect, MaxSelect: g.MaxSelect}
			for _, o := range g.Options {
				mg.Options = append(mg.Options, menu.Option{ID: o.ID, Name: strings.TrimSpace(o.Name), PriceDelta: o.PriceDelta, IsAvailable: o.IsAvailable})
			}
			it.Groups[gi] = mg
		}
	}
}

func groupInputs(gs []menu.OptionGroup, fresh bool) []GroupInput {
	out := make([]GroupInput, len(gs))
	for gi, g := range gs {
		in := GroupInput{ID: g.ID, Name: g.Name, MinSelect: g.MinSelect, MaxSelect: g.MaxSelect}
		if fresh {
			in.ID = uuid.Nil
		}
		for _, o := range g.Options {
			oi := OptionInput{ID: o.ID, Name: o.Name, PriceDelta: o.PriceDelta, IsAvailable: o.IsAvailable}
			if fresh {
				oi.ID = uuid.Nil
			}
			in.Options = append(in.Options, oi)
		}
		out[gi] = in
	}
	return out
}

// categoryHere checks that a category exists in the principal's location.
func (a *App) categoryHere(ctx context.Context, st *menusql.Store, p *Principal, id uuid.UUID) (*menu.Category, error) {
	c, err := st.GetCategory(ctx, p.TenantID(), id)
	if err != nil {
		return nil, menu.ErrCategoryRequired
	}
	if c.LocationID != p.Location.ID {
		return nil, menu.ErrCategoryRequired
	}
	return c, nil
}

// siblings lists the ids of a category's items in position order.
func (a *App) siblings(ctx context.Context, st *menusql.Store, p *Principal, categoryID uuid.UUID) ([]uuid.UUID, error) {
	items, err := st.ListItems(ctx, p.TenantID(), p.Location.ID)
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for _, i := range items {
		if i.CategoryID == categoryID {
			out = append(out, i.ID)
		}
	}
	return out, nil
}

func moveItem(ctx context.Context, st *menusql.Store, it *menu.Item, siblings []uuid.UUID, to int) error {
	if !slices.Contains(siblings, it.ID) {
		siblings = append(siblings, it.ID)
	}
	order := moveTo(slices.Clone(siblings), it.ID, to)
	it.Position = slices.Index(order, it.ID)
	return st.SetItemPositions(ctx, it.TenantID, order)
}

// audit records a change made by the principal in the active tenant.
func (a *App) audit(ctx context.Context, q database.Querier, p *Principal, action, entityType string, id uuid.UUID, data map[string]any) error {
	tid, actor := audit.Tenant(p.TenantID(), p.User.ID)
	return audit.Record(ctx, q, audit.Event{TenantID: tid, ActorUserID: actor, Action: action, EntityType: entityType, EntityID: id, Data: data})
}

func ptr[T any](v T) *T { return &v }
