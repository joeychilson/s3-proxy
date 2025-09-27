package cache

import (
	"net/http"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type Entry struct {
	Body         []byte
	Header       http.Header
	Status       int
	StoredAt     time.Time
	TTL          time.Duration
	StaleTTL     time.Duration
	Size         int64
	ETag         string
	LastModified time.Time
}

func (e *Entry) Fresh(now time.Time) bool {
	return now.Before(e.StoredAt.Add(e.TTL))
}

func (e *Entry) StaleButValid(now time.Time) bool {
	return now.Before(e.StoredAt.Add(e.TTL + e.StaleTTL))
}

func (e *Entry) Age(now time.Time) int {
	if now.Before(e.StoredAt) {
		return 0
	}
	return int(now.Sub(e.StoredAt).Seconds())
}

type Cache struct {
	mu    sync.RWMutex
	lru   *lru.Cache[string, *Entry]
	ttl   time.Duration
	stale time.Duration
	cap   int
}

func New(capacity int, ttl, stale time.Duration) (*Cache, error) {
	l, err := lru.New[string, *Entry](capacity)
	if err != nil {
		return nil, err
	}
	return &Cache{lru: l, ttl: ttl, stale: stale, cap: capacity}, nil
}

func (c *Cache) Get(key string) (*Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.lru.Get(key)
	if !ok {
		return nil, false
	}
	return entry, true
}

func (c *Cache) Set(key string, entry *Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry.TTL == 0 {
		entry.TTL = c.ttl
	}
	if entry.StaleTTL == 0 {
		entry.StaleTTL = c.stale
	}
	c.lru.Add(key, entry)
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Remove(key)
}

func (c *Cache) Stats() (size int, capacity int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len(), c.cap
}
