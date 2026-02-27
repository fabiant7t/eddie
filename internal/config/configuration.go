package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	defaultCycleInterval = 60 * time.Second
	envCycleInterval     = "APPORDOWN_CYCLE_INTERVAL"
	defaultConfigDir     = "config.d"
	defaultConfigPattern = "*.yaml"
	envConfigPath        = "APPORDOWN_CONFIG_PATH"
	defaultHTTPPort      = 8080
	defaultHTTPAddress   = "0.0.0.0"
	envHTTPAddress       = "APPORDOWN_HTTP_ADDRESS"
	envHTTPPort          = "APPORDOWN_HTTP_PORT"
	envHTTPBasicUser     = "APPORDOWN_HTTP_BASIC_AUTH_USERNAME"
	envHTTPBasicPassword = "APPORDOWN_HTTP_BASIC_AUTH_PASSWORD"
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
	ConfigurationPath string
	CycleInterval     time.Duration
	HTTPServer        HTTPServerConfiguration
	Mailserver        MailserverConfiguration
}

// HTTPServerConfiguration holds HTTP server settings.
type HTTPServerConfiguration struct {
	Address           string
	Port              int
	BasicAuthUsername string
	BasicAuthPassword string
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
	defaultConfigurationPath, err := resolveDefaultConfigurationPath()
	if err != nil {
		return Configuration{}, err
	}

	cfg := Configuration{
		ConfigurationPath: defaultConfigurationPath,
		CycleInterval: defaultCycleInterval,
		HTTPServer: HTTPServerConfiguration{
			Address: defaultHTTPAddress,
			Port:    defaultHTTPPort,
		},
		Mailserver: MailserverConfiguration{
			Port: defaultMailPort,
		},
	}

	if raw := os.Getenv(envConfigPath); raw != "" {
		cfg.ConfigurationPath = raw
	}
	if raw := os.Getenv(envCycleInterval); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envCycleInterval, err)
		}
		cfg.CycleInterval = d
	}
	if raw := os.Getenv(envHTTPAddress); raw != "" {
		cfg.HTTPServer.Address = raw
	}
	if raw := os.Getenv(envHTTPPort); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envHTTPPort, err)
		}
		cfg.HTTPServer.Port = port
	}
	if raw := os.Getenv(envHTTPBasicUser); raw != "" {
		cfg.HTTPServer.BasicAuthUsername = raw
	}
	if raw := os.Getenv(envHTTPBasicPassword); raw != "" {
		cfg.HTTPServer.BasicAuthPassword = raw
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
	fs.StringVar(&cfg.ConfigurationPath, "config-path", cfg.ConfigurationPath, "path or glob for configuration files")
	fs.DurationVar(&cfg.CycleInterval, "cycle-interval", cfg.CycleInterval, "cycle interval (e.g. 60s, 1m)")
	fs.StringVar(&cfg.HTTPServer.Address, "http-address", cfg.HTTPServer.Address, "http server listen address")
	fs.IntVar(&cfg.HTTPServer.Port, "http-port", cfg.HTTPServer.Port, "http server listen port")
	fs.StringVar(&cfg.HTTPServer.BasicAuthUsername, "http-basic-auth-username", cfg.HTTPServer.BasicAuthUsername, "http basic auth username")
	fs.StringVar(&cfg.HTTPServer.BasicAuthPassword, "http-basic-auth-password", cfg.HTTPServer.BasicAuthPassword, "http basic auth password")
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

func resolveDefaultConfigurationPath() (string, error) {
	baseConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(baseConfigDir, "appordown", defaultConfigDir, defaultConfigPattern), nil
}
