package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

const minJWTSecretLen = 32

type Config struct {
	HTTPPort              string
	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	Postgres              Postgres
	JWTSecret             []byte
	JWTTTL                time.Duration
	OverdueCheckInterval  time.Duration
	ShutdownTimeout       time.Duration
	LogLevel              slog.Level
}

type Postgres struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:  getEnv("HTTP_PORT", "8080"),
		Postgres:  Postgres{DSN: os.Getenv("DATABASE_URL")},
		JWTSecret: []byte(os.Getenv("JWT_SECRET")),
	}

	var err error
	if cfg.HTTPReadHeaderTimeout, err = getDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second); err != nil {
		return nil, err
	}
	if cfg.HTTPReadTimeout, err = getDuration("HTTP_READ_TIMEOUT", 10*time.Second); err != nil {
		return nil, err
	}
	if cfg.HTTPWriteTimeout, err = getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second); err != nil {
		return nil, err
	}
	if cfg.HTTPIdleTimeout, err = getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return nil, err
	}
	if cfg.Postgres.MaxOpenConns, err = getInt("DB_MAX_OPEN_CONNS", 25); err != nil {
		return nil, err
	}
	if cfg.Postgres.MaxIdleConns, err = getInt("DB_MAX_IDLE_CONNS", 25); err != nil {
		return nil, err
	}
	if cfg.Postgres.ConnMaxIdleTime, err = getDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute); err != nil {
		return nil, err
	}
	if cfg.Postgres.PingTimeout, err = getDuration("DB_PING_TIMEOUT", 5*time.Second); err != nil {
		return nil, err
	}
	if cfg.JWTTTL, err = getDuration("JWT_TTL", time.Hour); err != nil {
		return nil, err
	}
	if cfg.OverdueCheckInterval, err = getDuration("OVERDUE_CHECK_INTERVAL", time.Minute); err != nil {
		return nil, err
	}
	if cfg.ShutdownTimeout, err = getDuration("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return nil, err
	}
	if err = cfg.LogLevel.UnmarshalText([]byte(getEnv("LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("LOG_LEVEL: %w", err)
	}

	if err = cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Postgres.DSN == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.Postgres.MaxIdleConns > c.Postgres.MaxOpenConns {
		return errors.New("DB_MAX_IDLE_CONNS must not exceed DB_MAX_OPEN_CONNS")
	}
	if len(c.JWTSecret) < minJWTSecretLen {
		return fmt.Errorf("JWT_SECRET must be at least %d bytes", minJWTSecretLen)
	}
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}

func getInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return n, nil
}
