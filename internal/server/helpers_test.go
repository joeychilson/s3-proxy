package server

import (
	"net/http"
	"testing"
)

func TestShouldUseCache(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/object", nil)
	if !shouldUseCache(req) {
		t.Fatalf("expected cache usage")
	}
	req.Header.Set("Range", "bytes=0-1")
	if shouldUseCache(req) {
		t.Fatalf("range requests should bypass cache")
	}
	req.Header.Del("Range")
	req.Header.Set("Cache-Control", "no-cache")
	if shouldUseCache(req) {
		t.Fatalf("no-cache directive should bypass cache")
	}
}

func TestTTLFromHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("Cache-Control", "max-age=60")
	if ttl := ttlFromHeaders(headers, 0); ttl.Seconds() != 60 {
		t.Fatalf("expected ttl 60 got %v", ttl)
	}
	headers.Set("Cache-Control", "no-store")
	if ttl := ttlFromHeaders(headers, 10); ttl != 10 {
		t.Fatalf("fallback ttl expected, got %v", ttl)
	}
}

func TestHasNoStore(t *testing.T) {
	headers := http.Header{}
	headers.Set("Cache-Control", "public")
	if hasNoStore(headers) {
		t.Fatalf("should not report no-store")
	}
	headers.Set("Cache-Control", "public, no-store")
	if !hasNoStore(headers) {
		t.Fatalf("expected no-store detection")
	}
}

func TestCloneHeader(t *testing.T) {
	original := http.Header{"X-Test": {"value"}}
	copy := cloneHeader(original)
	copy.Set("X-Test", "modified")
	if original.Get("X-Test") != "value" {
		t.Fatalf("expected deep copy to leave original intact")
	}
}
