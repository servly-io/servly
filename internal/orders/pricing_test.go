package orders

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/menu"
	"github.com/servly-io/servly/internal/money"
)

type fixture struct {
	m                         Menu
	pizza, soldOut, hiddenCat *menu.Item
	normal, large, basil, bad uuid.UUID
}

func newFixture() *fixture {
	f := &fixture{normal: uuid.New(), large: uuid.New(), basil: uuid.New(), bad: uuid.New()}
	cat, hidden := uuid.New(), uuid.New()
	f.pizza = &menu.Item{ID: uuid.New(), CategoryID: cat, Name: "Margherita", Price: 950, IsEnabled: true, IsAvailable: true,
		Groups: []menu.OptionGroup{
			{ID: uuid.New(), Name: "Size", MinSelect: 1, MaxSelect: 1, Options: []menu.Option{
				{ID: f.normal, Name: "Normal", IsAvailable: true},
				{ID: f.large, Name: "Large", PriceDelta: 300, IsAvailable: true},
			}},
			{ID: uuid.New(), Name: "Extras", MinSelect: 0, MaxSelect: 2, Options: []menu.Option{
				{ID: f.basil, Name: "Basil", PriceDelta: 50, IsAvailable: true},
				{ID: f.bad, Name: "Truffle", PriceDelta: 900, IsAvailable: false},
			}},
		}}
	f.soldOut = &menu.Item{ID: uuid.New(), CategoryID: cat, Name: "Calzone", Price: 1100, IsEnabled: true, IsAvailable: false}
	f.hiddenCat = &menu.Item{ID: uuid.New(), CategoryID: hidden, Name: "Secret", Price: 1, IsEnabled: true, IsAvailable: true}
	f.m = Menu{
		Items:             map[uuid.UUID]*menu.Item{f.pizza.ID: f.pizza, f.soldOut.ID: f.soldOut, f.hiddenCat.ID: f.hiddenCat},
		EnabledCategories: map[uuid.UUID]bool{cat: true},
		NotesEnabled:      true,
	}
	return f
}

func TestPrice(t *testing.T) {
	f := newFixture()
	lines, subtotal, err := Price([]LineRequest{
		{MenuItemID: f.pizza.ID, Quantity: 2, OptionIDs: []uuid.UUID{f.large, f.basil}, Notes: " well done "},
		{MenuItemID: f.pizza.ID, Quantity: 1, OptionIDs: []uuid.UUID{f.normal}},
	}, "table by the window", f.m)
	if err != nil {
		t.Fatal(err)
	}
	// (950 + 300 + 50) × 2 = 2600; 950 × 1 = 950.
	if subtotal != 3550 || len(lines) != 2 {
		t.Fatalf("subtotal %d, lines %d", subtotal, len(lines))
	}
	l := lines[0]
	if l.Name != "Margherita" || l.UnitPrice != 950 || l.OptionsTotal != 350 || l.LineTotal != 2600 || l.Notes != "well done" ||
		len(l.Options) != 2 || l.Options[0].GroupName != "Size" || l.Options[0].OptionName != "Large" || l.Position != 0 {
		t.Fatalf("line: %+v", l)
	}
}

func TestPriceRejects(t *testing.T) {
	longNotes := strings.Repeat("x", MaxNotesLength+1)
	one := func(f *fixture, opts ...uuid.UUID) []LineRequest {
		return []LineRequest{{MenuItemID: f.pizza.ID, Quantity: 1, OptionIDs: opts}}
	}
	cases := map[string]struct {
		build func(f *fixture) ([]LineRequest, string)
		want  error
	}{
		"empty": {func(f *fixture) ([]LineRequest, string) { return nil, "" }, ErrEmptyOrder},
		"too many lines": {func(f *fixture) ([]LineRequest, string) {
			r := make([]LineRequest, MaxLines+1)
			for i := range r {
				r[i] = one(f, f.normal)[0]
			}
			return r, ""
		}, ErrTooManyLines},
		"quantity 0": {func(f *fixture) ([]LineRequest, string) {
			r := one(f, f.normal)
			r[0].Quantity = 0
			return r, ""
		}, ErrInvalidQuantity},
		"quantity 100": {func(f *fixture) ([]LineRequest, string) {
			r := one(f, f.normal)
			r[0].Quantity = 100
			return r, ""
		}, ErrInvalidQuantity},
		"unknown item": {func(f *fixture) ([]LineRequest, string) {
			return []LineRequest{{MenuItemID: uuid.New(), Quantity: 1}}, ""
		}, ErrItemUnavailable},
		"sold out": {func(f *fixture) ([]LineRequest, string) {
			return []LineRequest{{MenuItemID: f.soldOut.ID, Quantity: 1}}, ""
		}, ErrItemUnavailable},
		"hidden category": {func(f *fixture) ([]LineRequest, string) {
			return []LineRequest{{MenuItemID: f.hiddenCat.ID, Quantity: 1}}, ""
		}, ErrItemUnavailable},
		"missing required": {func(f *fixture) ([]LineRequest, string) { return one(f), "" }, ErrInvalidOptionChoice},
		"two sizes":        {func(f *fixture) ([]LineRequest, string) { return one(f, f.normal, f.large), "" }, ErrInvalidOptionChoice},
		"duplicate option": {func(f *fixture) ([]LineRequest, string) { return one(f, f.normal, f.basil, f.basil), "" }, ErrInvalidOptionChoice},
		"foreign option":   {func(f *fixture) ([]LineRequest, string) { return one(f, f.normal, uuid.New()), "" }, ErrInvalidOptionChoice},
		"option sold out":  {func(f *fixture) ([]LineRequest, string) { return one(f, f.normal, f.bad), "" }, ErrOptionUnavailable},
		"long line notes": {func(f *fixture) ([]LineRequest, string) {
			r := one(f, f.normal)
			r[0].Notes = longNotes
			return r, ""
		}, ErrNotesTooLong},
		"long order notes": {func(f *fixture) ([]LineRequest, string) { return one(f, f.normal), longNotes }, ErrNotesTooLong},
		"notes disabled": {func(f *fixture) ([]LineRequest, string) {
			f.m.NotesEnabled = false
			return one(f, f.normal), "hi"
		}, ErrNotesDisabled},
		"negative line": {func(f *fixture) ([]LineRequest, string) {
			f.pizza.Groups[0].Options[0].PriceDelta = -1000
			return one(f, f.normal), ""
		}, ErrNegativeLine},
		"overflow": {func(f *fixture) ([]LineRequest, string) {
			f.pizza.Price = money.MaxAmount * 100
			r := one(f, f.normal)
			r[0].Quantity = 99
			return r, ""
		}, money.ErrAmountRange},
	}
	for name, c := range cases {
		f := newFixture()
		reqs, notes := c.build(f)
		if _, _, err := Price(reqs, notes, f.m); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestPriceErrorNamesLine(t *testing.T) {
	f := newFixture()
	_, _, err := Price([]LineRequest{
		{MenuItemID: f.pizza.ID, Quantity: 1, OptionIDs: []uuid.UUID{f.normal}},
		{MenuItemID: f.soldOut.ID, Quantity: 1},
	}, "", f.m)
	e, ok := apperr.As(err)
	if !ok || e.Details["line"] != 1 || e.Details["menu_item_id"] != f.soldOut.ID {
		t.Fatalf("err = %#v", err)
	}
}
