package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fabiant7t/eddie/internal/config"
	apphttp "github.com/fabiant7t/eddie/internal/http"
	"github.com/fabiant7t/eddie/internal/mail"
	"github.com/fabiant7t/eddie/internal/monitor"
	"github.com/fabiant7t/eddie/internal/spec"
	"github.com/fabiant7t/eddie/internal/state"
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

	mailService := initializeMailService(cfg)
	parsedSpecs, err := spec.Parse(cfg.SpecPath)
	if err != nil {
		slog.Error("failed to parse specs", "spec_path", cfg.SpecPath, "error", err)
		notifySpecParseFailure(cfg, mailService, err)
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

	stateStore := state.NewInMemoryStore()
	runner := monitor.NewRunner(parsedSpecs, cfg.CycleInterval, stateStore, mailService, cfg.Mailserver.Receivers)
	go runner.Run(ctx)

	// HTTP server
	httpOpts := []apphttp.Option{}
	httpOpts = append(httpOpts, apphttp.WithAppVersion(version))
	httpOpts = append(httpOpts, apphttp.WithStatusSnapshot(func() apphttp.StatusSnapshot {
		snapshot := apphttp.StatusSnapshot{
			GeneratedAt: time.Now().UTC(),
			Specs:       make([]apphttp.SpecStatus, 0, len(parsedSpecs)),
		}
		for _, parsedSpec := range parsedSpecs {
			specState, hasState := stateStore.Get(parsedSpec.HTTP.Name)
			snapshot.Specs = append(snapshot.Specs, apphttp.SpecStatus{
				Name:                 parsedSpec.HTTP.Name,
				SourcePath:           parsedSpec.SourcePath,
				Disabled:             !parsedSpec.IsActive(),
				HasState:             hasState,
				Status:               string(specState.Status),
				ConsecutiveFailures:  specState.ConsecutiveFailures,
				ConsecutiveSuccesses: specState.ConsecutiveSuccesses,
				LastCycleStartedAt:   specState.LastCycleStartedAt,
				LastCycleAt:          specState.LastCycleAt,
			})
		}
		return snapshot
	}))
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

func notifySpecParseFailure(cfg config.Configuration, mailService *mail.Service, parseErr error) {
	if mailService == nil {
		slog.Warn("cannot send parse failure email: mail is not configured")
		return
	}
	if len(cfg.Mailserver.Receivers) == 0 {
		slog.Warn("cannot send parse failure email: no mail receivers configured")
		return
	}

	subjectLine := "Subject: eddie spec parse failure"
	body := fmt.Sprintf(
		"%s\r\n\r\nfailed to parse specs from %q\r\nerror: %v\r\n",
		subjectLine,
		cfg.SpecPath,
		parseErr,
	)

	sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, recipient := range cfg.Mailserver.Receivers {
		if err := mailService.Send(sendCtx, recipient, []byte(body)); err != nil {
			slog.Error("failed to send parse failure email", "recipient", recipient, "error", err)
		}
	}
}

func initializeMailService(cfg config.Configuration) *mail.Service {
	if cfg.Mailserver.Endpoint == "" || cfg.Mailserver.Username == "" || cfg.Mailserver.Password == "" || cfg.Mailserver.Sender == "" {
		slog.Info("mail notifications disabled: mailserver configuration is incomplete")
		return nil
	}

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
		slog.Error("mail notifications disabled: failed to initialize mail service", "error", err)
		return nil
	}

	return mailService
}
