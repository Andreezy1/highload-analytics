package domain

import (
	"fmt"
	"time"
	"unicode/utf8"
)

type Event struct {
	UserID    int64
	EventType string
	Time      time.Time
	PageURL   string
}

func (e *Event) Validate() error {
	if e.UserID <= 0 {
		return fmt.Errorf("%w: user_id <= 0", ErrValidate)
	}
	if e.EventType == "" {
		return fmt.Errorf("%w: event_type is required", ErrValidate)
	}
	if utf8.RuneCountInString(e.EventType) > 255 {
		return fmt.Errorf("%w: event_type exceeds 255 characters", ErrValidate)
	}
	if e.Time.IsZero() {
		return fmt.Errorf("%w: time is required", ErrValidate)
	}
	if e.PageURL == "" {
		return fmt.Errorf("%w: page_url is required", ErrValidate)
	}
	if utf8.RuneCountInString(e.PageURL) > 255 {
		return fmt.Errorf("%w: page_url exceeds 255 characters", ErrValidate)
	}
	return nil
}
