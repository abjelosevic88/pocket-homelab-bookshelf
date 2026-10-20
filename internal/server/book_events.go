package server

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

func (s *Server) bookStatusEvent(ctx context.Context, u *store.User, date string) func(*sql.Tx, *store.Book) error {
	return func(tx *sql.Tx, b *store.Book) error {
		if !s.core.Configured() {
			return nil
		}
		topic, when := "", date
		switch b.Status {
		case store.StatusReading:
			topic = "bookshelf.started"
			if when == "" {
				when = b.StartedOn
			}
		case store.StatusRead:
			topic = "bookshelf.finished"
			if when == "" {
				when = b.FinishedOn
			}
		case store.StatusAbandoned:
			topic = "bookshelf.abandoned"
		default:
			return nil
		}
		return core.QueueEvent(ctx, tx, topic, fmt.Sprintf("book:%d", b.ID), map[string]any{"book_id": b.ID, "title": b.Title, "authors": b.Authors, "status": b.Status, "date": when, "pages": b.Pages, "rating": b.Rating, "read_count": b.ReadCount, "series": b.Series, "user_id": b.UserID, "user_email": u.Email})
	}
}
func (s *Server) bookRatedEvent(ctx context.Context, u *store.User, b *store.Book, emit bool) core.CommitHook {
	if !emit || !s.core.Configured() {
		return nil
	}
	return func(tx *sql.Tx) error {
		return core.QueueEvent(ctx, tx, "bookshelf.rated", fmt.Sprintf("book:%d", b.ID), map[string]any{"book_id": b.ID, "title": b.Title, "authors": b.Authors, "rating": b.Rating, "status": b.Status, "favorite": b.Favorite, "user_id": b.UserID, "user_email": u.Email})
	}
}
