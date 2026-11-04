// Package config loads service configuration from the environment.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env       string `env:"APP_ENV" envDefault:"development"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	JWTSecret string `env:"JWT_SECRET,required,notEmpty"`

	HTTP  HTTPConfig  `envPrefix:"HTTP_"`
	DB    DBConfig    `envPrefix:"DB_"`
	Redis RedisConfig `envPrefix:"REDIS_"`
}

type HTTPConfig struct {
	Addr            string        `env:"ADDR" envDefault:":8080"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"5s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"10s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
}

type DBConfig struct {
	URL      string `env:"URL,required,notEmpty"`
	MaxConns int32  `env:"MAX_CONNS" envDefault:"10"`
}

type RedisConfig struct {
	Addr string `env:"ADDR" envDefault:"localhost:6379"`
	TTL  time.Duration `env:"TTL" envDefault:"5m"`
}

// Load parses the process environment into a Config.
func Load() (*Config, error) {
	var cfg Config
	opts := env.Options{RequiredIfNoDef: false}
	if err := env.ParseWithOptions(&cfg, opts); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
