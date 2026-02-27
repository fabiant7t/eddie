package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

const (
	defaultCycleInterval = 60 * time.Second
	envCycleInterval     = "APPORDOWN_CYCLE_INTERVAL"
)

// Configuration holds runtime settings for the app.
type Configuration struct {
	CycleInterval time.Duration
}

// Load parses configuration from environment and CLI args.
// Precedence is: CLI argument, environment, default.
func Load(args []string) (Configuration, error) {
	cfg := Configuration{
		CycleInterval: defaultCycleInterval,
	}

	if raw := os.Getenv(envCycleInterval); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Configuration{}, fmt.Errorf("invalid %s: %w", envCycleInterval, err)
		}
		cfg.CycleInterval = d
	}

	fs := flag.NewFlagSet("appordown", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.DurationVar(&cfg.CycleInterval, "cycle-interval", cfg.CycleInterval, "cycle interval (e.g. 60s, 1m)")
	if err := fs.Parse(args); err != nil {
		return Configuration{}, err
	}

	return cfg, nil
}
