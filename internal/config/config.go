package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr           string
	Bucket         string
	Region         string
	Endpoint       string
	AccessKey      string
	SecretKey      string
	CacheCapacity  int
	CacheTTL       time.Duration
	CacheStaleTTL  time.Duration
	MaxObjectSize  int64
	AuthToken      string
	RequestTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	RateLimitRPS   float64
}

const (
	defaultAddr           = ":8080"
	defaultCacheCapacity  = 2048
	defaultCacheTTL       = 5 * time.Minute
	defaultCacheStaleTTL  = 2 * time.Minute
	defaultMaxObjectSize  = 16 * 1024 * 1024 // 16 MiB
	defaultRequestTimeout = 15 * time.Second
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 15 * time.Second
	defaultIdleTimeout    = 60 * time.Second
	defaultRateLimitRPS   = 0 // disabled by default
)

func Load() (*Config, error) {
	cfg := &Config{
		Addr:           getString("SERVER_ADDR", defaultAddr),
		AuthToken:      os.Getenv("AUTH_TOKEN"),
		Endpoint:       os.Getenv("S3_ENDPOINT"),
		Region:         getString("S3_REGION", "auto"),
		AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("S3_SECRET_KEY"),
		Bucket:         os.Getenv("S3_BUCKET"),
		CacheCapacity:  getInt("CACHE_CAPACITY", defaultCacheCapacity),
		CacheTTL:       getDuration("CACHE_TTL", defaultCacheTTL),
		CacheStaleTTL:  getDuration("CACHE_STALE_TTL", defaultCacheStaleTTL),
		MaxObjectSize:  getInt64("MAX_OBJECT_SIZE", defaultMaxObjectSize),
		RequestTimeout: getDuration("REQUEST_TIMEOUT", defaultRequestTimeout),
		ReadTimeout:    getDuration("READ_TIMEOUT", defaultReadTimeout),
		WriteTimeout:   getDuration("WRITE_TIMEOUT", defaultWriteTimeout),
		IdleTimeout:    getDuration("IDLE_TIMEOUT", defaultIdleTimeout),
		RateLimitRPS:   getFloat("RATE_LIMIT_RPS", defaultRateLimitRPS),
	}

	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("AUTH_TOKEN must be provided")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("S3_ENDPOINT must be provided")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY must be provided")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET must be provided")
	}

	if cfg.CacheCapacity <= 0 {
		return nil, fmt.Errorf("CACHE_CAPACITY must be greater than zero")
	}
	if cfg.CacheTTL <= 0 {
		return nil, fmt.Errorf("CACHE_TTL must be greater than zero")
	}
	if cfg.CacheStaleTTL < 0 {
		return nil, fmt.Errorf("CACHE_STALE_TTL must be zero or positive")
	}
	if cfg.MaxObjectSize <= 0 {
		return nil, fmt.Errorf("MAX_OBJECT_SIZE must be greater than zero")
	}
	if cfg.RateLimitRPS < 0 {
		return nil, fmt.Errorf("RATE_LIMIT_RPS must be zero or positive")
	}

	return cfg, nil
}

func getString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func getInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed
		}
	}
	return def
}

func getFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		dur, err := time.ParseDuration(v)
		if err == nil {
			return dur
		}
	}
	return def
}
