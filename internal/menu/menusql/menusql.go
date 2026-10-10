// Package menusql implements menu.Repository on SQLite and PostgreSQL.
package menusql

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/menu"
	"github.com/servly-io/servly/internal/platform/database"
)

// Store is a menu.Repository over a database handle or transaction.
type Store struct{ q database.Querier }

var _ menu.Repository = (*Store)(nil)

func New(q database.Querier) *Store { return &Store{q: q} }

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return menu.ErrNotFound
	}
	return err
}

func expectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return menu.ErrNotFound
	}
	return nil
}

// Categories.

const categoryCols = `id, tenant_id, location_id, name, description, position, is_enabled, created_at, updated_at`

func scanCategory(row interface{ Scan(...any) error }) (*menu.Category, error) {
	var c menu.Category
	if err := row.Scan(&c.ID, &c.TenantID, &c.LocationID, &c.Name, &c.Description, &c.Position, &c.IsEnabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

func (s *Store) ListCategories(ctx context.Context, tenantID, locationID uuid.UUID) ([]menu.Category, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+categoryCols+` FROM menu_categories WHERE tenant_id = $1 AND location_id = $2 ORDER BY position, created_at`,
		tenantID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []menu.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) GetCategory(ctx context.Context, tenantID, id uuid.UUID) (*menu.Category, error) {
	return scanCategory(s.q.QueryRowContext(ctx,
		`SELECT `+categoryCols+` FROM menu_categories WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

func (s *Store) CreateCategory(ctx context.Context, c *menu.Category) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO menu_categories (`+categoryCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.ID, c.TenantID, c.LocationID, c.Name, c.Description, c.Position, c.IsEnabled, c.CreatedAt, c.UpdatedAt)
	if database.IsForeignKeyViolation(err) {
		return menu.ErrNotFound
	}
	return err
}

func (s *Store) UpdateCategory(ctx context.Context, c *menu.Category) error {
	return expectOne(s.q.ExecContext(ctx,
		`UPDATE menu_categories SET name = $3, description = $4, position = $5, is_enabled = $6, updated_at = $7
		  WHERE tenant_id = $1 AND id = $2`,
		c.TenantID, c.ID, c.Name, c.Description, c.Position, c.IsEnabled, c.UpdatedAt))
}

func (s *Store) SetCategoryPositions(ctx context.Context, tenantID uuid.UUID, idList []uuid.UUID) error {
	for i, id := range idList {
		if err := expectOne(s.q.ExecContext(ctx,
			`UPDATE menu_categories SET position = $3 WHERE tenant_id = $1 AND id = $2`, tenantID, id, i)); err != nil {
			return err
		}
	}
	return nil
}

// Items.

const itemCols = `i.id, i.tenant_id, i.category_id, i.name, i.description, i.price_minor, i.image_asset_id, i.position,
	i.is_enabled, i.is_available, i.archived_at, i.created_at, i.updated_at`

func scanItem(row interface{ Scan(...any) error }) (*menu.Item, error) {
	var i menu.Item
	if err := row.Scan(&i.ID, &i.TenantID, &i.CategoryID, &i.Name, &i.Description, &i.Price, &i.ImageAssetID, &i.Position,
		&i.IsEnabled, &i.IsAvailable, &i.ArchivedAt, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &i, nil
}

func (s *Store) ListItems(ctx context.Context, tenantID, locationID uuid.UUID) ([]menu.Item, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+itemCols+`
		   FROM menu_items i JOIN menu_categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
		  WHERE i.tenant_id = $1 AND c.location_id = $2 AND i.archived_at IS NULL
		  ORDER BY c.position, c.created_at, i.position, i.created_at`, tenantID, locationID)
	if err != nil {
		return nil, err
	}
	var items []menu.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, *it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.loadGroups(ctx, tenantID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) GetItem(ctx context.Context, tenantID, id uuid.UUID) (*menu.Item, error) {
	it, err := scanItem(s.q.QueryRowContext(ctx,
		`SELECT `+itemCols+` FROM menu_items i WHERE i.tenant_id = $1 AND i.id = $2`, tenantID, id))
	if err != nil {
		return nil, err
	}
	items := []menu.Item{*it}
	if err := s.loadGroups(ctx, tenantID, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

// loadGroups fills the active groups and options of items, in two queries.
func (s *Store) loadGroups(ctx context.Context, tenantID uuid.UUID, items []menu.Item) error {
	if len(items) == 0 {
		return nil
	}
	byItem := make(map[uuid.UUID]int, len(items))
	args := []any{tenantID}
	ph := make([]string, len(items))
	for k := range items {
		byItem[items[k].ID] = k
		args = append(args, items[k].ID)
		ph[k] = "$" + strconv.Itoa(k+2)
	}
	in := strings.Join(ph, ", ")

	rows, err := s.q.QueryContext(ctx,
		`SELECT id, tenant_id, item_id, name, min_select, max_select, position, created_at, updated_at
		   FROM option_groups WHERE tenant_id = $1 AND archived_at IS NULL AND item_id IN (`+in+`)
		  ORDER BY position, created_at`, args...)
	if err != nil {
		return err
	}
	type ref struct{ item, group int }
	groupAt := map[uuid.UUID]ref{}
	for rows.Next() {
		var g menu.OptionGroup
		if err := rows.Scan(&g.ID, &g.TenantID, &g.ItemID, &g.Name, &g.MinSelect, &g.MaxSelect, &g.Position, &g.CreatedAt, &g.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		k := byItem[g.ItemID]
		items[k].Groups = append(items[k].Groups, g)
		groupAt[g.ID] = ref{k, len(items[k].Groups) - 1}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(groupAt) == 0 {
		return err
	}

	rows, err = s.q.QueryContext(ctx,
		`SELECT o.id, o.tenant_id, o.option_group_id, o.name, o.price_delta_minor, o.position, o.is_available, o.created_at, o.updated_at
		   FROM options o JOIN option_groups g ON g.tenant_id = o.tenant_id AND g.id = o.option_group_id
		  WHERE o.tenant_id = $1 AND o.archived_at IS NULL AND g.archived_at IS NULL AND g.item_id IN (`+in+`)
		  ORDER BY o.position, o.created_at`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var o menu.Option
		if err := rows.Scan(&o.ID, &o.TenantID, &o.OptionGroupID, &o.Name, &o.PriceDelta, &o.Position, &o.IsAvailable, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return err
		}
		r := groupAt[o.OptionGroupID]
		g := &items[r.item].Groups[r.group]
		g.Options = append(g.Options, o)
	}
	return rows.Err()
}

func (s *Store) CreateItem(ctx context.Context, i *menu.Item) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO menu_items (id, tenant_id, category_id, name, description, price_minor, image_asset_id, position,
		                         is_enabled, is_available, archived_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		i.ID, i.TenantID, i.CategoryID, i.Name, i.Description, i.Price, i.ImageAssetID, i.Position,
		i.IsEnabled, i.IsAvailable, i.ArchivedAt, i.CreatedAt, i.UpdatedAt)
	if database.IsForeignKeyViolation(err) {
		return menu.ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.SaveGroups(ctx, i.TenantID, i.ID, i.Groups, i.CreatedAt)
}

func (s *Store) UpdateItem(ctx context.Context, i *menu.Item) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE menu_items SET category_id = $3, name = $4, description = $5, price_minor = $6, image_asset_id = $7,
		        position = $8, is_enabled = $9, is_available = $10, updated_at = $11
		  WHERE tenant_id = $1 AND id = $2 AND archived_at IS NULL`,
		i.TenantID, i.ID, i.CategoryID, i.Name, i.Description, i.Price, i.ImageAssetID, i.Position,
		i.IsEnabled, i.IsAvailable, i.UpdatedAt)
	if database.IsForeignKeyViolation(err) {
		return menu.ErrNotFound
	}
	return expectOne(res, err)
}

func (s *Store) SaveGroups(ctx context.Context, tenantID, itemID uuid.UUID, groups []menu.OptionGroup, now database.Timestamp) error {
	existing := []menu.Item{{ID: itemID}}
	if err := s.loadGroups(ctx, tenantID, existing); err != nil {
		return err
	}
	oldGroups := map[uuid.UUID]menu.OptionGroup{}
	oldOptions := map[uuid.UUID]uuid.UUID{} // option -> group
	for _, g := range existing[0].Groups {
		oldGroups[g.ID] = g
		for _, o := range g.Options {
			oldOptions[o.ID] = g.ID
		}
	}
	keptGroups := map[uuid.UUID]bool{}
	keptOptions := map[uuid.UUID]bool{}

	for gi := range groups {
		g := &groups[gi]
		g.TenantID, g.ItemID, g.Position, g.UpdatedAt = tenantID, itemID, gi, now
		if g.ID == uuid.Nil {
			g.ID, g.CreatedAt = ids.New(), now
			if _, err := s.q.ExecContext(ctx,
				`INSERT INTO option_groups (id, tenant_id, item_id, name, min_select, max_select, position, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				g.ID, tenantID, itemID, g.Name, g.MinSelect, g.MaxSelect, g.Position, g.CreatedAt, g.UpdatedAt); err != nil {
				return err
			}
		} else {
			if _, ok := oldGroups[g.ID]; !ok {
				return menu.ErrNotFound
			}
			if _, err := s.q.ExecContext(ctx,
				`UPDATE option_groups SET name = $3, min_select = $4, max_select = $5, position = $6, updated_at = $7
				  WHERE tenant_id = $1 AND id = $2`,
				tenantID, g.ID, g.Name, g.MinSelect, g.MaxSelect, g.Position, g.UpdatedAt); err != nil {
				return err
			}
		}
		keptGroups[g.ID] = true

		for oi := range g.Options {
			o := &g.Options[oi]
			o.TenantID, o.OptionGroupID, o.Position, o.UpdatedAt = tenantID, g.ID, oi, now
			if o.ID == uuid.Nil {
				o.ID, o.CreatedAt = ids.New(), now
				if _, err := s.q.ExecContext(ctx,
					`INSERT INTO options (id, tenant_id, option_group_id, name, price_delta_minor, position, is_available, created_at, updated_at)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
					o.ID, tenantID, g.ID, o.Name, o.PriceDelta, o.Position, o.IsAvailable, o.CreatedAt, o.UpdatedAt); err != nil {
					return err
				}
			} else {
				// An option may only stay in the group it belongs to.
				if gid, ok := oldOptions[o.ID]; !ok || gid != g.ID {
					return menu.ErrNotFound
				}
				if _, err := s.q.ExecContext(ctx,
					`UPDATE options SET name = $3, price_delta_minor = $4, position = $5, is_available = $6, updated_at = $7
					  WHERE tenant_id = $1 AND id = $2`,
					tenantID, o.ID, o.Name, o.PriceDelta, o.Position, o.IsAvailable, o.UpdatedAt); err != nil {
					return err
				}
			}
			keptOptions[o.ID] = true
		}
	}

	for id := range oldOptions {
		if !keptOptions[id] {
			if _, err := s.q.ExecContext(ctx, `UPDATE options SET archived_at = $3 WHERE tenant_id = $1 AND id = $2`, tenantID, id, now); err != nil {
				return err
			}
		}
	}
	for id := range oldGroups {
		if !keptGroups[id] {
			if _, err := s.q.ExecContext(ctx, `UPDATE option_groups SET archived_at = $3 WHERE tenant_id = $1 AND id = $2`, tenantID, id, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) SetItemPositions(ctx context.Context, tenantID uuid.UUID, idList []uuid.UUID) error {
	for i, id := range idList {
		if err := expectOne(s.q.ExecContext(ctx,
			`UPDATE menu_items SET position = $3 WHERE tenant_id = $1 AND id = $2`, tenantID, id, i)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ItemReferenced(ctx context.Context, tenantID, id uuid.UUID) (bool, error) {
	var n int
	err := s.q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM order_items WHERE tenant_id = $1 AND menu_item_id = $2`, tenantID, id).Scan(&n)
	return n > 0, err
}

func (s *Store) DeleteItem(ctx context.Context, tenantID, id uuid.UUID) error {
	if _, err := s.q.ExecContext(ctx,
		`DELETE FROM options WHERE tenant_id = $1 AND option_group_id IN
		   (SELECT id FROM option_groups WHERE tenant_id = $1 AND item_id = $2)`, tenantID, id); err != nil {
		return err
	}
	if _, err := s.q.ExecContext(ctx, `DELETE FROM option_groups WHERE tenant_id = $1 AND item_id = $2`, tenantID, id); err != nil {
		return err
	}
	return expectOne(s.q.ExecContext(ctx, `DELETE FROM menu_items WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

func (s *Store) ArchiveItem(ctx context.Context, tenantID, id uuid.UUID, now database.Timestamp) error {
	return expectOne(s.q.ExecContext(ctx,
		`UPDATE menu_items SET archived_at = $3, updated_at = $3 WHERE tenant_id = $1 AND id = $2 AND archived_at IS NULL`,
		tenantID, id, now))
}
