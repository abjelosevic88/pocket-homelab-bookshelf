package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// MetaCache keeps catalogue answers in the database (see 0006_meta_cache).
type MetaCache struct{ S *Store }

// Get returns a cached value and when it was fetched.
func (c MetaCache) Get(key string) ([]byte, time.Time, bool) {
	var v string
	var at int64
	err := c.S.DB.QueryRowContext(context.Background(), `SELECT value, fetched_at FROM meta_cache WHERE key = ?`, key).Scan(&v, &at)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.S.log.Warn("meta cache read", "err", err)
		}
		return nil, time.Time{}, false
	}
	return []byte(v), time.Unix(at, 0), true
}

// Put stores a value.
func (c MetaCache) Put(key string, value []byte) {
	_, err := c.S.DB.ExecContext(context.Background(), `INSERT INTO meta_cache (key, value, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, fetched_at = excluded.fetched_at`, key, string(value), time.Now().Unix())
	if err != nil {
		c.S.log.Warn("meta cache write", "err", err)
	}
}
