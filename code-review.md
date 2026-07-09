# Код-ревью: highload-analytics — оставшаяся работа

Дата актуализации: 2026-07-09. `go build`, `go vet`, `go test -race ./...` — зелёные.

Уже исправлено и удалено из этого файла: panic «send on closed channel» (guard + `sync.Once`), poison-pill батчи (валидация длины `page_url`, off-by-one, подключённый fallback c `mapError`), per-attempt таймаут и логирование потерь во `flush`, конфигурируемый `WORKER_COUNT` вместо `runtime.NumCPU()`, `ticker.Reset` после size-флаша, `db:`-теги из домена, `PageURL`/`decodeJSON`/`RATE_LIMIT_WINDOW`/`svc`, комментарии в compose, `slog.SetDefault`.

Ниже — только то, что осталось (включая две проблемы, появившиеся при исправлениях), с примерами фиксов.

---

## 1. CRITICAL (новая регрессия) — pprof-горутина запускает основной сервер вторым экземпляром

**Где:** `cmd/app/main.go:26-31`.

**Проблема:** при исправлении экспозиции pprof вызов заменили на `app.Server.ListenAndServe()` — это **основной** HTTP-сервер приложения, который параллельно запускает `run()` (`app.go:82`). Две горутины гонятся за бинд `:8080`: проигравшая падает с `address already in use` и пишет в лог «pprof server failed». А сам pprof теперь не слушает нигде — `_ "net/http/pprof"` регистрирует хендлеры в `DefaultServeMux`, который никто не обслуживает. Заодно в логе осталась опечатка `"^6060"`.

**Как исправить** — отдельный сервер с собственным mux на loopback:

```go
// main.go
go func() {
    mux := http.NewServeMux()
    mux.HandleFunc("/debug/pprof/", pprof.Index)         // import "net/http/pprof"
    mux.HandleFunc("/debug/pprof/profile", pprof.Profile) // (обычный import, не blank)
    mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
    mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

    logger.Info("starting pprof server", slog.String("addr", "localhost:6060"))
    if err := http.ListenAndServe("localhost:6060", mux); err != nil {
        logger.Error("pprof server failed", slog.Any("error", err))
    }
}()
```

Blank-import `_ "net/http/pprof"` при этом убрать — иначе хендлеры останутся и в `DefaultServeMux`.

Примечание для Docker: `localhost` внутри контейнера недостижим снаружи, поэтому маппинг `"6060:6060"` в `docker-compose.yaml:52` мёртвый — см. пункт 6.

## 2. High — `.env` с паролем БД закоммичен в git

**Где:** `.env:5` (`DB_PASSWORD=secret`), файл отслеживается git, `.gitignore` отсутствует.

**Как исправить:**

```bash
git rm --cached .env
printf '.env\n' >> .gitignore
cp .env .env.example   # и заменить в нём реальные значения на плейсхолдеры
git add .gitignore .env.example
```

В `.env.example`:

```
DB_PASSWORD=changeme
```

Если репозиторий когда-либо публиковался — пароль считать скомпрометированным и сменить.

## 3. High — rate-limit обходится подделкой `X-Forwarded-For`; дефолтный лимит выключает лимитер

**Где:** `internal/handler/middleware.go:11-32`, `config/config.go:25`.

**Проблема:** `getClientIP` безусловно верит `X-Forwarded-For` и `X-Real-IP`. Без доверенного прокси клиент шлёт случайный XFF в каждом запросе → бесконечный лимит + мусорные ключи в Redis. Плюс `CLIENT_LIMIT` по умолчанию 1 000 000 за 60s — лимитер фактически отключён из коробки. Ещё: `len(parts) > 0` на строке 14 — мёртвая проверка (`strings.Split` всегда возвращает ≥1 элемент).

**Как исправить** — доверять заголовкам только за прокси, управляя этим конфигом:

```go
// config: TrustProxyHeaders bool `env:"TRUST_PROXY_HEADERS" envDefault:"false"`
//         ClientLimit int `env:"CLIENT_LIMIT" envDefault:"1000"`

func getClientIP(r *http.Request, trustProxy bool) string {
    if trustProxy {
        if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
            ip, _, _ := strings.Cut(xff, ",") // первый адрес в цепочке
            if ip = strings.TrimSpace(ip); ip != "" {
                return ip
            }
        }
        if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
            return xri
        }
    }
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return ip
}
```

Fail-open при недоступном Redis (`middleware.go:43-50`) — допустимый трейдофф для аналитики, но зафиксируйте его комментарием и, в идеале, метрикой.

## 4. Medium (новое) — обёртка ошибок подключений через `%v` вместо `%w`

**Где:** `internal/repository/postgres/connect.go:27`, `internal/repository/redislimiter/connect.go:16`.

**Проблема:** контекст добавили, но `fmt.Errorf("%v: failed connect postgres", err)` **рвёт цепочку ошибок**: `%v` стирает тип, `errors.Is/As` по обёрнутой ошибке перестают работать. Порядок тоже перевёрнут — принято «что делали: %w». `errorlint` из вашего `.golangci.yml` это флагует. Кроме того, в `postgres/connect.go` первая ветка (`pgxpool.New`) по-прежнему возвращает голую ошибку.

**Как исправить:**

```go
// postgres/connect.go
pool, err := pgxpool.New(ctx, dsn)
if err != nil {
    return nil, fmt.Errorf("create postgres pool: %w", err)
}
if err := pool.Ping(ctx); err != nil {
    pool.Close()
    return nil, fmt.Errorf("ping postgres: %w", err)
}

// redislimiter/connect.go
if err := rdb.Ping(ctx).Err(); err != nil {
    return nil, fmt.Errorf("ping redis: %w", err)
}
```

## 5. Medium (новое) — `defer cancel()` внутри цикла ретраев

**Где:** `internal/service/service.go:122-123`.

**Проблема:** `defer` в цикле — отмены копятся до выхода из `flush`, контексты первых попыток не освобождаются, пока не завершатся все. Утечка ограничена тремя итерациями (на практике безвредна), но это антипаттерн, который ловит правило `defer: [loop]` из вашего `.golangci.yml`.

**Как исправить** — вынести попытку в функцию (заодно удобно добавить классификацию ошибок, пункт 7):

```go
attempt := func() error {
    flushCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()
    return s.repo.InsertBatch(flushCtx, batch)
}

for i := range maxRetries {
    if err = attempt(); err == nil {
        return
    }
    // ... остальная логика ретрая без изменений
}
```

## 6. Medium — мёртвый маппинг `6060:6060` в compose

**Где:** `docker-compose.yaml:51-52`.

**Проблема:** после переноса pprof на loopback (пункт 1) порт изнутри контейнера наружу не пробрасывается — маппинг не работает и вводит в заблуждение.

**Как исправить** (вариант для контейнера): внутри контейнера слушать `:6060`, а ограничение доступа сделать на публикации порта — только на loopback хоста:

```yaml
    ports:
      - "8080:8080"
      - "127.0.0.1:6060:6060"   # pprof доступен только с хоста
```

и в `main.go` для Docker-окружения слушать `":6060"` (адрес можно вынести в `PPROF_ADDR` env с дефолтом `localhost:6060`).

## 7. Medium — `flush` ретраит non-retryable ошибки

**Где:** `internal/service/service.go:121-149`.

**Проблема:** `mapError` уже переводит pg-ошибки в `domain.ErrValidate`/`domain.ErrConflict`, но `flush` ретраит всё подряд — ошибку валидации бессмысленно повторять 3 раза с бэкоффом.

**Как исправить:**

```go
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
```

Смежное (nice-to-have): после исчерпания ретраев батч теряется безвозвратно — для устойчивости к рестарту Postgres рассмотреть спилл на диск / DLQ.

## 8. Medium — тесты: оставшееся покрытие

✅ **Сделано (2026-07-09):** табличные тесты `Event.Validate` (`internal/domain/event_test.go`), `statusFromErr` (`internal/handler/utility_test.go`), `mapError` (`internal/repository/postgres/error_test.go`); `service_test.go` переписан через публичный API с синхронизацией каналами вместо `time.Sleep` — 10 тестов, включая флаш по тикеру, `ErrQueueFull`, ретраи, идемпотентный `Stop` и регрессионный тест на shutdown-гонку под `-race`.

**Осталось:**
- `internal/handler` — httptest для `POST /event` (202 / битый JSON → 400 / неизвестное поле → 400 / тело >1MB / `ErrQueueFull` → 503 / произвольная ошибка → 500 без утечки деталей) и для `RateLimiterMiddleware` (429, fail-open) + табличный `getClientIP`;
- `internal/repository/postgres` — интеграционный тест `insertRowByRowFallback` (testcontainers или compose + build tag `integration`): батч с одним poison-pill событием → остальные вставлены;
- после добавления классификации ошибок во `flush` (пункт 7) — тест, что `ErrValidate` не ретраится (ровно 1 вызов `InsertBatch`).

## 9. Low — `Start(ctx)` вводит в заблуждение

**Где:** `internal/service/service.go:68-73, 85-114`.

**Проблема:** воркер не имеет ветки `<-ctx.Done()` — отмена переданного контекста ничего не останавливает (выход только по закрытию канала). Передаётся `context.Background()`, так что сейчас это не баг, а мина для следующего разработчика.

**Как исправить** — проще всего убрать параметр:

```go
func (s *Service) Start() {
    for range s.workerCount {
        s.wg.Add(1)
        go s.worker(context.Background())
    }
}
```

Либо добавить в `select` воркера ветку `case <-ctx.Done():` с дренажем канала и финальным флашем.

## 10. Low — репозитории зависят от всего `*config.Config`

**Где:** `internal/repository/postgres/connect.go:11`, `internal/repository/redislimiter/connect.go:11`.

**Проблема:** инфраструктурный слой видит `ChanSize`, `BatchSize`, HTTP-порт — лишняя связность.

**Как исправить** — передавать только нужное:

```go
// postgres/connect.go
func NewPostgresConnect(ctx context.Context, dsn string) (*pgxpool.Pool, error) { ... }

// config/config.go
func (c *Config) PostgresDSN() string {
    return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
        c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

// redislimiter/connect.go
func NewRedisConnect(ctx context.Context, addr string) (*redis.Client, error) { ... }
```

## 11. Low — глобальный `slog` в репозитории

**Где:** `internal/repository/postgres/postgres.go:70`.

`slog.SetDefault(logger)` в `main.go` смягчил проблему (вывод теперь консистентный), но для тестируемости логгер лучше инжектить:

```go
type Repository struct {
    db     *pgxpool.Pool
    logger *slog.Logger
}

func NewPostgresRepository(db *pgxpool.Pool, logger *slog.Logger) *Repository {
    return &Repository{db: db, logger: logger}
}
// и в fallback: r.logger.Error(...) вместо slog.Error(...)
```

## 12. Nice-to-have — мелочи одним списком

- `internal/domain/error.go:9` — `"chanel overcrowded"` → `"queue is full"` (текст уходит клиенту в 503).
- `cmd/app/main.go:27` — `"starting pprof server on ^6060"` → нормальное сообщение (уйдёт само при фиксе пункта 1).
- `internal/domain/event.go:24,33` — сообщения `"event_type len string more 255"` → `"event_type exceeds 255 characters"`.
- `cmd/app/app.go:39` — `repopostgres` → `repo`.
- `cmd/app/main.go:15` — `_ = godotenv.Load()`: добавить `Debug`-лог, чтобы отличать «файла нет» от «файл битый».
- `internal/repository/postgres/postgres.go:28-36` — `pgx.CopyFromSlice(len(events), func(i int) ([]any, error) {...})` вместо предварительной сборки `[][]any` — меньше аллокаций в hot path.
- `internal/service/service.go` — задокументировать в `EventRepository`, что `InsertBatch` не должен сохранять слайс после возврата (батч переиспользуется через `batch[:0]`).
- `internal/domain/event.go` — опционально валидировать `Time` (не из будущего / не старше N дней).
- `Dockerfile` — при vendored-зависимостях `go mod download` не нужен (сборка возьмёт `vendor/`); в alpine добавить `ca-certificates`, если планируется `sslmode=require`.
- `docker-compose.yaml:67` — нет перевода строки в конце файла.
