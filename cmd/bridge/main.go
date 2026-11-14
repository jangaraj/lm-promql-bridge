package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lm-promql-bridge/internal/config"
	"lm-promql-bridge/internal/httpauth"
	"lm-promql-bridge/internal/lm"
	"lm-promql-bridge/internal/prom"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting LM PromQL Bridge...")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "err", err)
		os.Exit(1)
	}

	lmClient := lm.NewClient(cfg)
	promHandler := prom.NewHandler(lmClient, cfg.Debug)

	mux := http.NewServeMux()
	promHandler.RegisterRoutes(mux)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	handler := http.Handler(mux)
	if cfg.BasicAuthEnabled() {
		handler = httpauth.BasicAuth(cfg.ServerUsername, cfg.ServerPassword, mux)
	}

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.RequestTimeout + 10*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("Server listening",
			"port", cfg.ServerPort,
			"portal", cfg.Portal,
			"debug", cfg.Debug,
			"basic_auth", cfg.BasicAuthEnabled(),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced shutdown", "err", err)
	}

	slog.Info("Server stopped gracefully")
}
