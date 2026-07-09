package handler

import (
	"errors"
	"fmt"
	"highload-analytics/internal/domain"
	"net/http"
	"testing"
)

func TestStatusFromErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "not found",
			err:  domain.ErrNotFound,
			want: http.StatusNotFound,
		},
		{
			name: "wrapped not found",
			err:  fmt.Errorf("get event: %w", domain.ErrNotFound),
			want: http.StatusNotFound,
		},
		{
			name: "conflict",
			err:  domain.ErrConflict,
			want: http.StatusConflict,
		},
		{
			name: "wrapped conflict",
			err:  fmt.Errorf("%w: duplicate key", domain.ErrConflict),
			want: http.StatusConflict,
		},
		{
			name: "validation",
			err:  domain.ErrValidate,
			want: http.StatusBadRequest,
		},
		{
			name: "wrapped validation",
			err:  fmt.Errorf("%w: user_id <= 0", domain.ErrValidate),
			want: http.StatusBadRequest,
		},
		{
			name: "queue full",
			err:  domain.ErrQueueFull,
			want: http.StatusServiceUnavailable,
		},
		{
			name: "unknown error maps to internal",
			err:  errors.New("boom"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusFromErr(tt.err); got != tt.want {
				t.Fatalf("statusFromErr(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
