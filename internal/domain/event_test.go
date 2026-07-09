package domain_test

import (
	"errors"
	"highload-analytics/internal/domain"
	"strings"
	"testing"
	"time"
)

func validEvent() domain.Event {
	return domain.Event{
		UserID:    1,
		EventType: "click",
		Time:      time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC),
		PageURL:   "/home",
	}
}

func TestEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(e *domain.Event)
		wantErr bool
	}{
		{
			name:    "valid event",
			mutate:  func(e *domain.Event) {},
			wantErr: false,
		},
		{
			name:    "user_id zero",
			mutate:  func(e *domain.Event) { e.UserID = 0 },
			wantErr: true,
		},
		{
			name:    "user_id negative",
			mutate:  func(e *domain.Event) { e.UserID = -1 },
			wantErr: true,
		},
		{
			name:    "empty event_type",
			mutate:  func(e *domain.Event) { e.EventType = "" },
			wantErr: true,
		},
		{
			name:    "event_type exactly 255 runes is allowed",
			mutate:  func(e *domain.Event) { e.EventType = strings.Repeat("a", 255) },
			wantErr: false,
		},
		{
			name:    "event_type 256 runes is rejected",
			mutate:  func(e *domain.Event) { e.EventType = strings.Repeat("a", 256) },
			wantErr: true,
		},
		{
			name:    "event_type 255 multibyte runes is allowed",
			mutate:  func(e *domain.Event) { e.EventType = strings.Repeat("я", 255) },
			wantErr: false,
		},
		{
			name:    "zero time",
			mutate:  func(e *domain.Event) { e.Time = time.Time{} },
			wantErr: true,
		},
		{
			name:    "empty page_url",
			mutate:  func(e *domain.Event) { e.PageURL = "" },
			wantErr: true,
		},
		{
			name:    "page_url exactly 255 runes is allowed",
			mutate:  func(e *domain.Event) { e.PageURL = "/" + strings.Repeat("a", 254) },
			wantErr: false,
		},
		{
			name:    "page_url 256 runes is rejected",
			mutate:  func(e *domain.Event) { e.PageURL = "/" + strings.Repeat("a", 255) },
			wantErr: true,
		},
		{
			name:    "page_url 255 multibyte runes is allowed",
			mutate:  func(e *domain.Event) { e.PageURL = strings.Repeat("я", 255) },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent()
			tt.mutate(&event)

			err := event.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error, got nil")
				}
				if !errors.Is(err, domain.ErrValidate) {
					t.Fatalf("error %v is not wrapped with ErrValidate", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
