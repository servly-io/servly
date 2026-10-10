package menu

import (
	"errors"
	"testing"

	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
)

func TestItemValidate(t *testing.T) {
	ok := Item{Name: "Margherita", Price: 950, Groups: []OptionGroup{{Name: "Size", MinSelect: 1, MaxSelect: 1}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		item Item
		want error
	}{
		"no name":        {Item{Price: 1}, ErrNameRequired},
		"negative price": {Item{Name: "x", Price: -1}, ErrInvalidPrice},
		"huge price":     {Item{Name: "x", Price: money.MaxAmount + 1}, ErrInvalidPrice},
		"min > max":      {Item{Name: "x", Groups: []OptionGroup{{Name: "g", MinSelect: 2, MaxSelect: 1}}}, ErrInvalidSelection},
		"max 0":          {Item{Name: "x", Groups: []OptionGroup{{Name: "g", MaxSelect: 0}}}, ErrInvalidSelection},
	}
	for name, c := range cases {
		if err := c.item.Validate(); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestOrderable(t *testing.T) {
	i := Item{IsEnabled: true, IsAvailable: true}
	if !i.Orderable() {
		t.Fatal("enabled, available item must be orderable")
	}
	for _, mod := range []func(*Item){
		func(i *Item) { i.IsEnabled = false },
		func(i *Item) { i.IsAvailable = false },
		func(i *Item) { i.ArchivedAt = database.Now() },
	} {
		c := i
		mod(&c)
		if c.Orderable() {
			t.Errorf("%+v must not be orderable", c)
		}
	}
}
