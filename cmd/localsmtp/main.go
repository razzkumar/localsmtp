package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.io/razzkumar/localsmtp/internal/api"
	"github.io/razzkumar/localsmtp/internal/auth"
	"github.io/razzkumar/localsmtp/internal/config"
	"github.io/razzkumar/localsmtp/internal/smtpserver"
	"github.io/razzkumar/localsmtp/internal/sse"
	"github.io/razzkumar/localsmtp/internal/store"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.EnsureSchema(ctx); err != nil {
		logger.Error("ensure schema", "error", err)
		os.Exit(1)
	}

	authManager, err := auth.New(cfg.AuthSecret, 30*24*time.Hour)
	if err != nil {
		logger.Error("init auth", "error", err)
		os.Exit(1)
	}
	if cfg.AuthSecret == "" {
		logger.Warn("AUTH_SECRET not set; sessions reset on restart")
	}

	hub := sse.NewHub()
	apiServer := api.NewServer(cfg, db, authManager, hub, logger)

	smtpAuthCfg := smtpserver.AuthConfig{
		Enabled:  cfg.SMTPAuthEnabled,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
	}
	if smtpAuthCfg.Enabled {
		logger.Info("smtp auth enabled", "username", smtpAuthCfg.Username, "password", smtpAuthCfg.Password)
	} else {
		logger.Warn("smtp auth disabled; server accepts unauthenticated connections")
	}

	smtpAddr := fmt.Sprintf(":%d", cfg.SMTPPort)
	smtpSrv := smtpserver.New(db, hub, logger, smtpAddr, smtpAuthCfg)

	httpAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	httpSrv := &http.Server{
		Addr:    httpAddr,
		Handler: apiServer,
	}

	go func() {
		if err := smtpSrv.ListenAndServe(); err != nil {
			logger.Error("smtp server stopped", "error", err)
		}
	}()

	go func() {
		logger.Info("http server listening", "addr", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		logger.Error("shutdown http", "error", err)
	}
	if err := smtpSrv.Close(); err != nil {
		logger.Error("shutdown smtp", "error", err)
	}
}
