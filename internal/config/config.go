package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            int
	DBPath          string
	SchedulerTick   time.Duration
	ResultLimit     int
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	port, err := intFromEnv("APP_PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	tick, err := intFromEnv("SCHEDULER_TICK_SECONDS", 1)
	if err != nil {
		return Config{}, err
	}
	resultLimit, err := intFromEnv("DEFAULT_RESULT_LIMIT", 20)
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := intFromEnv("REQUEST_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := intFromEnv("SHUTDOWN_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./site_sentry.db"
	}

	return Config{
		Port:            port,
		DBPath:          dbPath,
		SchedulerTick:   time.Duration(tick) * time.Second,
		ResultLimit:     resultLimit,
		RequestTimeout:  time.Duration(requestTimeout) * time.Second,
		ShutdownTimeout: time.Duration(shutdownTimeout) * time.Second,
	}, nil
}

func intFromEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	iv, err := strconv.Atoi(v)
	if err != nil || iv <= 0 {
		return 0, fmt.Errorf("%s must be positive integer", key)
	}
	return iv, nil
}
