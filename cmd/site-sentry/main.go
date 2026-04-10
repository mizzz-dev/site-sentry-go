package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"site-sentry-go/internal/config"
	"site-sentry-go/internal/db"
	"site-sentry-go/internal/handler"
	"site-sentry-go/internal/repository"
	"site-sentry-go/internal/scheduler"
	"site-sentry-go/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	if err := db.Migrate(cfg.DBPath); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	repo := repository.NewSQLiteMonitorRepository(cfg.DBPath)
	svc := service.NewMonitorService(repo)
	h, err := handler.NewHTTPHandler(svc, cfg)
	if err != nil {
		log.Fatalf("handler init failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	runner := scheduler.NewRunner(svc, cfg.SchedulerTick)
	go runner.Start(ctx)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           h.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown failed: %v", err)
		}
	}()

	log.Printf("site-sentry-go started on :%d", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server failed: %v", err)
		os.Exit(1)
	}
}
