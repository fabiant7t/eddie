package main

import (
	"log/slog"
	"os"

	"github.com/fabiant7t/appordown/internal/config"
	"github.com/fabiant7t/appordown/internal/mail"
)

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("config",
		"configuration_path", cfg.ConfigurationPath,
		"cycle_interval", cfg.CycleInterval.String(),
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

}

func redact(value string) string {
	if value == "" {
		return ""
	}
	return "***"
}
