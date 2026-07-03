package postgres

import (
	"context"
	"highload-analytics/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) InsertBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(events))
	for _, event := range events {
		rows = append(rows, []any{
			event.UserID,
			event.EventType,
			event.Time,
			event.PageUrl,
		})
	}

	_, err := r.db.CopyFrom(
		ctx,
		pgx.Identifier{"events"},
		[]string{"user_id", "event_type", "time", "page_url"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return mapError(err)
	}
	return nil
}
