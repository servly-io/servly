package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
)

// optional is a PATCH field: absent (Set false), null (Set, Null) or a value.
type optional[T any] struct {
	Set  bool
	Null bool
	V    T
}

func (o *optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(b, []byte("null")) {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.V)
}

// ptr returns the value for a field that may not be null, or nil if absent.
// A null for such a field is a client error.
func (o optional[T]) ptr() (*T, error) {
	if !o.Set {
		return nil, nil
	}
	if o.Null {
		return nil, errMalformedJSON
	}
	return &o.V, nil
}

var errBadID = apperr.NotFound("not_found", "not found")

// pathID parses a UUID path parameter; a malformed id is simply not found.
func pathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, errBadID
	}
	return id, nil
}

// firstErr returns the first non-nil error.
func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
