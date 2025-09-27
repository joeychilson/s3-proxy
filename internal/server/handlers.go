package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joeychilson/s3-proxy/internal/cache"
	"github.com/joeychilson/s3-proxy/internal/origin"
)

func (s *Server) objectHandler(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(key, "..") {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	method := r.Method
	if method != http.MethodGet && method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	now := time.Now()
	useCache := shouldUseCache(r)
	lookupCache := useCache || method == http.MethodHead
	cKey := cacheKey(key)
	var entry *cache.Entry
	var ok bool
	if lookupCache {
		if entry, ok = s.cache.Get(cKey); ok {
			if entry.Fresh(now) {
				s.metrics.cacheHits.Inc()
				s.writeCacheEntry(w, r, entry, now, "HIT")
				return
			}
			if useCache && entry.StaleButValid(now) && method == http.MethodGet {
				s.metrics.cacheStales.Inc()
				s.writeCacheEntry(w, r, entry, now, "STALE")
				go s.revalidate(key, entry)
				return
			}
		}
	}

	cond := buildConditional(r)
	if entry != nil {
		if entry.ETag != "" && cond.IfNoneMatch == "" {
			cond.IfNoneMatch = entry.ETag
		}
		if !entry.LastModified.IsZero() && cond.IfModifiedSince == nil {
			lm := entry.LastModified
			cond.IfModifiedSince = &lm
		}
	}
	if method == http.MethodGet {
		cond.Range = r.Header.Get("Range")
	}

	obj, err := s.fetchFromOrigin(ctx, key, cond, method)
	if err != nil {
		s.handleOriginError(w, r, err, entry, now, cKey)
		return
	}
	if obj.Body != nil {
		defer obj.Body.Close()
	}

	shouldStore := useCache && method == http.MethodGet && cond.Range == "" && obj.StatusCode == http.StatusOK && obj.ContentLength > 0 && obj.ContentLength <= s.cfg.MaxObjectSize && !hasNoStore(obj.Headers)
	if shouldStore {
		body, readErr := io.ReadAll(io.LimitReader(obj.Body, s.cfg.MaxObjectSize+1))
		if readErr != nil {
			s.logger.Error("read origin body", "error", readErr, "key", key)
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
			return
		}
		if int64(len(body)) > s.cfg.MaxObjectSize {
			shouldStore = false
		} else {
			s.metrics.cacheMisses.Inc()
			e := &cache.Entry{
				Body:         append([]byte(nil), body...),
				Header:       cloneHeader(obj.Headers),
				Status:       obj.StatusCode,
				StoredAt:     now,
				TTL:          ttlFromHeaders(obj.Headers, s.cfg.CacheTTL),
				StaleTTL:     s.cfg.CacheStaleTTL,
				Size:         int64(len(body)),
				ETag:         obj.ETag,
				LastModified: valueOrZero(obj.LastModified),
			}
			if e.TTL <= 0 {
				e.TTL = s.cfg.CacheTTL
			}
			s.cache.Set(cKey, e)
			s.writeCacheEntry(w, r, e, now, "MISS")
			return
		}
	}

	copyHeaders(w.Header(), obj.Headers)
	w.Header().Set("X-Cache", "MISS")
	if obj.ContentLength > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
	}
	s.metrics.cacheMisses.Inc()
	w.WriteHeader(obj.StatusCode)
	if method == http.MethodHead {
		return
	}
	bytes, copyErr := io.Copy(w, obj.Body)
	if copyErr != nil {
		s.logger.Error("stream response", "error", copyErr, "key", key)
	}
	s.metrics.bytesServed.Add(float64(bytes))
}

func (s *Server) fetchFromOrigin(ctx context.Context, key string, cond *origin.Conditional, method string) (*origin.Object, error) {
	start := time.Now()
	if method == http.MethodHead {
		obj, err := s.origin.HeadObject(ctx, key, cond)
		if err == nil {
			s.metrics.originLatency.Observe(time.Since(start).Seconds())
		}
		return obj, err
	}
	obj, err := s.origin.GetObject(ctx, key, cond)
	if err == nil {
		s.metrics.originLatency.Observe(time.Since(start).Seconds())
	}
	return obj, err
}

func (s *Server) handleOriginError(w http.ResponseWriter, r *http.Request, err error, entry *cache.Entry, now time.Time, cacheKey string) {
	if errors.Is(err, origin.ErrNotModified) && entry != nil {
		entry.StoredAt = now
		s.cache.Set(cacheKey, entry)
		s.metrics.cacheHits.Inc()
		s.writeCacheEntry(w, r, entry, now, "REVALIDATED")
		return
	}
	if errors.Is(err, origin.ErrNotModified) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if errors.Is(err, origin.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, origin.ErrPrecondition) {
		http.Error(w, http.StatusText(http.StatusPreconditionFailed), http.StatusPreconditionFailed)
		return
	}
	s.metrics.originErrors.Inc()
	s.logger.Error("origin fetch failed", "error", err, "path", r.URL.Path)
	http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
}

func (s *Server) writeCacheEntry(w http.ResponseWriter, r *http.Request, entry *cache.Entry, now time.Time, state string) {
	copyHeaders(w.Header(), entry.Header)
	w.Header().Set("Age", strconv.Itoa(entry.Age(now)))
	w.Header().Set("X-Cache", state)
	w.WriteHeader(entry.Status)
	if r.Method == http.MethodHead {
		return
	}
	bytes, _ := w.Write(entry.Body)
	s.metrics.bytesServed.Add(float64(bytes))
}

func (s *Server) revalidate(key string, entry *cache.Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
	defer cancel()
	cond := &origin.Conditional{}
	if entry.ETag != "" {
		cond.IfNoneMatch = entry.ETag
	}
	if !entry.LastModified.IsZero() {
		lm := entry.LastModified
		cond.IfModifiedSince = &lm
	}
	obj, err := s.origin.GetObject(ctx, key, cond)
	if err != nil {
		if errors.Is(err, origin.ErrNotModified) {
			entry.StoredAt = time.Now()
			s.cache.Set(cacheKey(key), entry)
		}
		return
	}
	if obj.Body != nil {
		defer obj.Body.Close()
	}
	if obj.ContentLength <= 0 || obj.ContentLength > s.cfg.MaxObjectSize {
		return
	}
	body, err := io.ReadAll(io.LimitReader(obj.Body, s.cfg.MaxObjectSize+1))
	if err != nil {
		return
	}
	if int64(len(body)) > s.cfg.MaxObjectSize {
		return
	}
	updated := &cache.Entry{
		Body:         append([]byte(nil), body...),
		Header:       cloneHeader(obj.Headers),
		Status:       obj.StatusCode,
		StoredAt:     time.Now(),
		TTL:          ttlFromHeaders(obj.Headers, s.cfg.CacheTTL),
		StaleTTL:     s.cfg.CacheStaleTTL,
		Size:         int64(len(body)),
		ETag:         obj.ETag,
		LastModified: valueOrZero(obj.LastModified),
	}
	s.cache.Set(cacheKey(key), updated)
}

func (s *Server) purgeHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	for _, key := range payload.Keys {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		s.cache.Delete(cacheKey(k))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func shouldUseCache(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.Header.Get("Range") != "" {
		return false
	}
	cc := strings.ToLower(r.Header.Get("Cache-Control"))
	if strings.Contains(cc, "no-cache") || strings.Contains(cc, "max-age=0") {
		return false
	}
	pragma := strings.ToLower(r.Header.Get("Pragma"))
	if strings.Contains(pragma, "no-cache") {
		return false
	}
	return true
}

func cacheKey(key string) string {
	return key
}

func cloneHeader(h http.Header) http.Header {
	dup := make(http.Header, len(h))
	for k, v := range h {
		dup[k] = append([]string(nil), v...)
	}
	return dup
}

func copyHeaders(dst, src http.Header) {
	for k, v := range src {
		dst[k] = append([]string(nil), v...)
	}
}

func ttlFromHeaders(h http.Header, fallback time.Duration) time.Duration {
	if cc := h.Get("Cache-Control"); cc != "" {
		for part := range strings.SplitSeq(cc, ",") {
			part = strings.TrimSpace(strings.ToLower(part))
			if value, found := strings.CutPrefix(part, "max-age="); found {
				if secs, err := strconv.Atoi(value); err == nil {
					if secs <= 0 {
						return 0
					}
					return time.Duration(secs) * time.Second
				}
			}
		}
	}
	return fallback
}

func hasNoStore(h http.Header) bool {
	cc := strings.ToLower(h.Get("Cache-Control"))
	return strings.Contains(cc, "no-store")
}

func valueOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func buildConditional(r *http.Request) *origin.Conditional {
	cond := &origin.Conditional{}
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		cond.IfNoneMatch = inm
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if t, err := time.Parse(http.TimeFormat, ims); err == nil {
			cond.IfModifiedSince = &t
		}
	}
	return cond
}
