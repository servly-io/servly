package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/servly-io/servly/internal/apperr"
)

var errMalformedJSON = apperr.Invalid("malformed_json", "request body is not valid JSON for this endpoint")

// decodeJSON reads one JSON object from the body into v. Unknown fields and
// trailing data are rejected, so client typos fail loudly.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errBodyTooLarge
		}
		return errMalformedJSON
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errMalformedJSON
	}
	return nil
}
