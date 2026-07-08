package service

import (
	"context"
	"errors"
	"highload-analytics/internal/domain"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

type EventRepository interface {
	InsertBatch(ctx context.Context, event []domain.Event) error
}

type Service struct {
	repo          EventRepository
	eventChan     chan domain.Event
	wg            sync.WaitGroup
	batchSize     int
	flushInterval time.Duration
	logger        *slog.Logger
}

func NewService(
	repo EventRepository,
	chanSize int,
	batchSize int,
	flushInterval time.Duration,
	logger *slog.Logger,
) *Service {
	return &Service{
		repo:          repo,
		eventChan:     make(chan domain.Event, chanSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		logger:        logger,
	}
}

func (s *Service) Create(ctx context.Context, event domain.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	select {
	case s.eventChan <- event:
		return nil
	default:
		return domain.ErrQueueFull
	}
}

func (s *Service) Start(ctx context.Context) {
	workerCount := runtime.NumCPU()
	for range workerCount {
		s.wg.Add(1)
		go s.worker(ctx)
	}
}

func (s *Service) Stop() {
	close(s.eventChan)
	s.wg.Wait()
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	batch := make([]domain.Event, 0, s.batchSize)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if len(batch) > 0 {
				s.flush(ctx, batch)
				batch = batch[:0]
			}
		case event, ok := <-s.eventChan:
			if !ok {
				if len(batch) > 0 {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					s.flush(shutdownCtx, batch)
					cancel()
				}
				return
			}
			batch = append(batch, event)
			if len(batch) >= s.batchSize {
				s.flush(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Service) flush(ctx context.Context, batch []domain.Event) {
	flushCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	maxRetries := 3
	backoff := 500 * time.Millisecond

	var err error
	for i := range maxRetries {
		if err = s.repo.InsertBatch(flushCtx, batch); err == nil {
			return
		}
		if errors.Is(flushCtx.Err(), context.DeadlineExceeded) || errors.Is(flushCtx.Err(), context.Canceled) {
			break
		}
		s.logger.Warn("Transient flush failure, retrying...",
			slog.Int("attempt", i+1),
			slog.Any("error", err),
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff *= 2
		}
	}
	s.logger.Error("CRITICAL: Failed to flush batch after all retries. Data lost!",
		slog.Any("error", err),
		slog.Int("batch_size", len(batch)),
	)
}
