package main

import (
	"log/slog"
	"os"

	"github.com/fabiant7t/appordown/internal/config"
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
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	return "***"
}
