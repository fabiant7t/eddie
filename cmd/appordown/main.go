package main

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fabiant7t/appordown/internal/config"
	apphttp "github.com/fabiant7t/appordown/internal/http"
	"github.com/fabiant7t/appordown/internal/mail"
	"github.com/fabiant7t/appordown/internal/spec"
)

var (
	version  = "dev"
	date     string
	revision string
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logLevel, err := config.ParseSlogLevel(cfg.LogLevel)
	if err != nil {
		slog.Error("failed to parse log level", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// App information
	slog.Info("build",
		"version", version,
		"date", date,
		"revision", revision,
	)

	// Configuration information
	slog.Info("config",
		"spec_path", cfg.SpecPath,
		"cycle_interval", cfg.CycleInterval.String(),
		"log_level", cfg.LogLevel,
	)
	slog.Info("config.http",
		"address", cfg.HTTPServer.Address,
		"port", cfg.HTTPServer.Port,
		"basic_auth_username", cfg.HTTPServer.BasicAuthUsername,
		"basic_auth_password", redact(cfg.HTTPServer.BasicAuthPassword),
	)
	slog.Info("config.mail",
		"endpoint", cfg.Mailserver.Endpoint,
		"port", cfg.Mailserver.Port,
		"username", cfg.Mailserver.Username,
		"password", redact(cfg.Mailserver.Password),
		"sender", cfg.Mailserver.Sender,
		"receivers", cfg.Mailserver.Receivers,
		"no_tls", cfg.Mailserver.NoTLS,
	)

	parsedSpecs, err := spec.Parse(cfg.SpecPath)
	if err != nil {
		slog.Error("failed to parse specs", "spec_path", cfg.SpecPath, "error", err)
		os.Exit(1)
	}
	for _, parsedSpec := range parsedSpecs {
		if parsedSpec.IsActive() {
			slog.Debug("spec_parsed",
				"name", parsedSpec.HTTP.Name,
				"source", parsedSpec.SourcePath,
			)
		}
	}

	//Mail service
	opts := []mail.Option{
		mail.WithPort(cfg.Mailserver.Port),
	}
	for _, receiver := range cfg.Mailserver.Receivers {
		opts = append(opts, mail.WithReceiver(receiver))
	}
	if cfg.Mailserver.NoTLS {
		opts = append(opts, mail.WithNoTLS())
	}
	mailService, err := mail.New(
		cfg.Mailserver.Endpoint,
		cfg.Mailserver.Username,
		cfg.Mailserver.Password,
		cfg.Mailserver.Sender,
		opts...,
	)
	if err != nil {
		slog.Error("failed to initialize mail service", "error", err)
		os.Exit(1)
	}
	_ = mailService

	// HTTP server
	httpOpts := []apphttp.Option{}
	httpOpts = append(httpOpts, apphttp.WithAppVersion(version))
	if cfg.HTTPServer.BasicAuthUsername != "" || cfg.HTTPServer.BasicAuthPassword != "" {
		httpOpts = append(httpOpts, apphttp.WithBasicAuth(
			cfg.HTTPServer.BasicAuthUsername,
			cfg.HTTPServer.BasicAuthPassword,
		))
	}
	httpServer, err := apphttp.New(cfg.HTTPServer.Address, cfg.HTTPServer.Port, httpOpts...)
	if err != nil {
		slog.Error("failed to initialize http server", "error", err)
		os.Exit(1)
	}
	slog.Info("service running", "message", "press Ctrl+C to stop")

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received", "error", ctx.Err())

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown http server", "error", err)
			os.Exit(1)
		}
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			slog.Error("http server exited with error", "error", err)
			os.Exit(1)
		}
	}
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	return "***"
}
