package httpapi

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/servly-io/servly/internal/apperr"
)

var errRateLimited = apperr.New(apperr.KindRateLimited, "rate_limited", "too many requests; wait a moment and try again")

// limiter is a set of token buckets keyed by a string (an IP, an email, a
// token). It lives in the process, which suits the single-instance MVP.
type limiter struct {
	mu    sync.Mutex
	every time.Duration
	burst int
	keys  map[string]*bucket
	swept time.Time
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// newLimiter allows burst requests at once, then one per every.
func newLimiter(every time.Duration, burst int) *limiter {
	return &limiter{every: every, burst: burst, keys: map[string]*bucket{}}
}

func (l *limiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) > time.Minute {
		for k, b := range l.keys {
			// Idle long enough to have refilled completely: forget it.
			if now.Sub(b.seen) > l.every*time.Duration(l.burst)+time.Minute {
				delete(l.keys, k)
			}
		}
		l.swept = now
	}
	b, ok := l.keys[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.keys[key] = b
	}
	b.seen = now
	return b.lim.AllowN(now, 1)
}

// limited reports a rate-limit error with a Retry-After hint.
func (s *server) limited(w http.ResponseWriter, r *http.Request, l *limiter) {
	w.Header().Set("Retry-After", itoa(int(l.every.Seconds())+1))
	s.fail(w, r, errRateLimited)
}
