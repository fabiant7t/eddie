package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv(envCycleInterval, "")
	t.Setenv(envHTTPAddress, "")
	t.Setenv(envHTTPPort, "")
	t.Setenv(envHTTPBasicUser, "")
	t.Setenv(envHTTPBasicPassword, "")
	t.Setenv(envMailEndpoint, "")
	t.Setenv(envMailPort, "")
	t.Setenv(envMailUsername, "")
	t.Setenv(envMailPassword, "")
	t.Setenv(envMailSender, "")
	t.Setenv(envMailNoTLS, "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
	}
	if cfg.HTTPServer.Address != defaultHTTPAddress {
		t.Fatalf("HTTPServer.Address = %q, want %q", cfg.HTTPServer.Address, defaultHTTPAddress)
	}
	if cfg.HTTPServer.Port != defaultHTTPPort {
		t.Fatalf("HTTPServer.Port = %v, want %v", cfg.HTTPServer.Port, defaultHTTPPort)
	}
	if cfg.Mailserver.Port != defaultMailPort {
		t.Fatalf("Mailserver.Port = %v, want %v", cfg.Mailserver.Port, defaultMailPort)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv(envCycleInterval, "1m")
	t.Setenv(envHTTPAddress, "127.0.0.1")
	t.Setenv(envHTTPPort, "9090")
	t.Setenv(envHTTPBasicUser, "admin")
	t.Setenv(envHTTPBasicPassword, "admin-secret")
	t.Setenv(envMailEndpoint, "smtp.example.com")
	t.Setenv(envMailPort, "2525")
	t.Setenv(envMailUsername, "alice")
	t.Setenv(envMailPassword, "secret")
	t.Setenv(envMailSender, "noreply@example.com")
	t.Setenv(envMailNoTLS, "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
	}
	if cfg.HTTPServer.Address != "127.0.0.1" {
		t.Fatalf("HTTPServer.Address = %q, want %q", cfg.HTTPServer.Address, "127.0.0.1")
	}
	if cfg.HTTPServer.Port != 9090 {
		t.Fatalf("HTTPServer.Port = %v, want %v", cfg.HTTPServer.Port, 9090)
	}
	if cfg.HTTPServer.BasicAuthUsername != "admin" {
		t.Fatalf("HTTPServer.BasicAuthUsername = %q, want %q", cfg.HTTPServer.BasicAuthUsername, "admin")
	}
	if cfg.HTTPServer.BasicAuthPassword != "admin-secret" {
		t.Fatalf("HTTPServer.BasicAuthPassword = %q, want %q", cfg.HTTPServer.BasicAuthPassword, "admin-secret")
	}
	if cfg.Mailserver.Endpoint != "smtp.example.com" {
		t.Fatalf("Mailserver.Endpoint = %q, want %q", cfg.Mailserver.Endpoint, "smtp.example.com")
	}
	if cfg.Mailserver.Port != 2525 {
		t.Fatalf("Mailserver.Port = %v, want %v", cfg.Mailserver.Port, 2525)
	}
	if cfg.Mailserver.Username != "alice" {
		t.Fatalf("Mailserver.Username = %q, want %q", cfg.Mailserver.Username, "alice")
	}
	if cfg.Mailserver.Password != "secret" {
		t.Fatalf("Mailserver.Password = %q, want %q", cfg.Mailserver.Password, "secret")
	}
	if cfg.Mailserver.Sender != "noreply@example.com" {
		t.Fatalf("Mailserver.Sender = %q, want %q", cfg.Mailserver.Sender, "noreply@example.com")
	}
	if cfg.Mailserver.NoTLS != true {
		t.Fatalf("Mailserver.NoTLS = %v, want %v", cfg.Mailserver.NoTLS, true)
	}
}

func TestLoadCLIOverridesEnv(t *testing.T) {
	t.Setenv(envCycleInterval, "1m")
	t.Setenv(envHTTPAddress, "127.0.0.1")
	t.Setenv(envHTTPPort, "9090")
	t.Setenv(envHTTPBasicUser, "admin")
	t.Setenv(envHTTPBasicPassword, "admin-secret")
	t.Setenv(envMailEndpoint, "smtp.example.com")
	t.Setenv(envMailPort, "2525")
	t.Setenv(envMailUsername, "alice")
	t.Setenv(envMailPassword, "secret")
	t.Setenv(envMailSender, "noreply@example.com")
	t.Setenv(envMailNoTLS, "false")

	cfg, err := Load([]string{
		"--cycle-interval=60s",
		"--http-address=0.0.0.0",
		"--http-port=8088",
		"--http-basic-auth-username=cli-admin",
		"--http-basic-auth-password=cli-secret",
		"--mail-endpoint=smtp.cli.example.com",
		"--mail-port=1025",
		"--mail-username=bob",
		"--mail-password=override",
		"--mail-sender=alerts@example.com",
		"--mail-no-tls=true",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
	}
	if cfg.HTTPServer.Address != "0.0.0.0" {
		t.Fatalf("HTTPServer.Address = %q, want %q", cfg.HTTPServer.Address, "0.0.0.0")
	}
	if cfg.HTTPServer.Port != 8088 {
		t.Fatalf("HTTPServer.Port = %v, want %v", cfg.HTTPServer.Port, 8088)
	}
	if cfg.HTTPServer.BasicAuthUsername != "cli-admin" {
		t.Fatalf("HTTPServer.BasicAuthUsername = %q, want %q", cfg.HTTPServer.BasicAuthUsername, "cli-admin")
	}
	if cfg.HTTPServer.BasicAuthPassword != "cli-secret" {
		t.Fatalf("HTTPServer.BasicAuthPassword = %q, want %q", cfg.HTTPServer.BasicAuthPassword, "cli-secret")
	}
	if cfg.Mailserver.Endpoint != "smtp.cli.example.com" {
		t.Fatalf("Mailserver.Endpoint = %q, want %q", cfg.Mailserver.Endpoint, "smtp.cli.example.com")
	}
	if cfg.Mailserver.Port != 1025 {
		t.Fatalf("Mailserver.Port = %v, want %v", cfg.Mailserver.Port, 1025)
	}
	if cfg.Mailserver.Username != "bob" {
		t.Fatalf("Mailserver.Username = %q, want %q", cfg.Mailserver.Username, "bob")
	}
	if cfg.Mailserver.Password != "override" {
		t.Fatalf("Mailserver.Password = %q, want %q", cfg.Mailserver.Password, "override")
	}
	if cfg.Mailserver.Sender != "alerts@example.com" {
		t.Fatalf("Mailserver.Sender = %q, want %q", cfg.Mailserver.Sender, "alerts@example.com")
	}
	if cfg.Mailserver.NoTLS != true {
		t.Fatalf("Mailserver.NoTLS = %v, want %v", cfg.Mailserver.NoTLS, true)
	}
}

func TestLoadFormatEquivalence(t *testing.T) {
	t.Setenv(envCycleInterval, "")

	cfgA, err := Load([]string{"--cycle-interval=60s"})
	if err != nil {
		t.Fatalf("Load(60s) error = %v", err)
	}
	cfgB, err := Load([]string{"--cycle-interval=1m"})
	if err != nil {
		t.Fatalf("Load(1m) error = %v", err)
	}

	if cfgA.CycleInterval != cfgB.CycleInterval {
		t.Fatalf("60s parsed as %v, 1m parsed as %v; want equal", cfgA.CycleInterval, cfgB.CycleInterval)
	}
}
