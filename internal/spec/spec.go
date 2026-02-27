package spec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Spec defines one test spec document.
type Spec struct {
	Version    int      `yaml:"version"`
	HTTP       HTTPSpec `yaml:"http"`
	SourcePath string   `yaml:"-"`
}

// HTTPSpec defines the HTTP test configuration.
type HTTPSpec struct {
	Disabled        bool              `yaml:"disabled"`
	Name            string            `yaml:"name"`
	Method          string            `yaml:"method"`
	FollowRedirects bool              `yaml:"follow_redirects"`
	URL             string            `yaml:"url"`
	Args            map[string]string `yaml:"args"`
	Timeout         time.Duration     `yaml:"timeout"`
	Expect          HTTPExpect        `yaml:"expect"`
	Cycles          SpecCycles        `yaml:"cycles"`
	OnFailure       string            `yaml:"on_failure"`
	OnSuccess       string            `yaml:"on_success"`
}

// IsActive reports whether the spec should be used.
// Specs are active by default unless explicitly set to disabled: true.
func (s Spec) IsActive() bool {
	return !s.HTTP.Disabled
}

// HTTPExpect defines expected HTTP response checks.
type HTTPExpect struct {
	Code   int               `yaml:"code"`
	Header map[string]string `yaml:"header"`
	Body   HTTPExpectBody    `yaml:"body"`
}

// HTTPExpectBody defines expected HTTP response body checks.
type HTTPExpectBody struct {
	Exact    string `yaml:"exact"`
	Contains string `yaml:"contains"`
}

// SpecCycles defines success/failure cycle counters for alerting logic.
type SpecCycles struct {
	Failure int `yaml:"failure"`
	Success int `yaml:"success"`
}

// Parse loads one or more specs from file path expression.
// The expression supports relative paths, absolute paths, home expansion (~),
// and glob patterns (including **).
func Parse(pathExpression string) ([]Spec, error) {
	resolvedExpr, err := resolvePathExpression(pathExpression)
	if err != nil {
		return nil, err
	}

	paths, err := resolveSpecPaths(resolvedExpr)
	if err != nil {
		return nil, err
	}

	specs := make([]Spec, 0)
	for _, path := range paths {
		fileSpecs, err := parseSpecFile(path)
		if err != nil {
			return nil, err
		}
		specs = append(specs, fileSpecs...)
	}
	if err := validateSpecNames(specs); err != nil {
		return nil, err
	}

	return specs, nil
}

func resolvePathExpression(pathExpression string) (string, error) {
	expr := strings.TrimSpace(pathExpression)
	if expr == "" {
		return "", fmt.Errorf("path expression cannot be empty")
	}

	if expr == "~" || strings.HasPrefix(expr, "~/") || strings.HasPrefix(expr, "~\\") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		trimmed := strings.TrimPrefix(strings.TrimPrefix(expr, "~/"), "~\\")
		expr = filepath.Join(homeDir, trimmed)
	} else if strings.HasPrefix(expr, "~") {
		return "", fmt.Errorf("unsupported home expansion: %q", expr)
	}

	if filepath.IsAbs(expr) {
		return filepath.Clean(expr), nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}

	return filepath.Clean(filepath.Join(workingDir, expr)), nil
}

func resolveSpecPaths(resolvedExpression string) ([]string, error) {
	if strings.ContainsAny(resolvedExpression, "*?[") {
		paths, err := doublestar.FilepathGlob(resolvedExpression)
		if err != nil {
			return nil, fmt.Errorf("resolve glob %q: %w", resolvedExpression, err)
		}
		sort.Strings(paths)
		if len(paths) == 0 {
			return nil, fmt.Errorf("no spec files matched %q", resolvedExpression)
		}
		return paths, nil
	}

	if _, err := os.Stat(resolvedExpression); err != nil {
		return nil, fmt.Errorf("stat %q: %w", resolvedExpression, err)
	}
	return []string{resolvedExpression}, nil
}

func parseSpecFile(path string) ([]Spec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec file %q: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	specs := make([]Spec, 0)
	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode yaml document in %q: %w", path, err)
		}

		if isEmptyYAMLDocument(&doc) {
			continue
		}

		var spec Spec
		if err := doc.Decode(&spec); err != nil {
			return nil, fmt.Errorf("decode spec in %q: %w", path, err)
		}
		spec.SourcePath = path
		specs = append(specs, spec)
	}

	return specs, nil
}

func isEmptyYAMLDocument(doc *yaml.Node) bool {
	if doc == nil || len(doc.Content) == 0 {
		return true
	}

	root := doc.Content[0]
	if root.Kind == 0 {
		return true
	}
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return true
	}

	return false
}

func validateSpecNames(specs []Spec) error {
	seen := make(map[string]string, len(specs))
	for _, sp := range specs {
		name := strings.TrimSpace(sp.HTTP.Name)
		if name == "" {
			return fmt.Errorf("spec in %q has empty http.name", sp.SourcePath)
		}

		identity := "http:" + name
		if firstSource, ok := seen[identity]; ok {
			return fmt.Errorf("duplicate http.name %q found in %q and %q", name, firstSource, sp.SourcePath)
		}
		seen[identity] = sp.SourcePath
	}

	return nil
}
