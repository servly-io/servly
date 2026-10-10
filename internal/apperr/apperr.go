// Package apperr is the error vocabulary shared by the domain, the
// repositories and the HTTP layer. A domain or repository function returns an
// *Error; the HTTP layer maps its Kind to a status code and its Code to the
// "code" field of the JSON error body (docs/docs/api.md). Any other error is an
// internal error and its text never reaches a client.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error by how a caller should react to it.
type Kind int

const (
	KindInvalid         Kind = iota + 1 // the request is malformed or breaks a rule: 400/422
	KindUnauthenticated                 // no valid session: 401
	KindForbidden                       // authenticated, not permitted: 403
	KindNotFound                        // absent, or not visible in this tenant: 404
	KindConflict                        // concurrent change or duplicate: 409
	KindRateLimited                     // too many requests: 429
	KindTooLarge                        // request body too large: 413
)

// Error is an expected, client-safe failure.
type Error struct {
	Kind    Kind
	Code    string         // stable, snake_case, documented in docs/docs/api.md
	Message string         // safe to show to the client
	Details map[string]any // optional, safe to show to the client
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Is matches on Kind and Code, so errors.Is(err, orders.ErrInvalidTransition)
// holds for every error built from that template.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Kind == e.Kind && t.Code == e.Code
}

// WithDetails returns a copy of e carrying details.
func (e *Error) WithDetails(details map[string]any) *Error {
	c := *e
	c.Details = details
	return &c
}

// WithMessage returns a copy of e with a more specific message.
func (e *Error) WithMessage(format string, args ...any) *Error {
	c := *e
	c.Message = fmt.Sprintf(format, args...)
	return &c
}

func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

func Invalid(code, message string) *Error   { return New(KindInvalid, code, message) }
func Forbidden(code, message string) *Error { return New(KindForbidden, code, message) }
func NotFound(code, message string) *Error  { return New(KindNotFound, code, message) }
func Conflict(code, message string) *Error  { return New(KindConflict, code, message) }

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
