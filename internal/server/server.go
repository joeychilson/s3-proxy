package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/joeychilson/s3-proxy/internal/cache"
	"github.com/joeychilson/s3-proxy/internal/config"
	"github.com/joeychilson/s3-proxy/internal/origin"
)

type Server struct {
	cfg      *config.Config
	origin   *origin.Client
	cache    *cache.Cache
	metrics  *metrics
	logger   *slog.Logger
	registry *prometheus.Registry
	authTok  string
	limiter  *rateLimiter
	httpSrv  *http.Server
	once     sync.Once
}

func New(ctx context.Context, cfg *config.Config) (*Server, error) {
	originClient, err := origin.New(ctx, cfg.Endpoint, cfg.Region, cfg.AccessKey, cfg.SecretKey, cfg.Bucket, cfg.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("create origin client: %w", err)
	}

	cacheStore, err := cache.New(cfg.CacheCapacity, cfg.CacheTTL, cfg.CacheStaleTTL)
	if err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := newMetrics(registry)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	srv := &Server{
		cfg:      cfg,
		origin:   originClient,
		cache:    cacheStore,
		metrics:  m,
		logger:   logger,
		registry: registry,
		authTok:  cfg.AuthToken,
	}

	if cfg.RateLimitRPS > 0 {
		srv.limiter = newRateLimiter(cfg.RateLimitRPS, cfg.RateLimitRPS)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(srv.logMiddleware)
	if srv.limiter != nil {
		r.Use(srv.rateLimitMiddleware)
	}

	// Main endpoints
	r.Method(http.MethodGet, "/*", http.HandlerFunc(srv.objectHandler))
	r.Method(http.MethodHead, "/*", http.HandlerFunc(srv.objectHandler))

	// Admin endpoints
	r.With(srv.authMiddleware).Post("/cache/purge", srv.purgeHandler)
	r.With(srv.authMiddleware).Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	// Health check endpoint
	r.Get("/healthz", srv.healthHandler)

	srv.httpSrv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return srv, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		s.once.Do(func() {
			if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("server shutdown", "error", err)
			}
		})
	}()

	s.logger.Info("server starting", "addr", s.cfg.Addr)
	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
