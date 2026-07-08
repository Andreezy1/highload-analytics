package domain

import (
	"fmt"
	"time"
	"unicode/utf8"
)

type Event struct {
	UserID    int64     `db:"user_id"`
	EventType string    `db:"event_type"`
	Time      time.Time `db:"time"`
	PageUrl   string    `db:"page_url"`
}

func (e *Event) Validate() error {
	if e.UserID <= 0 {
		return fmt.Errorf("%w: user_id <= 0", ErrValidate)
	}
	if e.EventType == "" {
		return fmt.Errorf("%w: event_type is required", ErrValidate)
	}
	if utf8.RuneCountInString(e.EventType) >= 255 {
		return fmt.Errorf("%w: event_type len string more 255", ErrValidate)
	}
	if e.Time.IsZero() {
		return fmt.Errorf("%w: time is required", ErrValidate)
	}
	if e.PageUrl == "" {
		return fmt.Errorf("%w: page_url is required", ErrValidate)
	}
	return nil
}
