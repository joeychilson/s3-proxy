package config

import "testing"

func TestLoadMissingRequired(t *testing.T) {
	for _, key := range []string{"AUTH_TOKEN", "S3_BUCKET", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
		t.Setenv(key, "")
	}
	if _, err := Load(); err == nil {
		t.Fatalf("expected error for missing required configuration")
	}
}

func TestLoadSuccess(t *testing.T) {
	t.Setenv("AUTH_TOKEN", "token")
	t.Setenv("S3_ENDPOINT", "https://example.com")
	t.Setenv("S3_BUCKET", "bucket")
	t.Setenv("S3_ACCESS_KEY", "AKIA")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("CACHE_CAPACITY", "128")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CacheCapacity != 128 {
		t.Fatalf("expected cache capacity 128, got %d", cfg.CacheCapacity)
	}
	if cfg.Bucket != "bucket" {
		t.Fatalf("unexpected bucket %s", cfg.Bucket)
	}
}
