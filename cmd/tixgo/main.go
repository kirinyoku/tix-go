package main

import (
	"context"
	"log/slog"
	"os"

	_ "github.com/kirinyoku/tix-go/docs"
	"github.com/kirinyoku/tix-go/internal/app"
	"github.com/kirinyoku/tix-go/internal/config"
)

// @title TixGo API
// @version 1.0
// @description This is a sample server for a ticketing service.
// @host localhost:8080
// @BasePath /
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.New()
	if err != nil {
		logger.Error("failed to load config", "error", err)
	}

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create application", "error", err)
	}

	if err := application.Run(context.Background()); err != nil {
		logger.Error("application finished with error", "error", err)
	}
}
