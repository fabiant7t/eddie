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
	Version    int        `yaml:"version"`
	HTTP       *HTTPSpec  `yaml:"http"`
	TLS        *TLSSpec   `yaml:"tls"`
	Probe      *ProbeSpec `yaml:"probe"`
	SourcePath string     `yaml:"-"`
}

// HTTPSpec defines the HTTP test configuration.
type HTTPSpec struct {
	Disabled        bool              `yaml:"disabled"`
	Name            string            `yaml:"name"`
	Method          string            `yaml:"method"`
	FollowRedirects bool              `yaml:"follow_redirects"`
	InsecureSkipTLS bool              `yaml:"insecure_skip_verify"`
	URL             string            `yaml:"url"`
	Args            map[string]string `yaml:"args"`
	Headers         map[string]string `yaml:"headers"`
	MailReceivers   []string          `yaml:"mail_receivers"`
	Timeout         time.Duration     `yaml:"timeout"`
	Expect          HTTPExpect        `yaml:"expect"`
	Cycles          SpecCycles        `yaml:"cycles"`
	OnFailure       string            `yaml:"on_failure"`
	OnResolved      string            `yaml:"on_resolved"`
}

// TLSSpec defines the TLS test configuration.
type TLSSpec struct {
	Disabled         bool          `yaml:"disabled"`
	Name             string        `yaml:"name"`
	Host             string        `yaml:"host"`
	Port             int           `yaml:"port"`
	ServerName       string        `yaml:"server_name"`
	Verify           *bool         `yaml:"verify"`
	RejectSelfSigned *bool         `yaml:"reject_selfsigned"`
	MinVersion       string        `yaml:"min_version"`
	Timeout          time.Duration `yaml:"timeout"`
	CertMinDaysValid *int          `yaml:"cert_min_days_valid"`
	MailReceivers    []string      `yaml:"mail_receivers"`
	Cycles           SpecCycles    `yaml:"cycles"`
	OnFailure        string        `yaml:"on_failure"`
	OnResolved       string        `yaml:"on_resolved"`
}

// ProbeSpec defines composable multi-request assertions.
type ProbeSpec struct {
	Disabled      bool           `yaml:"disabled"`
	Name          string         `yaml:"name"`
	Requests      []ProbeRequest `yaml:"requests"`
	Extracts      []ProbeExtract `yaml:"extracts"`
	Asserts       []ProbeAssert  `yaml:"asserts"`
	MailReceivers []string       `yaml:"mail_receivers"`
	Cycles        SpecCycles     `yaml:"cycles"`
	OnFailure     string         `yaml:"on_failure"`
	OnResolved    string         `yaml:"on_resolved"`
}

// ProbeRequest defines one HTTP request executed by a probe.
type ProbeRequest struct {
	ID              string            `yaml:"id"`
	Method          string            `yaml:"method"`
	URL             string            `yaml:"url"`
	Args            map[string]string `yaml:"args"`
	Headers         map[string]string `yaml:"headers"`
	FollowRedirects bool              `yaml:"follow_redirects"`
	InsecureSkipTLS bool              `yaml:"insecure_skip_verify"`
	Timeout         time.Duration     `yaml:"timeout"`
}

// ProbeExtract defines a value extraction from one request result.
type ProbeExtract struct {
	ID         string      `yaml:"id"`
	From       string      `yaml:"from"`
	Source     ProbeSource `yaml:"source"`
	Transforms []string    `yaml:"transforms"`
}

// ProbeSource defines where an extract reads from.
type ProbeSource struct {
	Type string `yaml:"type"`
	Key  string `yaml:"key"`
}

// ProbeAssert defines one assertion over extracted values.
type ProbeAssert struct {
	ID     string         `yaml:"id"`
	Op     string         `yaml:"op"`
	Left   ProbeOperand   `yaml:"left"`
	Right  ProbeOperand   `yaml:"right"`
	Values []ProbeOperand `yaml:"values"`
}

// ProbeOperand is either a reference to an extract ID or a literal value.
type ProbeOperand struct {
	Ref   string `yaml:"ref"`
	Value any    `yaml:"value"`
}

// IsActive reports whether the spec should be used.
// Specs are active by default unless explicitly set to disabled: true.
func (s Spec) IsActive() bool {
	switch {
	case s.HTTP != nil:
		return !s.HTTP.Disabled
	case s.TLS != nil:
		return !s.TLS.Disabled
	case s.Probe != nil:
		return !s.Probe.Disabled
	default:
		return false
	}
}

// Kind returns the spec type (http/tls).
func (s Spec) Kind() string {
	switch {
	case s.HTTP != nil:
		return "http"
	case s.TLS != nil:
		return "tls"
	case s.Probe != nil:
		return "probe"
	default:
		return "unknown"
	}
}

// Name returns the spec name regardless of type.
func (s Spec) Name() string {
	switch {
	case s.HTTP != nil:
		return s.HTTP.Name
	case s.TLS != nil:
		return s.TLS.Name
	case s.Probe != nil:
		return s.Probe.Name
	default:
		return ""
	}
}

// ID returns the stable identity for state tracking.
func (s Spec) ID() string {
	name := strings.TrimSpace(s.Name())
	kind := s.Kind()
	if name == "" || kind == "unknown" {
		return ""
	}
	return kind + ":" + name
}

// HTTPExpect defines expected HTTP response checks.
type HTTPExpect struct {
	Code           int               `yaml:"code"`
	CodeAnyOf      []int             `yaml:"code_any_of"`
	Header         map[string]string `yaml:"header"`
	HeaderContains map[string]string `yaml:"header_contains"`
	Body           HTTPExpectBody    `yaml:"body"`
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
		definedKinds := 0
		if sp.HTTP != nil {
			definedKinds++
		}
		if sp.TLS != nil {
			definedKinds++
		}
		if sp.Probe != nil {
			definedKinds++
		}
		if definedKinds != 1 {
			return fmt.Errorf("spec in %q must define exactly one of http, tls, or probe", sp.SourcePath)
		}

		switch {
		case sp.HTTP != nil:
			name := strings.TrimSpace(sp.HTTP.Name)
			if name == "" {
				return fmt.Errorf("spec in %q has empty http.name", sp.SourcePath)
			}
			if err := validateMailReceivers(sp.SourcePath, "http", sp.HTTP.MailReceivers); err != nil {
				return err
			}

			identity := "http:" + name
			if firstSource, ok := seen[identity]; ok {
				return fmt.Errorf("duplicate http.name %q found in %q and %q", name, firstSource, sp.SourcePath)
			}
			seen[identity] = sp.SourcePath
		case sp.TLS != nil:
			name := strings.TrimSpace(sp.TLS.Name)
			if name == "" {
				return fmt.Errorf("spec in %q has empty tls.name", sp.SourcePath)
			}
			if sp.TLS.CertMinDaysValid != nil && *sp.TLS.CertMinDaysValid < 0 {
				return fmt.Errorf("spec in %q has negative tls.cert_min_days_valid", sp.SourcePath)
			}
			if err := validateMailReceivers(sp.SourcePath, "tls", sp.TLS.MailReceivers); err != nil {
				return err
			}

			identity := "tls:" + name
			if firstSource, ok := seen[identity]; ok {
				return fmt.Errorf("duplicate tls.name %q found in %q and %q", name, firstSource, sp.SourcePath)
			}
			seen[identity] = sp.SourcePath
		case sp.Probe != nil:
			name := strings.TrimSpace(sp.Probe.Name)
			if name == "" {
				return fmt.Errorf("spec in %q has empty probe.name", sp.SourcePath)
			}
			if err := validateMailReceivers(sp.SourcePath, "probe", sp.Probe.MailReceivers); err != nil {
				return err
			}
			if err := validateProbeSpec(sp.SourcePath, sp.Probe); err != nil {
				return err
			}

			identity := "probe:" + name
			if firstSource, ok := seen[identity]; ok {
				return fmt.Errorf("duplicate probe.name %q found in %q and %q", name, firstSource, sp.SourcePath)
			}
			seen[identity] = sp.SourcePath
		}
	}

	return nil
}

func validateMailReceivers(sourcePath, kind string, receivers []string) error {
	for idx, receiver := range receivers {
		if strings.TrimSpace(receiver) == "" {
			return fmt.Errorf("spec in %q has empty %s.mail_receivers[%d]", sourcePath, kind, idx)
		}
	}
	return nil
}

func validateProbeSpec(sourcePath string, probe *ProbeSpec) error {
	if probe == nil {
		return fmt.Errorf("spec in %q has nil probe", sourcePath)
	}
	if len(probe.Requests) == 0 {
		return fmt.Errorf("spec in %q must define at least one probe.requests item", sourcePath)
	}
	if len(probe.Extracts) == 0 {
		return fmt.Errorf("spec in %q must define at least one probe.extracts item", sourcePath)
	}
	if len(probe.Asserts) == 0 {
		return fmt.Errorf("spec in %q must define at least one probe.asserts item", sourcePath)
	}

	requestIDs := make(map[string]struct{}, len(probe.Requests))
	for idx, req := range probe.Requests {
		id := strings.TrimSpace(req.ID)
		if id == "" {
			return fmt.Errorf("spec in %q has empty probe.requests[%d].id", sourcePath, idx)
		}
		if _, exists := requestIDs[id]; exists {
			return fmt.Errorf("spec in %q has duplicate probe.requests.id %q", sourcePath, id)
		}
		requestIDs[id] = struct{}{}
		if strings.TrimSpace(req.URL) == "" {
			return fmt.Errorf("spec in %q has empty probe.requests[%d].url", sourcePath, idx)
		}
	}

	extractIDs := make(map[string]struct{}, len(probe.Extracts))
	for idx, ex := range probe.Extracts {
		id := strings.TrimSpace(ex.ID)
		if id == "" {
			return fmt.Errorf("spec in %q has empty probe.extracts[%d].id", sourcePath, idx)
		}
		if _, exists := extractIDs[id]; exists {
			return fmt.Errorf("spec in %q has duplicate probe.extracts.id %q", sourcePath, id)
		}
		extractIDs[id] = struct{}{}

		from := strings.TrimSpace(ex.From)
		if from == "" {
			return fmt.Errorf("spec in %q has empty probe.extracts[%d].from", sourcePath, idx)
		}
		if _, exists := requestIDs[from]; !exists {
			return fmt.Errorf("spec in %q references unknown probe request %q", sourcePath, from)
		}
		sourceType := strings.TrimSpace(ex.Source.Type)
		switch sourceType {
		case "header":
			if strings.TrimSpace(ex.Source.Key) == "" {
				return fmt.Errorf("spec in %q has empty probe.extracts[%d].source.key for header source", sourcePath, idx)
			}
		case "body", "json":
		default:
			return fmt.Errorf("spec in %q has unsupported probe.extracts[%d].source.type %q", sourcePath, idx, sourceType)
		}
	}

	assertionIDs := make(map[string]struct{}, len(probe.Asserts))
	for idx, assertion := range probe.Asserts {
		assertID := strings.TrimSpace(assertion.ID)
		if assertID == "" {
			return fmt.Errorf("spec in %q has empty probe.asserts[%d].id", sourcePath, idx)
		}
		if _, exists := assertionIDs[assertID]; exists {
			return fmt.Errorf("spec in %q has duplicate probe.asserts.id %q", sourcePath, assertID)
		}
		assertionIDs[assertID] = struct{}{}
		op := strings.TrimSpace(assertion.Op)
		switch op {
		case "eq", "neq", "gt", "gte", "lt", "lte", "contains", "matches", "all_equal":
		default:
			return fmt.Errorf("spec in %q has unsupported probe.asserts[%d].op %q", sourcePath, idx, assertion.Op)
		}
		if op == "all_equal" {
			if len(assertion.Values) < 2 {
				return fmt.Errorf("spec in %q requires probe.asserts[%d].values with at least two operands for all_equal", sourcePath, idx)
			}
			for valueIdx, operand := range assertion.Values {
				if err := validateProbeOperand(sourcePath, extractIDs, operand, fmt.Sprintf("probe.asserts[%d].values[%d]", idx, valueIdx)); err != nil {
					return err
				}
			}
			continue
		}
		if len(assertion.Values) > 0 {
			return fmt.Errorf("spec in %q does not allow probe.asserts[%d].values for op %q", sourcePath, idx, op)
		}
		if err := validateProbeOperand(sourcePath, extractIDs, assertion.Left, fmt.Sprintf("probe.asserts[%d].left", idx)); err != nil {
			return err
		}
		if err := validateProbeOperand(sourcePath, extractIDs, assertion.Right, fmt.Sprintf("probe.asserts[%d].right", idx)); err != nil {
			return err
		}
	}

	return nil
}

func validateProbeOperand(sourcePath string, extractIDs map[string]struct{}, operand ProbeOperand, fieldPath string) error {
	ref := strings.TrimSpace(operand.Ref)
	hasRef := ref != ""
	hasValue := operand.Value != nil
	if hasRef == hasValue {
		return fmt.Errorf("spec in %q requires exactly one of %s.ref or %s.value", sourcePath, fieldPath, fieldPath)
	}
	if hasRef {
		if _, exists := extractIDs[ref]; !exists {
			return fmt.Errorf("spec in %q references unknown probe extract %q in %s.ref", sourcePath, ref, fieldPath)
		}
	}
	return nil
}
