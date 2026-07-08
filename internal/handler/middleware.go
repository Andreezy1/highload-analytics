package handler

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
}

func RateLimiterMiddleware(limiter RateLimiter,
	logger *slog.Logger,
	limit int,
	window time.Duration,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientKey := getClientIP(r)
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
