package main

import (
	"highload-analytics/config"
	"log/slog"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	logger := newLogger()
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", slog.Any("error", err))
		os.Exit(1)
	}
	app, err := newApp(cfg, logger)
	if err != nil {
		logger.Error("create application", slog.Any("error", err))
		os.Exit(1)
	}
	go func() {
		logger.Info("starting pprof server on ^6060")
		if err := http.ListenAndServe(":6060", nil); err != nil {
			logger.Info("pprof server failed^ %v", err)
		}
	}()
	run(app, logger)

}
