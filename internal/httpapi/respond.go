package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/servly-io/servly/internal/apperr"
)

// errorBody is the one JSON error shape of the API (docs/docs/api.md).
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes err as the API error body.
func writeError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	reqID := middleware.GetReqID(r.Context())
	e, ok := apperr.As(err)
	if !ok {
		log.ErrorContext(r.Context(), "request failed", "error", err, "request_id", reqID, "path", r.URL.Path)
		writeJSON(w, http.StatusInternalServerError, errorBody{errorDetail{
			Code: "internal_error", Message: "internal error", RequestID: reqID,
		}})
		return
	}
	writeJSON(w, statusFor(e.Kind), errorBody{errorDetail{
		Code: e.Code, Message: e.Message, Details: e.Details, RequestID: reqID,
	}})
}

func statusFor(k apperr.Kind) int {
	switch k {
	case apperr.KindInvalid:
		return http.StatusUnprocessableEntity
	case apperr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindRateLimited:
		return http.StatusTooManyRequests
	case apperr.KindTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

var (
	errRouteNotFound    = apperr.NotFound("route_not_found", "no such endpoint")
	errMethodNotAllowed = apperr.New(apperr.KindInvalid, "method_not_allowed", "method not allowed")
	errForbidden        = apperr.Forbidden("forbidden", "your role does not permit this")
	errBodyTooLarge     = apperr.New(apperr.KindTooLarge, "request_too_large", "request body too large")
)
