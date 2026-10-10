package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/menu"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/tenant"
)

func (s *server) menuRoutes(r chi.Router) {
	manage := func(h http.HandlerFunc) http.HandlerFunc { return s.can(tenant.PermManageMenu, h) }
	r.Get("/menu/categories", s.listCategories)
	r.Post("/menu/categories", manage(s.createCategory))
	r.Patch("/menu/categories/{id}", manage(s.updateCategory))
	r.Get("/menu/items", s.listItems)
	r.Get("/menu/items/{id}", s.getItem)
	r.Post("/menu/items", manage(s.createItem))
	r.Patch("/menu/items/{id}", manage(s.updateItem))
	r.Post("/menu/items/{id}/duplicate", manage(s.duplicateItem))
	r.Delete("/menu/items/{id}", manage(s.deleteItem))
}

type categoryJSON struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Position    int       `json:"position"`
	IsEnabled   bool      `json:"is_enabled"`
}

func categoryView(c menu.Category) categoryJSON {
	return categoryJSON{ID: c.ID, Name: c.Name, Description: c.Description, Position: c.Position, IsEnabled: c.IsEnabled}
}

type categoryRequest struct {
	Name        optional[string] `json:"name"`
	Description optional[string] `json:"description"`
	IsEnabled   optional[bool]   `json:"is_enabled"`
	Position    optional[int]    `json:"position"`
}

func (req categoryRequest) input() (app.CategoryInput, error) {
	var in app.CategoryInput
	var e1, e2, e3, e4 error
	in.Name, e1 = req.Name.ptr()
	in.Description, e2 = req.Description.ptr()
	in.IsEnabled, e3 = req.IsEnabled.ptr()
	in.Position, e4 = req.Position.ptr()
	return in, firstErr(e1, e2, e3, e4)
}

func (s *server) listCategories(w http.ResponseWriter, r *http.Request) {
	cs, err := s.App.ListCategories(r.Context(), principal(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]categoryJSON, 0, len(cs))
	for _, c := range cs {
		out = append(out, categoryView(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *server) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, err := s.App.CreateCategory(r.Context(), principal(r), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, categoryView(*c))
}

func (s *server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req categoryRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, err := s.App.UpdateCategory(r.Context(), principal(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, categoryView(*c))
}

type optionJSON struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	PriceDelta  money.Amount `json:"price_delta_minor"`
	IsAvailable bool         `json:"is_available"`
}

type groupJSON struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	MinSelect int          `json:"min_select"`
	MaxSelect int          `json:"max_select"`
	Options   []optionJSON `json:"options"`
}

type itemJSON struct {
	ID           uuid.UUID    `json:"id"`
	CategoryID   uuid.UUID    `json:"category_id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Price        money.Amount `json:"price_minor"`
	ImageAssetID *uuid.UUID   `json:"image_asset_id"`
	ImageURL     *string      `json:"image_url"`
	IsEnabled    bool         `json:"is_enabled"`
	IsAvailable  bool         `json:"is_available"`
	Position     int          `json:"position"`
	OptionGroups []groupJSON  `json:"option_groups"`
}

func itemView(i menu.Item) itemJSON {
	v := itemJSON{ID: i.ID, CategoryID: i.CategoryID, Name: i.Name, Description: i.Description, Price: i.Price,
		IsEnabled: i.IsEnabled, IsAvailable: i.IsAvailable, Position: i.Position, OptionGroups: groupViews(i.Groups)}
	if i.ImageAssetID.Valid {
		id := i.ImageAssetID.UUID
		url := assetURL(id)
		v.ImageAssetID, v.ImageURL = &id, &url
	}
	return v
}

func groupViews(gs []menu.OptionGroup) []groupJSON {
	out := make([]groupJSON, 0, len(gs))
	for _, g := range gs {
		gv := groupJSON{ID: g.ID, Name: g.Name, MinSelect: g.MinSelect, MaxSelect: g.MaxSelect, Options: []optionJSON{}}
		for _, o := range g.Options {
			gv.Options = append(gv.Options, optionJSON{ID: o.ID, Name: o.Name, PriceDelta: o.PriceDelta, IsAvailable: o.IsAvailable})
		}
		out = append(out, gv)
	}
	return out
}

// assetURL is where the public serves an uploaded image.
func assetURL(id uuid.UUID) string { return "/api/public/assets/" + id.String() }

type optionRequest struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	PriceDelta  money.Amount `json:"price_delta_minor"`
	IsAvailable *bool        `json:"is_available"`
}

type groupRequest struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	MinSelect int             `json:"min_select"`
	MaxSelect int             `json:"max_select"`
	Options   []optionRequest `json:"options"`
}

type itemRequest struct {
	CategoryID   optional[uuid.UUID]      `json:"category_id"`
	Name         optional[string]         `json:"name"`
	Description  optional[string]         `json:"description"`
	Price        optional[money.Amount]   `json:"price_minor"`
	ImageAssetID optional[uuid.UUID]      `json:"image_asset_id"`
	IsEnabled    optional[bool]           `json:"is_enabled"`
	IsAvailable  optional[bool]           `json:"is_available"`
	Position     optional[int]            `json:"position"`
	OptionGroups optional[[]groupRequest] `json:"option_groups"`
}

func (req itemRequest) input() (app.ItemInput, error) {
	var in app.ItemInput
	var errs [9]error
	in.CategoryID, errs[0] = req.CategoryID.ptr()
	in.Name, errs[1] = req.Name.ptr()
	in.Description, errs[2] = req.Description.ptr()
	in.Price, errs[3] = req.Price.ptr()
	in.IsEnabled, errs[4] = req.IsEnabled.ptr()
	in.IsAvailable, errs[5] = req.IsAvailable.ptr()
	in.Position, errs[6] = req.Position.ptr()
	if req.ImageAssetID.Set { // null removes the image
		in.ImageAssetID = &uuid.NullUUID{UUID: req.ImageAssetID.V, Valid: !req.ImageAssetID.Null}
	}
	groups, err := req.OptionGroups.ptr()
	errs[7] = err
	if groups != nil {
		gs := make([]app.GroupInput, len(*groups))
		for gi, g := range *groups {
			gs[gi] = app.GroupInput{ID: g.ID, Name: g.Name, MinSelect: g.MinSelect, MaxSelect: g.MaxSelect}
			for _, o := range g.Options {
				avail := o.IsAvailable == nil || *o.IsAvailable
				gs[gi].Options = append(gs[gi].Options, app.OptionInput{ID: o.ID, Name: o.Name, PriceDelta: o.PriceDelta, IsAvailable: avail})
			}
		}
		in.Groups = &gs
	}
	return in, firstErr(errs[:]...)
}

func (s *server) listItems(w http.ResponseWriter, r *http.Request) {
	var cat uuid.NullUUID
	if q := r.URL.Query().Get("category_id"); q != "" {
		id, err := uuid.Parse(q)
		if err != nil {
			s.fail(w, r, errMalformedJSON.WithMessage("category_id is not a valid id"))
			return
		}
		cat = uuid.NullUUID{UUID: id, Valid: true}
	}
	items, err := s.App.ListItems(r.Context(), principal(r), cat)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]itemJSON, 0, len(items))
	for _, i := range items {
		out = append(out, itemView(i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *server) getItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	it, err := s.App.GetItem(r.Context(), principal(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemView(*it))
}

func (s *server) createItem(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	it, err := s.App.CreateItem(r.Context(), principal(r), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemView(*it))
}

func (s *server) updateItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req itemRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	it, err := s.App.UpdateItem(r.Context(), principal(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemView(*it))
}

func (s *server) duplicateItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	it, err := s.App.DuplicateItem(r.Context(), principal(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemView(*it))
}

func (s *server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	archived, err := s.App.DeleteItem(r.Context(), principal(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if archived {
		writeJSON(w, http.StatusOK, map[string]bool{"archived": true})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
