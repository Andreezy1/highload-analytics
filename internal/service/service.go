package service

import (
	"context"
	"errors"
	"highload-analytics/internal/domain"
	"log/slog"
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
	workerCount   int
	logger        *slog.Logger

	mu       sync.RWMutex
	closed   bool
	stopOnce sync.Once
}

func NewService(
	repo EventRepository,
	chanSize int,
	batchSize int,
	workerCount int,
	flushInterval time.Duration,
	logger *slog.Logger,
) *Service {
	return &Service{
		repo:          repo,
		eventChan:     make(chan domain.Event, chanSize),
		batchSize:     batchSize,
		workerCount:   workerCount,
		flushInterval: flushInterval,
		logger:        logger,
	}
}

func (s *Service) Create(ctx context.Context, event domain.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return domain.ErrQueueFull
	}

	select {
	case s.eventChan <- event:
		return nil
	default:
		return domain.ErrQueueFull
	}
}

func (s *Service) Start(ctx context.Context) {
	for range s.workerCount {
		s.wg.Add(1)
		go s.worker(ctx)
	}
}

func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.eventChan)
		s.mu.Unlock()
		s.wg.Wait()
	})
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
				ticker.Reset(s.flushInterval)
			}
		}
	}
}

func (s *Service) flush(ctx context.Context, batch []domain.Event) {
	maxRetries := 3
	backoff := 500 * time.Millisecond
	var err error

	attempt := func() error {
		flushCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return s.repo.InsertBatch(flushCtx, batch)
	}
	for i := range maxRetries {
		if err = attempt(); err == nil {
			return
		}
		if errors.Is(err, domain.ErrValidate) || errors.Is(err, domain.ErrConflict) {
			s.logger.Error("Non-retryable flush error, dropping batch",
				slog.Any("error", err),
				slog.Int("batch_size", len(batch)),
			)
			return
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.logger.Error("Context expired during batch flush, processing halted",
				slog.Any("error", ctx.Err()),
				slog.Int("batch_size", len(batch)),
			)
			return
		}
		s.logger.Warn("Transient flush failure, retrying...",
			slog.Int("attempt", i+1),
			slog.Any("error", err),
		)

		select {
		case <-ctx.Done():
			s.logger.Error("Context canceled during backoff sleep, batch lost",
				slog.Any("error", ctx.Err()),
				slog.Int("batch_size", len(batch)),
			)
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
