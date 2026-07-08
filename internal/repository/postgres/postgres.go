package postgres

import (
	"context"
	"errors"
	"highload-analytics/internal/domain"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *Repository) insertRowByRowFallback(
	ctx context.Context,
	events []domain.Event,
	batchErr error,
) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return batchErr
	}

	query := `INSERT INTO events (user_id, event_type, time, page_url)
			  VALUES ($1,$2,$3,$4)`

	for _, event := range events {
		_, err := r.db.Exec(ctx, query, event.UserID, event.EventType, event.Time, event.PageUrl)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
				return err
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				slog.Error("Poison pill event dropped during fallback insertion",
					slog.String("sql_state", pgErr.Code),
					slog.Any("error", err),
					slog.Any("failed_event", event),
				)
				continue
			}
			return err
		}

	}
	return nil
}
