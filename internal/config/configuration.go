package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultCycleInterval = 60 * time.Second
	envCycleInterval     = "APPORDOWN_CYCLE_INTERVAL"
	defaultMailPort      = 587
	envMailEndpoint      = "APPORDOWN_MAIL_ENDPOINT"
	envMailPort          = "APPORDOWN_MAIL_PORT"
	envMailUsername      = "APPORDOWN_MAIL_USERNAME"
	envMailPassword      = "APPORDOWN_MAIL_PASSWORD"
	envMailSender        = "APPORDOWN_MAIL_SENDER"
	envMailNoTLS         = "APPORDOWN_MAIL_NO_TLS"
)

// Configuration holds runtime settings for the app.
type Configuration struct {
	CycleInterval time.Duration
	Mailserver    MailserverConfiguration
}

// MailserverConfiguration holds SMTP settings.
type MailserverConfiguration struct {
	Endpoint string
	Port     int
	Username string
	Password string
	Sender   string
	NoTLS    bool
}

// Load parses configuration from environment and CLI args.
// Precedence is: CLI argument, environment, default.
func Load(args []string) (Configuration, error) {
	cfg := Configuration{
		CycleInterval: defaultCycleInterval,
		Mailserver: MailserverConfiguration{
			Port: defaultMailPort,
		},
	}

	if raw := os.Getenv(envCycleInterval); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envCycleInterval, err)
		}
		cfg.CycleInterval = d
	}

	if raw := os.Getenv(envMailEndpoint); raw != "" {
		cfg.Mailserver.Endpoint = raw
	}
	if raw := os.Getenv(envMailPort); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envMailPort, err)
		}
		cfg.Mailserver.Port = port
	}
	if raw := os.Getenv(envMailUsername); raw != "" {
		cfg.Mailserver.Username = raw
	}
	if raw := os.Getenv(envMailPassword); raw != "" {
		cfg.Mailserver.Password = raw
	}
	if raw := os.Getenv(envMailSender); raw != "" {
		cfg.Mailserver.Sender = raw
	}
	if raw := os.Getenv(envMailNoTLS); raw != "" {
		noTLS, err := strconv.ParseBool(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envMailNoTLS, err)
		}
		cfg.Mailserver.NoTLS = noTLS
	}

	fs := flag.NewFlagSet("appordown", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.DurationVar(&cfg.CycleInterval, "cycle-interval", cfg.CycleInterval, "cycle interval (e.g. 60s, 1m)")
	fs.StringVar(&cfg.Mailserver.Endpoint, "mail-endpoint", cfg.Mailserver.Endpoint, "mail server endpoint")
	fs.IntVar(&cfg.Mailserver.Port, "mail-port", cfg.Mailserver.Port, "mail server port")
	fs.StringVar(&cfg.Mailserver.Username, "mail-username", cfg.Mailserver.Username, "mail server username")
	fs.StringVar(&cfg.Mailserver.Password, "mail-password", cfg.Mailserver.Password, "mail server password")
	fs.StringVar(&cfg.Mailserver.Sender, "mail-sender", cfg.Mailserver.Sender, "mail sender address")
	fs.BoolVar(&cfg.Mailserver.NoTLS, "mail-no-tls", cfg.Mailserver.NoTLS, "disable TLS for mail server")
	if err := fs.Parse(args); err != nil {
		return Configuration{}, err
	}

	return cfg, nil
}
