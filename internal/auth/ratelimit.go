package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter counts failed login attempts per key in a sliding window.
// In-memory is enough: one instance, ~20 users.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
	now    func() time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, fails: map[string][]time.Time{}, now: time.Now}
}

func (l *limiter) prune(key string, now time.Time) []time.Time {
	ts := l.fails[key]
	i := 0
	for i < len(ts) && now.Sub(ts[i]) > l.window {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.fails, key)
	} else {
		l.fails[key] = ts
	}
	return ts
}

// Blocked reports whether key is over the limit and how long until it frees up.
func (l *limiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	ts := l.prune(key, now)
	if len(ts) < l.max {
		return false, 0
	}
	return true, l.window - now.Sub(ts[0])
}

func (l *limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.fails[key] = append(l.prune(key, now), now)
	if len(l.fails) > 10000 { // bound memory under a spray of IPs
		for k := range l.fails {
			l.prune(k, now)
		}
	}
}

func (l *limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

// clientIP returns the caller's IP. Behind Railway's proxy (trustProxy) the
// right-most X-Forwarded-For entry is the one the proxy itself appended.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
