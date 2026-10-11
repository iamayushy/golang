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

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/iamayushy/golang/internal/config"
	"github.com/iamayushy/golang/internal/mongostore"
	"github.com/iamayushy/golang/internal/server"
	"github.com/iamayushy/golang/internal/shortener"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return err
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(disconnectCtx)
	}()

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(initCtx, nil); err != nil {
		return fmt.Errorf("connecting to MongoDB: %w", err)
	}
	repo, err := mongostore.NewLinkRepository(initCtx, client.Database(cfg.MongoDB))
	if err != nil {
		return err
	}

	ping := func(ctx context.Context) error { return client.Ping(ctx, nil) }
	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: server.New(logger, shortener.NewService(repo), server.Options{
			BaseURL:          cfg.BaseURL,
			FrontendURL:      cfg.FrontendURL,
			AllowedOrigins:   cfg.AllowedOrigins,
			CreatesPerMinute: cfg.RateLimit,
			Ping:             ping,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.Addr, "db", cfg.MongoDB)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
