package main

import (
	"highload-analytics/config"
	"log/slog"
	"net/http"
	"net/http/pprof"
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
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

		logger.Info("starting pprof server", slog.String("addr", "localhost:6060"))
		if err := http.ListenAndServe("localhost:6060", mux); err != nil {
			logger.Error("pprof server failed", slog.Any("error", err))
		}
	}()
	run(app, logger)

}
