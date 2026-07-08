package main

import (
	"context"
	"errors"
	"highload-analytics/config"
	"highload-analytics/internal/handler"
	"highload-analytics/internal/repository/postgres"
	"highload-analytics/internal/repository/redislimiter"
	"highload-analytics/internal/service"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type App struct {
	Server  *http.Server
	DB      *pgxpool.Pool
	Redis   *redis.Client
	Service *service.Service
}

func newApp(cfg *config.Config, logger *slog.Logger) (*App, error) {
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbCancel()

	db, err := postgres.NewPostgresConnect(dbCtx, cfg)
	if err != nil {
		return nil, err
	}
	repopostgres := postgres.NewPostgresRepository(db)

	service := service.NewService(repopostgres, cfg.ChanSize, cfg.BatchSize, cfg.FlushInterval, logger)
	service.Start(context.Background())

	redisCtx, redisCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer redisCancel()
	rdb, err := redislimiter.NewRedisConnect(redisCtx, cfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	limiter := redislimiter.NewRedisLimiter(rdb)

	h := handler.NewHandler(service, logger)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(handler.RateLimiterMiddleware(limiter, logger, cfg.ClientLimit, cfg.WindowMs))
	h.RegisterRoutes(router)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return &App{
		Server:  server,
		DB:      db,
		Redis:   rdb,
		Service: service,
	}, nil
}

func run(app *App, logger *slog.Logger) {
	go func() {
		logger.Info("server started",
			slog.String("addr", app.Server.Addr),
		)
		if err := app.Server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server",
				slog.Any("error", err),
			)
		}
	}()
	waitShutdown(app, logger)
}

func waitShutdown(app *App, logger *slog.Logger) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		logger.Error(
			"shutdown server",
			slog.Any("error", err),
		)
	}
	app.Service.Stop()
	logger.Info("all background workers stopped cleanly")
	if err := app.Redis.Close(); err != nil {
		logger.Error("close redis error",
			slog.Any("error", err),
		)
	}
	logger.Info("redis connection closed")
	app.DB.Close()
	logger.Info("database connect closed")
	logger.Info("server stopped")
}
