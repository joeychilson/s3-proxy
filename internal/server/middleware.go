package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"size", rw.bytes,
			"duration", duration.String(),
			"remote", r.RemoteAddr,
		)
	})
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)
		limiter := s.limiter.get(ip)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkToken(r, s.authTok) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	})
}

func checkToken(r *http.Request, expected string) bool {
	if expected == "" {
		return true
	}
	token := r.Header.Get("X-Auth-Token")
	if token == "" {
		auth := r.Header.Get("Authorization")
		if value, found := strings.CutPrefix(strings.ToLower(auth), "bearer "); found {
			token = strings.TrimSpace(value)
		}
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return subtleConstantTimeEquals(token, expected)
}

func subtleConstantTimeEquals(a, b string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

type rateLimiter struct {
	limit float64
	burst float64
	mu    sync.Mutex
	store map[string]*rate.Limiter
}

func newRateLimiter(limit, burst float64) *rateLimiter {
	if burst < limit {
		burst = limit
	}
	return &rateLimiter{limit: limit, burst: burst, store: make(map[string]*rate.Limiter)}
}

func (r *rateLimiter) get(key string) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limiter, ok := r.store[key]; ok {
		return limiter
	}
	limiter := rate.NewLimiter(rate.Limit(r.limit), int(r.burst))
	r.store[key] = limiter
	return limiter
}

func realIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		for part := range strings.SplitSeq(xf, ",") {
			return strings.TrimSpace(part)
		}
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	return r.RemoteAddr
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += int64(n)
	return n, err
}
