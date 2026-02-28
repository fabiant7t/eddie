package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSpecBaseDir(t *testing.T) {
	tempDir := t.TempDir()
	specDir := filepath.Join(tempDir, "spec.d")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir spec dir: %v", err)
	}
	specFile := filepath.Join(specDir, "api.yaml")
	if err := os.WriteFile(specFile, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("mkdir home dir: %v", err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

	relDir := filepath.Join(tempDir, "relative")
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		t.Fatalf("mkdir relative dir: %v", err)
	}
	relSpecDir := filepath.Join(relDir, "specs")
	if err := os.MkdirAll(relSpecDir, 0o755); err != nil {
		t.Fatalf("mkdir relative specs dir: %v", err)
	}

	workingDir := filepath.Join(tempDir, "work")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}

	homeSpecDir := filepath.Join(homeDir, "spec.d")
	if err := os.MkdirAll(homeSpecDir, 0o755); err != nil {
		t.Fatalf("mkdir home spec dir: %v", err)
	}

	absDir, err := resolveSpecBaseDir(specDir)
	if err != nil {
		t.Fatalf("resolveSpecBaseDir(dir) error: %v", err)
	}
	if absDir != specDir {
		t.Fatalf("resolveSpecBaseDir(dir) = %q, want %q", absDir, specDir)
	}

	absFile, err := resolveSpecBaseDir(specFile)
	if err != nil {
		t.Fatalf("resolveSpecBaseDir(file) error: %v", err)
	}
	if absFile != specDir {
		t.Fatalf("resolveSpecBaseDir(file) = %q, want %q", absFile, specDir)
	}

	globDir, err := resolveSpecBaseDir(filepath.Join(specDir, "*.yaml"))
	if err != nil {
		t.Fatalf("resolveSpecBaseDir(glob) error: %v", err)
	}
	if globDir != specDir {
		t.Fatalf("resolveSpecBaseDir(glob) = %q, want %q", globDir, specDir)
	}

	homeResolved, err := resolveSpecBaseDir(filepath.Join("~", "spec.d", "*.yaml"))
	if err != nil {
		t.Fatalf("resolveSpecBaseDir(home) error: %v", err)
	}
	if homeResolved != homeSpecDir {
		t.Fatalf("resolveSpecBaseDir(home) = %q, want %q", homeResolved, homeSpecDir)
	}

	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	relInput := filepath.Join("..", "relative", "specs", "*.yaml")
	relResolved, err := resolveSpecBaseDir(relInput)
	if err != nil {
		t.Fatalf("resolveSpecBaseDir(relative) error: %v", err)
	}
	if relResolved != relSpecDir {
		t.Fatalf("resolveSpecBaseDir(relative) = %q, want %q", relResolved, relSpecDir)
	}

	if _, err := resolveSpecBaseDir("~unknown/specs"); err == nil || !strings.Contains(err.Error(), "unsupported home expansion") {
		t.Fatalf("resolveSpecBaseDir(~unknown) error = %v, want unsupported home expansion", err)
	}
}
