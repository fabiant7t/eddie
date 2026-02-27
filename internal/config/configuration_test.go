package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv(envCycleInterval, "")
	t.Setenv(envMailEndpoint, "")
	t.Setenv(envMailPort, "")
	t.Setenv(envMailUsername, "")
	t.Setenv(envMailPassword, "")
	t.Setenv(envMailNoTLS, "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
	}
	if cfg.Mailserver.Port != defaultMailPort {
		t.Fatalf("Mailserver.Port = %v, want %v", cfg.Mailserver.Port, defaultMailPort)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv(envCycleInterval, "1m")
	t.Setenv(envMailEndpoint, "smtp.example.com")
	t.Setenv(envMailPort, "2525")
	t.Setenv(envMailUsername, "alice")
	t.Setenv(envMailPassword, "secret")
	t.Setenv(envMailNoTLS, "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
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
	if cfg.Mailserver.NoTLS != true {
		t.Fatalf("Mailserver.NoTLS = %v, want %v", cfg.Mailserver.NoTLS, true)
	}
}

func TestLoadCLIOverridesEnv(t *testing.T) {
	t.Setenv(envCycleInterval, "1m")
	t.Setenv(envMailEndpoint, "smtp.example.com")
	t.Setenv(envMailPort, "2525")
	t.Setenv(envMailUsername, "alice")
	t.Setenv(envMailPassword, "secret")
	t.Setenv(envMailNoTLS, "false")

	cfg, err := Load([]string{
		"--cycle-interval=60s",
		"--mail-endpoint=smtp.cli.example.com",
		"--mail-port=1025",
		"--mail-username=bob",
		"--mail-password=override",
		"--mail-no-tls=true",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
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
