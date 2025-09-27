package cache

import (
	"net/http"
	"testing"
	"time"
)

func TestCacheSetGet(t *testing.T) {
	c, err := New(4, time.Second, time.Second)
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}

	entry := &Entry{
		Body:     []byte("hello"),
		Header:   http.Header{"Content-Type": {"text/plain"}},
		Status:   http.StatusOK,
		StoredAt: time.Now(),
		TTL:      time.Second,
		StaleTTL: 2 * time.Second,
	}

	c.Set("greeting", entry)

	got, ok := c.Get("greeting")
	if !ok {
		t.Fatalf("expected cache hit")
	}
	if string(got.Body) != "hello" {
		t.Fatalf("unexpected body %q", string(got.Body))
	}

	age := got.Age(time.Now())
	if age < 0 {
		t.Fatalf("age should not be negative")
	}
}

func TestFreshness(t *testing.T) {
	now := time.Now()
	entry := &Entry{TTL: time.Second, StaleTTL: 2 * time.Second, StoredAt: now.Add(-1500 * time.Millisecond)}
	if entry.Fresh(now) {
		t.Fatalf("entry should be stale")
	}
	if !entry.StaleButValid(now) {
		t.Fatalf("entry should be usable during stale window")
	}
	entry.StoredAt = now.Add(-4 * time.Second)
	if entry.StaleButValid(now) {
		t.Fatalf("entry should be expired")
	}
}
