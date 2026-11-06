package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qw145/hotelbooking/handlers"
	"github.com/qw145/hotelbooking/helper"
	"github.com/qw145/hotelbooking/middleware"
	"github.com/qw145/hotelbooking/service"
	"github.com/subosito/gotenv"
)

var (
	cfg *helper.Config
	log *slog.Logger
)

func getConfiguration() {
	var env = flag.String("env", "development", "environment: development, test or productuion")
	flag.Parse()

	gotenv.Load()

	cfg = helper.NewConfig()
	cfg.Env = *env

	log = helper.NewLogger(cfg)
	log.Info("starting hotel booking API",
		"env", cfg.Env,
		"driver", cfg.Driver,
		"addr", cfg.HTTPAddr,
	)

	helper.Cfg = cfg
	helper.Log = log
}

func main() {
	getConfiguration()

	if err := run(); err != nil {
		// The logger may not exist yet, so report to stderr directly.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := helper.OpenDb(ctx)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if helper.Db == nil {
		return fmt.Errorf("database not connected")
	}

	sqlDB, err := helper.Db.DB()
	if err != nil {
		return fmt.Errorf("resolve connection pool: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Error("closing database", "error", err)
		}
	}()

	///////////////////////////////////////

	pool := service.NewPool(cfg.EmailWorkerCount, cfg.EmailQueueSize)
	pool.Run(context.Background())

	svc, err := service.NewService()
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}
	api, err := handlers.NewAPI(svc, sqlDB.PingContext, pool)
	if err != nil {
		return fmt.Errorf("API: %w", err)
	}

	var mux *http.ServeMux = api.Handler()
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: middleware.Recoverer(log)(
			middleware.RequestID()(
				middleware.AccessLog(log)(
					middleware.SecurityHeaders(mux),
				),
			),
		),
		// Timeouts prevent a slow or idle client from holding a connection
		// indefinitely.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening",
			"addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	///////////////////////////////////////

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received, draining")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		if closeErr := srv.Close(); closeErr != nil {
			log.Error("forced close failed", "error", closeErr)
		}
	} else {
		log.Info("http server stopped")
	}

	go func() {
		close(pool.Collector)
		pool.Wg.Wait()

		log.Info("a")
		close(pool.Results)
	}()

	for res := range pool.Results {
		if res.Err != nil {
			fmt.Printf("Job failed: %v\n", res.Err)
		}
	}

	// Final check on how the lifecycle completed
	if ctx.Err() != nil {
		fmt.Printf("Execution lifecycle finished with context state: %v\n", ctx.Err())
	} else {
		fmt.Println("All jobs completed perfectly!")
	}

	return nil
}
