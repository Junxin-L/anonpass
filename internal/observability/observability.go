package observability

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu        sync.Mutex
	requests  map[string]int64
	durations map[string][]time.Duration
}

func NewMetrics() *Metrics {
	return &Metrics{
		requests:  make(map[string]int64),
		durations: make(map[string][]time.Duration),
	}
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		key := r.Method + " " + routeKey(r.URL.Path)
		m.mu.Lock()
		m.requests[key+fmt.Sprintf(" %d", rec.status)]++
		m.durations[key] = append(m.durations[key], time.Since(start))
		m.mu.Unlock()
	})
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		keys := make([]string, 0, len(m.requests))
		for key := range m.requests {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			method, route, status := splitRequestKey(key)
			fmt.Fprintf(w, "anonpass_requests_total{method=%q,route=%q,status=%q} %d\n", method, route, status, m.requests[key])
		}
		routeKeys := make([]string, 0, len(m.durations))
		for key := range m.durations {
			routeKeys = append(routeKeys, key)
		}
		sort.Strings(routeKeys)
		for _, key := range routeKeys {
			samples := append([]time.Duration(nil), m.durations[key]...)
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			method, route := splitRouteKey(key)
			fmt.Fprintf(w, "anonpass_request_duration_seconds{method=%q,route=%q,quantile=%q} %f\n", method, route, "0.50", secondsAt(samples, 50))
			fmt.Fprintf(w, "anonpass_request_duration_seconds{method=%q,route=%q,quantile=%q} %f\n", method, route, "0.95", secondsAt(samples, 95))
			fmt.Fprintf(w, "anonpass_request_duration_seconds{method=%q,route=%q,quantile=%q} %f\n", method, route, "0.99", secondsAt(samples, 99))
		}
	})
}

type RateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimiter(rate, burst float64) *RateLimiter {
	return &RateLimiter{
		rate:    rate,
		burst:   burst,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	if l.rate <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ip == "" {
			ip = "unknown"
		}
		if !l.allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b := l.buckets[ip]
	if b == nil {
		l.buckets[ip] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type BodyLimit struct {
	Bytes int64
}

func (b BodyLimit) Middleware(next http.Handler) http.Handler {
	if b.Bytes <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, b.Bytes)
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func routeKey(path string) string {
	switch {
	case path == "/metrics":
		return "/metrics"
	case strings.HasPrefix(path, "/v1/issuer/"):
		return "/v1/issuer/*"
	case strings.HasPrefix(path, "/v1/gateway/"):
		return "/v1/gateway/*"
	case strings.HasPrefix(path, "/v1/demo/"):
		return "/v1/demo/*"
	default:
		return path
	}
}

func splitRequestKey(key string) (string, string, string) {
	parts := strings.SplitN(key, " ", 3)
	if len(parts) != 3 {
		return "unknown", "unknown", "0"
	}
	return parts[0], parts[1], parts[2]
}

func splitRouteKey(key string) (string, string) {
	parts := strings.SplitN(key, " ", 2)
	if len(parts) != 2 {
		return "unknown", "unknown"
	}
	return parts[0], parts[1]
}

func secondsAt(samples []time.Duration, p int) float64 {
	if len(samples) == 0 {
		return 0
	}
	idx := (len(samples) - 1) * p / 100
	return samples[idx].Seconds()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
