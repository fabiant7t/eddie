package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRelativePathWithMultiDocument(t *testing.T) {
	tempDir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	relativePath := filepath.Join("foo", "bar", "myspec.yaml")
	absolutePath := filepath.Join(tempDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	writeSpecFile(t, absolutePath, twoDocSpecYAML())

	specs, err := Parse(relativePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 2)
	}
	if specs[0].HTTP.Name != "first" || specs[1].HTTP.Name != "second" {
		t.Fatalf("unexpected spec names: %q, %q", specs[0].HTTP.Name, specs[1].HTTP.Name)
	}
	if specs[0].SourcePath != absolutePath || specs[1].SourcePath != absolutePath {
		t.Fatalf("unexpected source path: %q / %q", specs[0].SourcePath, specs[1].SourcePath)
	}
}

func TestParseTildePath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	path := filepath.Join(homeDir, "specs", "foo.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	writeSpecFile(t, path, oneDocSpecYAML("tilde"))

	specs, err := Parse("~/specs/foo.yaml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 1)
	}
	if specs[0].HTTP.Name != "tilde" {
		t.Fatalf("spec name = %q, want %q", specs[0].HTTP.Name, "tilde")
	}
}

func TestParseAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baz.yaml")
	writeSpecFile(t, path, oneDocSpecYAML("absolute"))

	specs, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 1)
	}
	if specs[0].HTTP.Name != "absolute" {
		t.Fatalf("spec name = %q, want %q", specs[0].HTTP.Name, "absolute")
	}
}

func TestParseGlobPath(t *testing.T) {
	tempDir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	pathA := filepath.Join(tempDir, "spec.d", "a.yaml")
	pathB := filepath.Join(tempDir, "spec.d", "nested", "b.yaml")
	if err := os.MkdirAll(filepath.Dir(pathB), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	writeSpecFile(t, pathA, oneDocSpecYAML("a"))
	writeSpecFile(t, pathB, oneDocSpecYAML("b"))

	specs, err := Parse("spec.d/**/*.yaml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 2)
	}
	if specs[0].HTTP.Name != "a" || specs[1].HTTP.Name != "b" {
		t.Fatalf("unexpected spec names: %q, %q", specs[0].HTTP.Name, specs[1].HTTP.Name)
	}
}

func TestIsActiveDefaultAndExplicitTrueDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enabled.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nhttp:\n  name: default-enabled\n---\nversion: 1\nhttp:\n  name: disabled\n  disabled: true\n")

	specs, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 2)
	}
	if !specs[0].IsActive() {
		t.Fatalf("spec[0] should be active by default")
	}
	if specs[1].IsActive() {
		t.Fatalf("spec[1] should be inactive when disabled=true")
	}
}

func TestParseRejectsEmptyName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-name.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nhttp:\n  name: \"\"\n  method: GET\n  url: http://example.com\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseRejectsDuplicateNames(t *testing.T) {
	tempDir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	pathA := filepath.Join(tempDir, "spec.d", "a.yaml")
	pathB := filepath.Join(tempDir, "spec.d", "nested", "b.yaml")
	if err := os.MkdirAll(filepath.Dir(pathB), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	writeSpecFile(t, pathA, oneDocSpecYAML("duplicate-name"))
	writeSpecFile(t, pathB, oneDocSpecYAML("duplicate-name"))

	_, err = Parse("spec.d/**/*.yaml")
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func writeSpecFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func oneDocSpecYAML(name string) string {
	return "---\nversion: 1\nhttp:\n  name: " + name + "\n  method: GET\n  url: http://example.com\n"
}

func twoDocSpecYAML() string {
	return "---\nversion: 1\nhttp:\n  name: first\n  method: GET\n  url: http://example.com/one\n---\nversion: 1\nhttp:\n  name: second\n  method: GET\n  url: http://example.com/two\n"
}
