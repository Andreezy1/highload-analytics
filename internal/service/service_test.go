package service

import (
	"context"
	"highload-analytics/internal/domain"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// MockRepository — фейковый репозиторий для перехвата батчей в тестах
type MockRepository struct {
	mu           sync.Mutex
	SavedBatches [][]domain.Event
	InsertFunc   func(ctx context.Context, events []domain.Event) error
}

func (m *MockRepository) InsertBatch(ctx context.Context, events []domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Копируем батч, чтобы избежать race condition при очистке в воркере
	cpy := make([]domain.Event, len(events))
	copy(cpy, events)
	m.SavedBatches = append(m.SavedBatches, cpy)

	if m.InsertFunc != nil {
		return m.InsertFunc(ctx, events)
	}
	return nil
}

// Помощник для создания сервиса в тестах
func newTestService(repo *MockRepository, batchSize int, interval time.Duration) *Service {
	return &Service{
		repo:          repo,
		logger:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
		batchSize:     batchSize,
		flushInterval: interval,
		eventChan:     make(chan domain.Event, batchSize*2),
	}
}

// ТЕСТ 1: Проверяем сброс батча при достижении лимита по размеру (Size Trigger)
func TestWorker_FlushOnSizeTrigger(t *testing.T) {
	mockRepo := &MockRepository{}
	// Ставим заведомо большой интервал тикера (1 час), чтобы он гарантированно не сработал во время теста
	svc := newTestService(mockRepo, 3, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.wg.Add(1)
	go svc.worker(ctx)

	// Отправляем 3 события (размер нашего батча)
	svc.eventChan <- domain.Event{UserID: 1, EventType: "click"}
	svc.eventChan <- domain.Event{UserID: 2, EventType: "view"}
	svc.eventChan <- domain.Event{UserID: 3, EventType: "purchase"}

	// Даем горутине воркера микросекунду переварить данные
	time.Sleep(10 * time.Millisecond)

	mockRepo.mu.Lock()
	batchesCount := len(mockRepo.SavedBatches)
	mockRepo.mu.Unlock()

	if batchesCount != 1 {
		t.Fatalf("Ожидали 1 сброшенный батч, получили %d. Триггер размера не сработал.", batchesCount)
	}

	if len(mockRepo.SavedBatches[0]) != 3 {
		t.Errorf("Ожидали 3 элемента в батче, получили %d", len(mockRepo.SavedBatches[0]))
	}
}

// ТЕСТ 2: Проверяем Graceful Shutdown — выгребание остатков (Drain) при закрытии канала
func TestWorker_DrainOnShutdown(t *testing.T) {
	mockRepo := &MockRepository{}
	// Опять отключаем тикер большим таймаутом
	svc := newTestService(mockRepo, 5, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.wg.Add(1)
	go svc.worker(ctx)

	// Отправляем всего 2 события (батч размером 5 еще НЕ заполнен)
	svc.eventChan <- domain.Event{UserID: 1, EventType: "click"}
	svc.eventChan <- domain.Event{UserID: 2, EventType: "view"}

	time.Sleep(5 * time.Millisecond)

	// В этот момент в базу ничего улететь не должно
	mockRepo.mu.Lock()
	if len(mockRepo.SavedBatches) != 0 {
		t.Errorf("Батч улетел раньше времени")
	}
	mockRepo.mu.Unlock()

	// Имитируем остановку приложения: закрываем канал, как это происходит при Graceful Shutdown
	close(svc.eventChan)
	svc.wg.Wait() // Ждем, пока воркер штатно завершит работу и выйдет из цикла

	// Теперь остатки (2 ивента) обязаны быть сохранены!
	mockRepo.mu.Lock()
	defer mockRepo.mu.Unlock()

	if len(mockRepo.SavedBatches) != 1 {
		t.Fatalf("После закрытия канала остатки данных потерялись. Ожидали 1 батч, получили %d", len(mockRepo.SavedBatches))
	}

	if len(mockRepo.SavedBatches[0]) != 2 {
		t.Errorf("Ожидали 2 «зависших» элемента в финальном батче, получили %d", len(mockRepo.SavedBatches[0]))
	}
}
