package handler

import (
	"highload-analytics/internal/domain"
	"time"
)

type eventDtoRequest struct {
	UserID    int64     `json:"user_id"`
	EventType string    `json:"event_type"`
	Time      time.Time `json:"time"`
	PageUrl   string    `json:"page_url"`
}

func newEventToDomain(event eventDtoRequest) domain.Event {
	return domain.Event{
		UserID:    event.UserID,
		EventType: event.EventType,
		Time:      event.Time,
		PageUrl:   event.PageUrl,
	}
}
