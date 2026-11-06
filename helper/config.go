package helper

import (
	"os"
	"strconv"
	"time"
)

type Driver string

const (
	DriverSQLite Driver = "sqlite"
)

type Config struct {
	// Env is the deployment environment: "development", "test" or "production".
	// Test-only endpoints (seed/reset) are disabled when Env is "production".
	Env string

	// HTTPAddr is the listen address for the API server, e.g. ":8080".
	HTTPAddr string

	// Driver selects the database dialect.
	Driver Driver

	// DSN is the driver-specific connection string.
	//
	// SQLite: a file path or "file:booking.db?cache=shared", or
	//         "file::memory:?cache=shared" for an in-memory database.
	DSN string

	// AutoMigrate runs GORM's schema migration on startup. Convenient for
	// SQLite development; for Azure SQL, prefer the versioned SQL files in
	// migrations/ so schema changes are reviewable.
	AutoMigrate bool

	// ShutdownTimeout bounds how long graceful shutdown waits for in-flight
	// requests and the email worker to drain.
	ShutdownTimeout time.Duration

	// EmailWorkerCount is the size of the async confirmation-email pool.
	EmailWorkerCount int

	// EmailQueueSize is the buffer depth of the confirmation-email queue.
	EmailQueueSize int

	// EmailSendDelay simulates the latency of an external email provider.
	EmailSendDelay time.Duration

	// DBConnectTimeout bounds the initial database connection attempt, so a
	// misconfigured host fails fast instead of hanging.
	DBConnectTimeout time.Duration
}

var Cfg *Config

func NewConfig() *Config {
	// Load reads configuration from the environment, applying defaults.
	return &Config{
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		Driver:           Driver(getEnv("DB_DRIVER", string(DriverSQLite))),
		DSN:              getEnv("DB_DSN", "booking.db"),
		AutoMigrate:      getBool("DB_AUTO_MIGRATE", true),
		ShutdownTimeout:  getDuration("SHUTDOWN_TIMEOUT", 15*time.Second), // in seconds
		EmailWorkerCount: getInt("EMAIL_WORKER_COUNT", 4),
		EmailQueueSize:   getInt("EMAIL_QUEUE_SIZE", 256),
		EmailSendDelay:   getDuration("EMAIL_SEND_DELAY", 2*time.Second),    // in seconds
		DBConnectTimeout: getDuration("DB_CONNECT_TIMEOUT", 10*time.Second), // in seconds
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}
