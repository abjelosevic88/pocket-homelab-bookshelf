package metadata

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Cache keeps catalogue answers across restarts (the server stores them in SQLite).
type Cache interface {
	Get(key string) (value []byte, fetchedAt time.Time, ok bool)
	Put(key string, value []byte)
}

type memEntry struct {
	at  time.Time
	val any
}

var (
	memMu    sync.Mutex
	mem      = map[string]memEntry{}
	inflight = map[string]bool{}
	refreshQ = make(chan struct{}, 3) // background refreshes running at once
)

// cached returns the answer for key: from memory, else from the persistent
// cache, else by calling fetch. An answer older than ttl is still returned at
// once and refreshed in the background, so pages never wait on a catalogue
// for something they have shown before. Failed fetches are not cached.
func cached[T any](c *Client, key string, ttl time.Duration, fetch func(ctx context.Context) (T, error), ctx context.Context) (T, error) {
	memMu.Lock()
	e, ok := mem[key]
	memMu.Unlock()
	if !ok && c.Cache != nil {
		if raw, at, found := c.Cache.Get(key); found {
			var v T
			if json.Unmarshal(raw, &v) == nil {
				e, ok = memEntry{at: at, val: v}, true
				memMu.Lock()
				mem[key] = e
				memMu.Unlock()
			}
		}
	}
	if ok {
		if time.Since(e.at) > ttl {
			c.refresh(key, func(ctx context.Context) (any, error) { return fetch(ctx) })
		}
		return e.val.(T), nil
	}
	v, err := fetch(ctx)
	if err == nil {
		c.store(key, v)
	}
	return v, err
}

func (c *Client) store(key string, v any) {
	memMu.Lock()
	mem[key] = memEntry{at: time.Now(), val: v}
	memMu.Unlock()
	if c.Cache != nil {
		if raw, err := json.Marshal(v); err == nil {
			c.Cache.Put(key, raw)
		}
	}
}

// refresh fetches key again in the background, once at a time.
func (c *Client) refresh(key string, fetch func(ctx context.Context) (any, error)) {
	memMu.Lock()
	if inflight[key] {
		memMu.Unlock()
		return
	}
	inflight[key] = true
	memMu.Unlock()
	go func() {
		defer func() {
			memMu.Lock()
			delete(inflight, key)
			memMu.Unlock()
		}()
		refreshQ <- struct{}{}
		defer func() { <-refreshQ }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if v, err := fetch(ctx); err == nil {
			c.store(key, v)
		}
	}()
}
