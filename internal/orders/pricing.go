package orders

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/menu"
	"github.com/servly-io/servly/internal/money"
)

// LineRequest is one line of a guest's order: ids and counts only. Prices
// never come from the client.
type LineRequest struct {
	MenuItemID uuid.UUID
	Quantity   int
	OptionIDs  []uuid.UUID
	Notes      string
}

var (
	ErrEmptyOrder          = apperr.Invalid("empty_order", "the order has no items")
	ErrTooManyLines        = apperr.Invalid("too_many_lines", "an order has at most 50 lines")
	ErrInvalidQuantity     = apperr.Invalid("invalid_quantity", "quantity must be between 1 and 99")
	ErrItemUnavailable     = apperr.Invalid("item_unavailable", "an item is no longer available")
	ErrOptionUnavailable   = apperr.Invalid("option_unavailable", "a chosen option is not available")
	ErrInvalidOptionChoice = apperr.Invalid("invalid_option_selection", "the chosen options do not fit the item's choices")
	ErrNotesTooLong        = apperr.Invalid("notes_too_long", "notes are at most 500 characters")
	ErrNotesDisabled       = apperr.Invalid("notes_disabled", "this restaurant does not take order notes")
	ErrNegativeLine        = apperr.Invalid("negative_line_price", "a line would cost less than nothing")
)

// Menu is what pricing needs to know about the current menu: the orderable
// items by id and which categories guests can see.
type Menu struct {
	Items             map[uuid.UUID]*menu.Item
	EnabledCategories map[uuid.UUID]bool
	NotesEnabled      bool
}

// Price validates the requested lines against the current menu and computes
// the snapshot lines and the subtotal. It implements the rules of
// docs/docs/domain-model.md#pricing; every error names the offending line.
func Price(reqs []LineRequest, orderNotes string, m Menu) ([]Line, money.Amount, error) {
	if len(reqs) == 0 {
		return nil, 0, ErrEmptyOrder
	}
	if len(reqs) > MaxLines {
		return nil, 0, ErrTooManyLines
	}
	if err := checkNotes(orderNotes, m.NotesEnabled); err != nil {
		return nil, 0, err
	}
	lines := make([]Line, 0, len(reqs))
	var subtotal money.Amount
	for i, r := range reqs {
		at := map[string]any{"line": i}
		if r.Quantity < 1 || r.Quantity > MaxQuantity {
			return nil, 0, ErrInvalidQuantity.WithDetails(at)
		}
		if err := checkNotes(r.Notes, m.NotesEnabled); err != nil {
			return nil, 0, err.(*apperr.Error).WithDetails(at)
		}
		it := m.Items[r.MenuItemID]
		if it == nil || !it.Orderable() || !m.EnabledCategories[it.CategoryID] {
			at["menu_item_id"] = r.MenuItemID
			return nil, 0, ErrItemUnavailable.WithDetails(at)
		}

		chosen, err := chooseOptions(it, r.OptionIDs, at)
		if err != nil {
			return nil, 0, err
		}
		var optTotal money.Amount
		for _, c := range chosen {
			if optTotal, err = money.Add(optTotal, c.PriceDelta); err != nil {
				return nil, 0, err
			}
		}
		unit, err := money.Add(it.Price, optTotal)
		if err != nil {
			return nil, 0, err
		}
		if unit < 0 {
			return nil, 0, ErrNegativeLine.WithDetails(at)
		}
		lineTotal, err := money.Mul(unit, r.Quantity)
		if err != nil {
			return nil, 0, err
		}
		if subtotal, err = money.Add(subtotal, lineTotal); err != nil {
			return nil, 0, err
		}
		lines = append(lines, Line{
			MenuItemID:   uuid.NullUUID{UUID: it.ID, Valid: true},
			Position:     i,
			Name:         it.Name,
			Quantity:     r.Quantity,
			UnitPrice:    it.Price,
			OptionsTotal: optTotal,
			LineTotal:    lineTotal,
			Notes:        strings.TrimSpace(r.Notes),
			Options:      chosen,
		})
	}
	return lines, subtotal, nil
}

func checkNotes(notes string, enabled bool) error {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return nil
	}
	if !enabled {
		return ErrNotesDisabled
	}
	if utf8.RuneCountInString(notes) > MaxNotesLength {
		return ErrNotesTooLong
	}
	return nil
}

// chooseOptions resolves option ids against the item's groups and checks
// availability, duplicates and every group's min/max.
func chooseOptions(it *menu.Item, ids []uuid.UUID, at map[string]any) ([]LineOption, error) {
	type ref struct {
		group *menu.OptionGroup
		opt   *menu.Option
	}
	byID := map[uuid.UUID]ref{}
	for gi := range it.Groups {
		g := &it.Groups[gi]
		for oi := range g.Options {
			byID[g.Options[oi].ID] = ref{g, &g.Options[oi]}
		}
	}
	perGroup := map[uuid.UUID]int{}
	seen := map[uuid.UUID]bool{}
	var out []LineOption
	for _, id := range ids {
		r, ok := byID[id]
		if !ok || seen[id] {
			return nil, ErrInvalidOptionChoice.WithDetails(with(at, "option_id", id))
		}
		if !r.opt.IsAvailable {
			return nil, ErrOptionUnavailable.WithDetails(with(at, "option_id", id))
		}
		seen[id] = true
		perGroup[r.group.ID]++
		out = append(out, LineOption{
			OptionID:   uuid.NullUUID{UUID: id, Valid: true},
			GroupName:  r.group.Name,
			OptionName: r.opt.Name,
			PriceDelta: r.opt.PriceDelta,
		})
	}
	for _, g := range it.Groups {
		if n := perGroup[g.ID]; n < g.MinSelect || n > g.MaxSelect {
			return nil, ErrInvalidOptionChoice.WithDetails(with(at, "option_group_id", g.ID))
		}
	}
	return out, nil
}

func with(m map[string]any, k string, v any) map[string]any {
	c := make(map[string]any, len(m)+1)
	for kk, vv := range m {
		c[kk] = vv
	}
	c[k] = v
	return c
}
