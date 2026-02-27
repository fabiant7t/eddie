package main

import (
	"fmt"
	"os"

	"github.com/fabiant7t/appordown/internal/config"
)

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("appordown (cycle-interval=%s)\n", cfg.CycleInterval)
}
