package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alex/zus_home_assessment/internal/config"
	"github.com/alex/zus_home_assessment/internal/events"
	"github.com/alex/zus_home_assessment/internal/httpapi"
	"github.com/alex/zus_home_assessment/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to create db pool", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		logger.Error("failed to ping db", "error", err)
		os.Exit(1)
	}

	publisher := events.NewKafkaOrderPublisher(cfg.KafkaBrokers, cfg.KafkaOrderTopic)
	defer publisher.Close()

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewServer(store.New(db), publisher, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpapi.Shutdown(shutdownCtx, server); err != nil {
		logger.Error("api shutdown failed", "error", err)
		os.Exit(1)
	}
}
