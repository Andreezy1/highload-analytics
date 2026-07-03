package handler

import (
	"log/slog"
	"net"
	"net/http"
	"time"
)

func RateLimiterMiddleware(limiter RateLimiter,
	logger *slog.Logger,
	limit int,
	window time.Duration,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientKey, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				clientKey = r.RemoteAddr
			}
			allowed, err := limiter.Allow(r.Context(), clientKey, limit, window)
			if err != nil {
				logger.Error("rate limiter failure, falling back to fail-open",
					slog.String("client_ip", clientKey),
					slog.Any("error", err),
				)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				logger.Warn("rate limit exceeded",
					slog.String("client_ip", clientKey),
				)
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
