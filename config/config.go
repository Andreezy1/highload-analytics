package config

import (
	"time"

	"github.com/caarlos0/env"
)

type Config struct {
	HTTPPort string `env:"HTTP_PORT" envDefault:"8080"`

	DBHost     string `env:"DB_HOST,required"`
	DBPort     string `env:"DB_PORT,required"`
	DBUser     string `env:"DB_USER,required"`
	DBPassword string `env:"DB_PASSWORD,required"`
	DBName     string `env:"DB_NAME,required"`
	DBSSLMode  string `env:"DB_SSLMODE" envDefault:"disable"`

	RedisAddress string `env:"REDIS_ADDRESS,required"`

	WorkerCount     int           `env:"WORKER_COUNT" envDefault:"5"`
	ChanSize        int           `env:"CHAN_SIZE" envDefault:"10000"`
	BatchSize       int           `env:"BATCH_SIZE" envDefault:"500"`
	FlushInterval   time.Duration `env:"FLUSH_INTERVAL" envDefault:"1s"`
	ClientLimit     int           `env:"CLIENT_LIMIT" envDefault:"1000000"`
	RateLimitWindow time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"60s"`
}

func Load() (*Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
