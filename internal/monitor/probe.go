package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fabiant7t/eddie/internal/spec"
)

type probeRequestResult struct {
	StatusCode int
	Headers    http.Header
	Body       string
}

func validateProbeSpec(ctx context.Context, parsedSpec spec.Spec) error {
	probe := parsedSpec.Probe
	if probe == nil {
		return fmt.Errorf("missing probe spec")
	}

	requests := make(map[string]probeRequestResult, len(probe.Requests))
	for _, request := range probe.Requests {
		result, err := executeProbeRequest(ctx, request)
		if err != nil {
			return fmt.Errorf("request %q: %w", request.ID, err)
		}
		requests[request.ID] = result
	}

	extracted := make(map[string]any, len(probe.Extracts))
	for _, extract := range probe.Extracts {
		requestResult, ok := requests[extract.From]
		if !ok {
			return fmt.Errorf("extract %q references unknown request %q", extract.ID, extract.From)
		}
		value, err := extractProbeValue(requestResult, extract)
		if err != nil {
			return fmt.Errorf("extract %q: %w", extract.ID, err)
		}
		extracted[extract.ID] = value
	}

	for _, assertion := range probe.Asserts {
		if err := evaluateProbeAssert(assertion, extracted); err != nil {
			return fmt.Errorf("assert %q: %w", assertion.ID, err)
		}
	}

	return nil
}

func executeProbeRequest(ctx context.Context, request spec.ProbeRequest) (probeRequestResult, error) {
	reqTimeout := request.Timeout
	if reqTimeout <= 0 {
		reqTimeout = 15 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, reqTimeout)
	defer cancel()

	targetURL, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil {
		return probeRequestResult{}, fmt.Errorf("parse url: %w", err)
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return probeRequestResult{}, fmt.Errorf("url must include scheme and host: %q", request.URL)
	}

	if len(request.Args) > 0 {
		query := targetURL.Query()
		for key, value := range request.Args {
			query.Set(key, expandProbeTemplate(value))
		}
		targetURL.RawQuery = query.Encode()
	}

	method := strings.TrimSpace(request.Method)
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(reqCtx, method, targetURL.String(), nil)
	if err != nil {
		return probeRequestResult{}, fmt.Errorf("build request: %w", err)
	}
	for headerName, value := range request.Headers {
		expandedValue := expandProbeTemplate(value)
		if strings.EqualFold(headerName, "host") {
			req.Host = expandedValue
			continue
		}
		req.Header.Set(headerName, expandedValue)
	}

	client := &http.Client{Timeout: reqTimeout}
	if request.InsecureSkipTLS {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	if !request.FollowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return probeRequestResult{}, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return probeRequestResult{}, fmt.Errorf("read response body: %w", err)
	}

	return probeRequestResult{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       string(bodyBytes),
	}, nil
}

func expandProbeTemplate(raw string) string {
	out := raw
	out = strings.ReplaceAll(out, "{unix_ts}", strconv.FormatInt(time.Now().Unix(), 10))
	return out
}

func extractProbeValue(result probeRequestResult, extract spec.ProbeExtract) (any, error) {
	var value any
	switch strings.TrimSpace(extract.Source.Type) {
	case "header":
		value = result.Headers.Get(extract.Source.Key)
	case "body":
		value = result.Body
	case "json":
		var decoded any
		if err := json.Unmarshal([]byte(result.Body), &decoded); err != nil {
			return nil, fmt.Errorf("parse json body: %w", err)
		}
		normalized, err := json.Marshal(decoded)
		if err != nil {
			return nil, fmt.Errorf("canonicalize json body: %w", err)
		}
		value = string(normalized)
	case "json_path":
		var decoded any
		if err := json.Unmarshal([]byte(result.Body), &decoded); err != nil {
			return nil, fmt.Errorf("parse json body: %w", err)
		}
		resolved, err := resolveJSONPath(decoded, extract.Source.Key)
		if err != nil {
			return nil, err
		}
		value = resolved
	default:
		return nil, fmt.Errorf("unsupported source type %q", extract.Source.Type)
	}

	var err error
	for _, transform := range extract.Transforms {
		value, err = applyProbeTransform(value, transform)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

func applyProbeTransform(value any, transform string) (any, error) {
	switch strings.TrimSpace(transform) {
	case "trim_space":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("transform trim_space requires string value")
		}
		return strings.TrimSpace(text), nil
	case "strip_quotes":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("transform strip_quotes requires string value")
		}
		return strings.Trim(text, `"`), nil
	case "lowercase":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("transform lowercase requires string value")
		}
		return strings.ToLower(text), nil
	case "as_int":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("transform as_int requires string value")
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("transform as_int failed: %w", err)
		}
		return parsed, nil
	case "age_seconds":
		timestamp, err := probeTimeValue(value)
		if err != nil {
			return nil, fmt.Errorf("transform age_seconds failed: %w", err)
		}
		return int64(time.Since(timestamp).Seconds()), nil
	default:
		return nil, fmt.Errorf("unsupported transform %q", transform)
	}
}

func resolveJSONPath(root any, rawPath string) (any, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return nil, fmt.Errorf("json_path cannot be empty")
	}
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return root, nil
	}

	segments := strings.Split(path, ".")
	current := root
	for _, segment := range segments {
		key := strings.TrimSpace(segment)
		if key == "" {
			return nil, fmt.Errorf("invalid json_path %q", rawPath)
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("json_path %q is not addressable at %q", rawPath, key)
		}
		next, exists := object[key]
		if !exists {
			return nil, fmt.Errorf("json_path %q key %q not found", rawPath, key)
		}
		current = next
	}
	return current, nil
}

func evaluateProbeAssert(assertion spec.ProbeAssert, extracted map[string]any) error {
	op := strings.TrimSpace(assertion.Op)
	if op == "all_equal" {
		values := make([]any, 0, len(assertion.Values))
		for _, operand := range assertion.Values {
			value, err := resolveProbeOperand(operand, extracted)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		for idx := 1; idx < len(values); idx++ {
			equal, err := probeValuesEqual(values[0], values[idx])
			if err != nil {
				return err
			}
			if !equal {
				return fmt.Errorf("values are not equal: %v != %v", values[0], values[idx])
			}
		}
		return nil
	}

	left, err := resolveProbeOperand(assertion.Left, extracted)
	if err != nil {
		return err
	}
	right, err := resolveProbeOperand(assertion.Right, extracted)
	if err != nil {
		return err
	}

	switch op {
	case "eq":
		equal, err := probeValuesEqual(left, right)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("values are not equal: %v != %v", left, right)
		}
	case "neq":
		equal, err := probeValuesEqual(left, right)
		if err != nil {
			return err
		}
		if equal {
			return fmt.Errorf("values are equal: %v == %v", left, right)
		}
	case "gt", "gte", "lt", "lte":
		leftNum, err := probeNumericValue(left)
		if err != nil {
			return fmt.Errorf("left operand: %w", err)
		}
		rightNum, err := probeNumericValue(right)
		if err != nil {
			return fmt.Errorf("right operand: %w", err)
		}
		switch op {
		case "gt":
			if !(leftNum > rightNum) {
				return fmt.Errorf("expected %v > %v", leftNum, rightNum)
			}
		case "gte":
			if !(leftNum >= rightNum) {
				return fmt.Errorf("expected %v >= %v", leftNum, rightNum)
			}
		case "lt":
			if !(leftNum < rightNum) {
				return fmt.Errorf("expected %v < %v", leftNum, rightNum)
			}
		case "lte":
			if !(leftNum <= rightNum) {
				return fmt.Errorf("expected %v <= %v", leftNum, rightNum)
			}
		}
	case "contains":
		leftText, ok := left.(string)
		if !ok {
			return fmt.Errorf("left operand for contains must be string")
		}
		rightText, ok := right.(string)
		if !ok {
			return fmt.Errorf("right operand for contains must be string")
		}
		if !strings.Contains(leftText, rightText) {
			return fmt.Errorf("expected %q to contain %q", leftText, rightText)
		}
	case "matches":
		leftText, ok := left.(string)
		if !ok {
			return fmt.Errorf("left operand for matches must be string")
		}
		rightText, ok := right.(string)
		if !ok {
			return fmt.Errorf("right operand for matches must be string")
		}
		matcher, err := regexp.Compile(rightText)
		if err != nil {
			return fmt.Errorf("compile regex %q: %w", rightText, err)
		}
		if !matcher.MatchString(leftText) {
			return fmt.Errorf("expected %q to match %q", leftText, rightText)
		}
	default:
		return fmt.Errorf("unsupported assert op %q", op)
	}

	return nil
}

func resolveProbeOperand(operand spec.ProbeOperand, extracted map[string]any) (any, error) {
	ref := strings.TrimSpace(operand.Ref)
	if ref != "" {
		value, ok := extracted[ref]
		if !ok {
			return nil, fmt.Errorf("unknown extract reference %q", ref)
		}
		return value, nil
	}
	return operand.Value, nil
}

func probeValuesEqual(left, right any) (bool, error) {
	leftNum, leftIsNum := probeMaybeNumeric(left)
	rightNum, rightIsNum := probeMaybeNumeric(right)
	if leftIsNum && rightIsNum {
		return leftNum == rightNum, nil
	}
	return reflect.DeepEqual(left, right), nil
}

func probeMaybeNumeric(value any) (float64, bool) {
	number, err := probeNumericValue(value)
	if err != nil {
		return 0, false
	}
	return number, true
}

func probeNumericValue(value any) (float64, error) {
	switch typed := value.(type) {
	case int:
		return float64(typed), nil
	case int8:
		return float64(typed), nil
	case int16:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case uint:
		return float64(typed), nil
	case uint8:
		return float64(typed), nil
	case uint16:
		return float64(typed), nil
	case uint32:
		return float64(typed), nil
	case uint64:
		return float64(typed), nil
	case float32:
		return float64(typed), nil
	case float64:
		return typed, nil
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q as number: %w", typed, err)
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q as number: %w", typed, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("value %v (%T) is not numeric", value, value)
	}
}

func probeTimeValue(value any) (time.Time, error) {
	switch typed := value.(type) {
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return time.Time{}, fmt.Errorf("empty string")
		}
		if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return time.Unix(unix, 0).UTC(), nil
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05Z07:00",
			"2006-01-02 15:04:05",
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, raw); err == nil {
				return parsed.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("cannot parse %q as timestamp", raw)
	case int:
		return time.Unix(int64(typed), 0).UTC(), nil
	case int64:
		return time.Unix(typed, 0).UTC(), nil
	case float64:
		return time.Unix(int64(typed), 0).UTC(), nil
	case json.Number:
		unix, err := typed.Int64()
		if err != nil {
			return time.Time{}, fmt.Errorf("cannot parse %q as unix seconds: %w", typed, err)
		}
		return time.Unix(unix, 0).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("value %v (%T) is not a timestamp", value, value)
	}
}
