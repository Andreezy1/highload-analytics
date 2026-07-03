package handler

import (
	"context"
	"highload-analytics/internal/domain"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type EventService interface {
	Create(ctx context.Context, event domain.Event) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type Handler struct {
	service EventService
	logger  *slog.Logger
}

func NewHandler(service EventService, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var event eventDtoRequest
	if err := h.newDecoder(w, r, &event); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Create(r.Context(), newEventToDomain(event)); err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusAccepted, nil)
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/event", func(r chi.Router) {
		r.Post("/", h.create)
	})
}
