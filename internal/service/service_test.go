package service_test

import (
	"context"
	"errors"
	"highload-analytics/internal/domain"
	"highload-analytics/internal/service"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockRepository перехватывает батчи и сигналит о каждом успешном флаше
// в канал flushed, чтобы тесты синхронизировались без time.Sleep.
type mockRepository struct {
	mu           sync.Mutex
	savedBatches [][]domain.Event
	insertFunc   func(ctx context.Context, events []domain.Event) error
	flushed      chan []domain.Event
}

func newMockRepository() *mockRepository {
	return &mockRepository{flushed: make(chan []domain.Event, 16)}
}

func (m *mockRepository) InsertBatch(ctx context.Context, events []domain.Event) error {
	// Копия обязательна: воркер переиспользует слайс через batch[:0].
	cpy := make([]domain.Event, len(events))
	copy(cpy, events)

	if m.insertFunc != nil {
		if err := m.insertFunc(ctx, cpy); err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.savedBatches = append(m.savedBatches, cpy)
	m.mu.Unlock()

	m.flushed <- cpy
	return nil
}

func (m *mockRepository) batches() [][]domain.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]domain.Event, len(m.savedBatches))
	copy(out, m.savedBatches)
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validEvent(userID int64) domain.Event {
	return domain.Event{
		UserID:    userID,
		EventType: "click",
		Time:      time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC),
		PageURL:   "/home",
	}
}

func waitFlush(t *testing.T, repo *mockRepository) []domain.Event {
	t.Helper()
	select {
	case batch := <-repo.flushed:
		return batch
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flush")
		return nil
	}
}

// Флаш по достижении batchSize; тикер заведомо не срабатывает (1 час).
func TestService_FlushOnSizeTrigger(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 10, 3, 1, time.Hour, discardLogger())
	svc.Start(t.Context())
	defer svc.Stop()

	for i := int64(1); i <= 3; i++ {
		if err := svc.Create(t.Context(), validEvent(i)); err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
	}

	batch := waitFlush(t, repo)
	if len(batch) != 3 {
		t.Fatalf("want batch of 3 events, got %d", len(batch))
	}
}

// Флаш по тикеру: событий меньше batchSize, батч уходит по интервалу.
func TestService_FlushOnTicker(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 10, 100, 1, 20*time.Millisecond, discardLogger())
	svc.Start(t.Context())
	defer svc.Stop()

	if err := svc.Create(t.Context(), validEvent(1)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	batch := waitFlush(t, repo)
	if len(batch) != 1 {
		t.Fatalf("want batch of 1 event, got %d", len(batch))
	}
}

// Graceful shutdown: недобранный батч сбрасывается при Stop.
func TestService_DrainOnShutdown(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 10, 5, 1, time.Hour, discardLogger())
	svc.Start(t.Context())

	for i := int64(1); i <= 2; i++ {
		if err := svc.Create(t.Context(), validEvent(i)); err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
	}

	// Stop ждёт воркеров, так что после возврата финальный флаш гарантирован.
	svc.Stop()

	batches := repo.batches()
	if len(batches) != 1 {
		t.Fatalf("want exactly 1 drained batch, got %d", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Fatalf("want 2 events in drained batch, got %d", len(batches[0]))
	}
}

// Переполнение буфера: без воркеров второй Create в канал размера 1 не влезает.
func TestService_CreateQueueFull(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 1, 100, 1, time.Hour, discardLogger())
	// Start намеренно не вызывается — канал никто не вычитывает.

	if err := svc.Create(t.Context(), validEvent(1)); err != nil {
		t.Fatalf("first Create must fit into buffer: %v", err)
	}
	err := svc.Create(t.Context(), validEvent(2))
	if !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("want ErrQueueFull, got %v", err)
	}
}

// Невалидное событие отклоняется до постановки в очередь.
func TestService_CreateValidatesEvent(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 10, 100, 1, time.Hour, discardLogger())

	err := svc.Create(t.Context(), domain.Event{})
	if !errors.Is(err, domain.ErrValidate) {
		t.Fatalf("want ErrValidate, got %v", err)
	}
}

// После Stop сервис отвечает ErrQueueFull, а не паникует.
func TestService_CreateAfterStop(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 10, 100, 1, time.Hour, discardLogger())
	svc.Start(t.Context())
	svc.Stop()

	err := svc.Create(t.Context(), validEvent(1))
	if !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("want ErrQueueFull after Stop, got %v", err)
	}
}

// Повторный Stop не должен паниковать (двойной close канала).
func TestService_StopIsIdempotent(t *testing.T) {
	repo := newMockRepository()
	svc := service.NewService(repo, 10, 100, 1, time.Hour, discardLogger())
	svc.Start(t.Context())

	svc.Stop()
	svc.Stop()
}

// Регрессионный тест на гонку shutdown: конкурентные Create во время Stop
// не должны паниковать отправкой в закрытый канал (ловится под -race).
func TestService_ConcurrentCreateAndStop(t *testing.T) {
	repo := newMockRepository()
	// Дренируем сигнальный канал, чтобы его буфер не заблокировал воркеров.
	go func() {
		for range repo.flushed {
			continue
		}
	}()

	svc := service.NewService(repo, 1000, 10, 2, time.Hour, discardLogger())
	svc.Start(t.Context())

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func(producer int) {
			defer wg.Done()
			for i := int64(1); ; i++ {
				err := svc.Create(t.Context(), validEvent(int64(producer)*1_000_000+i))
				if err != nil && !errors.Is(err, domain.ErrQueueFull) {
					t.Errorf("unexpected Create error: %v", err)
					return
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		}(w)
	}

	// Короткая пауза только чтобы Stop пересёкся с активными Create;
	// корректность теста от её длительности не зависит.
	time.Sleep(10 * time.Millisecond)
	svc.Stop()
	close(stop)
	wg.Wait()

	if err := svc.Create(t.Context(), validEvent(1)); !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("want ErrQueueFull after Stop, got %v", err)
	}
}

// Ретрай: первая вставка падает, вторая успешна — батч не теряется.
func TestService_FlushRetriesTransientError(t *testing.T) {
	var calls atomic.Int64
	repo := newMockRepository()
	repo.insertFunc = func(_ context.Context, _ []domain.Event) error {
		if calls.Add(1) == 1 {
			return errors.New("transient: connection reset")
		}
		return nil
	}

	svc := service.NewService(repo, 10, 1, 1, time.Hour, discardLogger())
	svc.Start(t.Context())
	defer svc.Stop()

	if err := svc.Create(t.Context(), validEvent(1)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	batch := waitFlush(t, repo)
	if len(batch) != 1 {
		t.Fatalf("want batch of 1 event, got %d", len(batch))
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("want 2 InsertBatch calls (fail + retry), got %d", got)
	}
}

// Исчерпание ретраев: ровно maxRetries попыток, затем батч отбрасывается.
func TestService_FlushGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int64
	repo := newMockRepository()
	repo.insertFunc = func(_ context.Context, _ []domain.Event) error {
		calls.Add(1)
		return errors.New("permanent failure")
	}

	svc := service.NewService(repo, 10, 1, 1, time.Hour, discardLogger())
	svc.Start(t.Context())

	if err := svc.Create(t.Context(), validEvent(1)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Stop дожидается воркера, который к этому моменту прошёл все ретраи.
	svc.Stop()

	if got := calls.Load(); got != 3 {
		t.Fatalf("want exactly 3 InsertBatch attempts, got %d", got)
	}
	if batches := repo.batches(); len(batches) != 0 {
		t.Fatalf("batch must be dropped after retries, got %d saved batches", len(batches))
	}
}
