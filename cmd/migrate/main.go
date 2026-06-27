package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alex/zus_home_assessment/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx := context.Background()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to create db pool", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	sql, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		logger.Error("failed to read migration", "error", err)
		os.Exit(1)
	}

	if _, err := db.Exec(ctx, string(sql)); err != nil {
		logger.Error("failed to run migration", "error", err)
		os.Exit(1)
	}

	logger.Info("migration complete")
}
