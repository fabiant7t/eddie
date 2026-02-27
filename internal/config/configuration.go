package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCycleInterval = 60 * time.Second
	envCycleInterval     = "EDDIE_CYCLE_INTERVAL"
	defaultLogLevel      = "INFO"
	envLogLevel          = "EDDIE_LOG_LEVEL"
	envLogLevelAlt       = "EDDIE_LOGLEVEL"
	defaultConfigDir     = "config.d"
	envSpecPath          = "EDDIE_SPEC_PATH"
	defaultHTTPPort      = 8080
	defaultHTTPAddress   = "0.0.0.0"
	envHTTPAddress       = "EDDIE_HTTP_ADDRESS"
	envHTTPPort          = "EDDIE_HTTP_PORT"
	envHTTPBasicUser     = "EDDIE_HTTP_BASIC_AUTH_USERNAME"
	envHTTPBasicPassword = "EDDIE_HTTP_BASIC_AUTH_PASSWORD"
	defaultMailPort      = 587
	envMailEndpoint      = "EDDIE_MAIL_ENDPOINT"
	envMailPort          = "EDDIE_MAIL_PORT"
	envMailUsername      = "EDDIE_MAIL_USERNAME"
	envMailPassword      = "EDDIE_MAIL_PASSWORD"
	envMailSender        = "EDDIE_MAIL_SENDER"
	envMailReceivers     = "EDDIE_MAIL_RECEIVERS"
	envMailNoTLS         = "EDDIE_MAIL_NO_TLS"
)

// Configuration holds runtime settings for the app.
type Configuration struct {
	SpecPath      string
	CycleInterval time.Duration
	LogLevel      string
	HTTPServer    HTTPServerConfiguration
	Mailserver    MailserverConfiguration
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
	Endpoint  string
	Port      int
	Username  string
	Password  string
	Sender    string
	Receivers []string
	NoTLS     bool
}

// Load parses configuration from environment and CLI args.
// Precedence is: CLI argument, environment, default.
func Load(args []string) (Configuration, error) {
	defaultSpecPath, err := resolveDefaultSpecPath()
	if err != nil {
		return Configuration{}, err
	}

	cfg := Configuration{
		SpecPath:      defaultSpecPath,
		CycleInterval: defaultCycleInterval,
		LogLevel:      defaultLogLevel,
		HTTPServer: HTTPServerConfiguration{
			Address: defaultHTTPAddress,
			Port:    defaultHTTPPort,
		},
		Mailserver: MailserverConfiguration{
			Port: defaultMailPort,
		},
	}

	if raw := os.Getenv(envSpecPath); raw != "" {
		cfg.SpecPath = raw
	}
	if raw := os.Getenv(envCycleInterval); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envCycleInterval, err)
		}
		cfg.CycleInterval = d
	}
	if raw := os.Getenv(envLogLevel); raw != "" {
		cfg.LogLevel = raw
	} else if raw := os.Getenv(envLogLevelAlt); raw != "" {
		cfg.LogLevel = raw
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
	if raw := os.Getenv(envMailReceivers); raw != "" {
		cfg.Mailserver.Receivers = parseCSVList(raw)
	}
	if raw := os.Getenv(envMailNoTLS); raw != "" {
		noTLS, err := strconv.ParseBool(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envMailNoTLS, err)
		}
		cfg.Mailserver.NoTLS = noTLS
	}

	fs := flag.NewFlagSet("eddie", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.SpecPath, "spec-path", cfg.SpecPath, "spec path value")
	fs.DurationVar(&cfg.CycleInterval, "cycle-interval", cfg.CycleInterval, "cycle interval (e.g. 60s, 1m)")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level (DEBUG, INFO, WARN, ERROR)")
	fs.StringVar(&cfg.HTTPServer.Address, "http-address", cfg.HTTPServer.Address, "http server listen address")
	fs.IntVar(&cfg.HTTPServer.Port, "http-port", cfg.HTTPServer.Port, "http server listen port")
	fs.StringVar(&cfg.HTTPServer.BasicAuthUsername, "http-basic-auth-username", cfg.HTTPServer.BasicAuthUsername, "http basic auth username")
	fs.StringVar(&cfg.HTTPServer.BasicAuthPassword, "http-basic-auth-password", cfg.HTTPServer.BasicAuthPassword, "http basic auth password")
	fs.StringVar(&cfg.Mailserver.Endpoint, "mail-endpoint", cfg.Mailserver.Endpoint, "mail server endpoint")
	fs.IntVar(&cfg.Mailserver.Port, "mail-port", cfg.Mailserver.Port, "mail server port")
	fs.StringVar(&cfg.Mailserver.Username, "mail-username", cfg.Mailserver.Username, "mail server username")
	fs.StringVar(&cfg.Mailserver.Password, "mail-password", cfg.Mailserver.Password, "mail server password")
	fs.StringVar(&cfg.Mailserver.Sender, "mail-sender", cfg.Mailserver.Sender, "mail sender address")
	fs.Var(newStringSliceFlag(&cfg.Mailserver.Receivers), "mail-receiver", "mail receiver address (repeatable)")
	fs.BoolVar(&cfg.Mailserver.NoTLS, "mail-no-tls", cfg.Mailserver.NoTLS, "disable TLS for mail server")
	if err := fs.Parse(args); err != nil {
		return Configuration{}, err
	}
	logLevel, err := normalizeLogLevel(cfg.LogLevel)
	if err != nil {
		return Configuration{}, fmt.Errorf("invalid %s: %w", envLogLevel, err)
	}
	cfg.LogLevel = logLevel

	return cfg, nil
}

func normalizeLogLevel(raw string) (string, error) {
	level := strings.ToUpper(strings.TrimSpace(raw))
	switch level {
	case "DEBUG", "INFO", "WARN", "ERROR":
		return level, nil
	default:
		return "", fmt.Errorf("unsupported log level %q", raw)
	}
}

func ParseSlogLevel(logLevel string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(logLevel)) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", logLevel)
	}
}

func resolveDefaultSpecPath() (string, error) {
	baseConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(baseConfigDir, "eddie", defaultConfigDir), nil
}
func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

type stringSliceFlag struct {
	target  *[]string
	changed bool
}

func newStringSliceFlag(target *[]string) *stringSliceFlag {
	return &stringSliceFlag{target: target}
}

func (f *stringSliceFlag) String() string {
	return strings.Join(*f.target, ",")
}

func (f *stringSliceFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("mail receiver cannot be empty")
	}

	if !f.changed {
		*f.target = nil
		f.changed = true
	}
	*f.target = append(*f.target, trimmed)
	return nil
}
