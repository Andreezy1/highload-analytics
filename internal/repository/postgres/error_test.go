package postgres

import (
	"errors"
	"fmt"
	"highload-analytics/internal/domain"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapError(t *testing.T) {
	errOther := errors.New("connection reset")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "no rows maps to not found",
			err:  pgx.ErrNoRows,
			want: domain.ErrNotFound,
		},
		{
			name: "wrapped no rows maps to not found",
			err:  fmt.Errorf("query event: %w", pgx.ErrNoRows),
			want: domain.ErrNotFound,
		},
		{
			name: "unique violation maps to conflict",
			err:  &pgconn.PgError{Code: pgerrcode.UniqueViolation, Message: "duplicate key"},
			want: domain.ErrConflict,
		},
		{
			name: "check violation maps to validate",
			err:  &pgconn.PgError{Code: pgerrcode.CheckViolation, Message: "check failed"},
			want: domain.ErrValidate,
		},
		{
			name: "other pg error passes through unchanged",
			err:  &pgconn.PgError{Code: pgerrcode.ForeignKeyViolation, Message: "fk violation"},
			want: nil, // проверяется отдельно: ошибка возвращается как есть
		},
		{
			name: "non-pg error passes through unchanged",
			err:  errOther,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapError(tt.err)
			if got == nil {
				t.Fatal("mapError returned nil for non-nil error")
			}
			if tt.want != nil {
				if !errors.Is(got, tt.want) {
					t.Fatalf("mapError(%v) = %v, want wrapped %v", tt.err, got, tt.want)
				}
				return
			}
			// Немаппящиеся ошибки должны возвращаться без изменений,
			// чтобы не потерять исходную причину.
			if !errors.Is(got, tt.err) {
				t.Fatalf("mapError(%v) = %v, want original error preserved", tt.err, got)
			}
			if errors.Is(got, domain.ErrNotFound) || errors.Is(got, domain.ErrConflict) || errors.Is(got, domain.ErrValidate) {
				t.Fatalf("mapError(%v) unexpectedly mapped to a domain error: %v", tt.err, got)
			}
		})
	}
}
