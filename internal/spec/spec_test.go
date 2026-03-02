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
	if specs[0].Name() != "first" || specs[1].Name() != "second" {
		t.Fatalf("unexpected spec names: %q, %q", specs[0].Name(), specs[1].Name())
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
	if specs[0].Name() != "tilde" {
		t.Fatalf("spec name = %q, want %q", specs[0].Name(), "tilde")
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
	if specs[0].Name() != "absolute" {
		t.Fatalf("spec name = %q, want %q", specs[0].Name(), "absolute")
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
	if specs[0].Name() != "a" || specs[1].Name() != "b" {
		t.Fatalf("unexpected spec names: %q, %q", specs[0].Name(), specs[1].Name())
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

func TestParseRejectsMissingType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-type.yaml")
	writeSpecFile(t, path, "---\nversion: 1\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseRejectsBothTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "both-types.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nhttp:\n  name: foo\n  url: http://example.com\n  method: GET\ntls:\n  name: bar\n  host: example.com\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseTLSName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tls.yaml")
	writeSpecFile(t, path, tlsDocSpecYAML("tls-name"))

	specs, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 1)
	}
	if specs[0].Name() != "tls-name" || specs[0].Kind() != "tls" {
		t.Fatalf("unexpected spec identity: %q/%q", specs[0].Kind(), specs[0].Name())
	}
}

func TestParseProbeName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nprobe:\n  name: compare-json\n  requests:\n    - id: a\n      url: https://example.com/a.json\n    - id: b\n      url: https://example.com/b.json\n  extracts:\n    - id: a_json\n      from: a\n      source:\n        type: json\n    - id: b_json\n      from: b\n      source:\n        type: json\n  asserts:\n    - id: same\n      op: all_equal\n      values:\n        - ref: a_json\n        - ref: b_json\n")

	specs, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 1)
	}
	if specs[0].Name() != "compare-json" || specs[0].Kind() != "probe" {
		t.Fatalf("unexpected spec identity: %q/%q", specs[0].Kind(), specs[0].Name())
	}
}

func TestParseProbeRejectsUnknownExtractRef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe-bad-ref.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nprobe:\n  name: compare-json\n  requests:\n    - id: a\n      url: https://example.com/a.json\n  extracts:\n    - id: a_json\n      from: a\n      source:\n        type: json\n  asserts:\n    - id: same\n      op: eq\n      left:\n        ref: a_json\n      right:\n        ref: missing\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseProbeWithJSONPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe-json-path.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nprobe:\n  name: dms-freshness\n  requests:\n    - id: dms\n      url: https://example.com/dms.json\n  extracts:\n    - id: age\n      from: dms\n      source:\n        type: json_path\n        key: $.updated\n      transforms:\n        - age_seconds\n  asserts:\n    - id: age-check\n      op: lte\n      left:\n        ref: age\n      right:\n        value: 600\n")

	_, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseS3Name(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s3.yaml")
	writeSpecFile(t, path, "---\nversion: 1\ns3:\n  name: logs-check\n  endpoint: https://s3.example.com\n  path_style: true\n  auth:\n    mode: static\n    access_key_id: foo\n    secret_access_key: bar\n  list:\n    bucket: logs\n    prefix: app/\n    expect:\n      count_gte: 1\n")

	specs, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), 1)
	}
	if specs[0].Name() != "logs-check" || specs[0].Kind() != "s3" {
		t.Fatalf("unexpected spec identity: %q/%q", specs[0].Kind(), specs[0].Name())
	}
}

func TestParseRejectsNegativeEveryCycles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "negative-every-cycles.yaml")
	writeSpecFile(t, path, "---\nversion: 1\ns3:\n  name: logs-check\n  every_cycles: -1\n  list:\n    bucket: logs\n    expect:\n      count_gte: 1\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseRejectsS3StaticAuthWithoutSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s3-static-auth.yaml")
	writeSpecFile(t, path, "---\nversion: 1\ns3:\n  name: logs-check\n  auth:\n    mode: static\n    access_key_id: foo\n  list:\n    bucket: logs\n    expect:\n      count_gte: 1\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseRejectsEmptyHTTPMailReceiver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-http-mail-receiver.yaml")
	writeSpecFile(t, path, "---\nversion: 1\nhttp:\n  name: foo\n  method: GET\n  url: http://example.com\n  mail_receivers:\n    - ops@example.com\n    - \"  \"\n")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseRejectsEmptyTLSMailReceiver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-tls-mail-receiver.yaml")
	writeSpecFile(t, path, "---\nversion: 1\ntls:\n  name: foo\n  host: example.com\n  mail_receivers:\n    - alerts@example.com\n    - \"\"\n")

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

func tlsDocSpecYAML(name string) string {
	return "---\nversion: 1\ntls:\n  name: " + name + "\n  host: example.com\n"
}
