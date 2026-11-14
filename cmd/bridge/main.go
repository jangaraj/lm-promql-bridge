package main

import (
	"context"
	"crypto/tls"
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
	"lm-promql-bridge/internal/servertls"
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

	var tlsConfig *tls.Config
	if cfg.ServerTLSEnabled {
		tlsConfig, err = servertls.NewConfig(cfg.ServerTLSCert, cfg.ServerTLSKey)
		if err != nil {
			slog.Error("Failed to configure TLS", "err", err)
			os.Exit(1)
		}
	}

	lmClient := lm.NewClient(cfg)
	promHandler := prom.NewHandler(lmClient, cfg.Debug)

	mux := http.NewServeMux()
	promHandler.RegisterRoutes(mux)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	handler := prom.LimitRequestBody(http.Handler(mux))
	if cfg.BasicAuthEnabled() {
		handler = httpauth.BasicAuth(cfg.ServerUsername, cfg.ServerPassword, handler)
	}

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      handler,
		TLSConfig:    tlsConfig,
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
			"tls", cfg.ServerTLSEnabled,
		)
		var err error
		if cfg.ServerTLSEnabled {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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
