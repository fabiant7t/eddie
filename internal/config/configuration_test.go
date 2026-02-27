package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv(envCycleInterval, "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv(envCycleInterval, "1m")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
	}
}

func TestLoadCLIOverridesEnv(t *testing.T) {
	t.Setenv(envCycleInterval, "1m")

	cfg, err := Load([]string{"--cycle-interval=60s"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CycleInterval != 60*time.Second {
		t.Fatalf("CycleInterval = %v, want %v", cfg.CycleInterval, 60*time.Second)
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
