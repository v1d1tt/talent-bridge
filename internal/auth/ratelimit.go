package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter counts sign-in requests per client IP in fixed one-minute windows.
// It is per server instance (at most 2 run), which is enough to stop one
// client from hammering the sign-in routes and, through them, Google.
type limiter struct {
	perMinute int // 0 or less: no limit

	mu   sync.Mutex
	hits map[string]*window
}

type window struct {
	start time.Time
	n     int
}

func (l *limiter) allow(r *http.Request) bool {
	if l.perMinute <= 0 {
		return true
	}
	ip, now := clientIP(r), time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil || len(l.hits) > 10_000 { // a crude cap on memory
		l.hits = map[string]*window{}
	}
	w := l.hits[ip]
	if w == nil || now.Sub(w.start) >= time.Minute {
		w = &window{start: now}
		l.hits[ip] = w
	}
	w.n++
	return w.n <= l.perMinute
}

// clientIP is the caller's IP. Behind the Azure Container Apps ingress the
// TCP peer is the proxy; the proxy appends the real client address as the
// last X-Forwarded-For entry (earlier entries come from the client and can
// be forged).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
