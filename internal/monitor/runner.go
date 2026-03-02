package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/fabiant7t/eddie/internal/mail"
	"github.com/fabiant7t/eddie/internal/spec"
	"github.com/fabiant7t/eddie/internal/state"
)

type transitionType int

const (
	transitionNone transitionType = iota
	transitionFailure
	transitionRecovery
	staleCycleGapMultiplier = 2
)

// Runner executes spec checks in cycles.
type Runner struct {
	specs          []spec.Spec
	cycleInterval  time.Duration
	stateStore     state.Store
	mailService    *mail.Service
	mailRecipients []string
	cycleNumber    uint64
}

// NewRunner creates a monitoring runner.
func NewRunner(
	specs []spec.Spec,
	cycleInterval time.Duration,
	stateStore state.Store,
	mailService *mail.Service,
	mailRecipients []string,
) *Runner {
	return &Runner{
		specs:          specs,
		cycleInterval:  cycleInterval,
		stateStore:     stateStore,
		mailService:    mailService,
		mailRecipients: mailRecipients,
	}
}

// Run executes checks immediately and then every cycle interval.
func (r *Runner) Run(ctx context.Context) {
	r.runCycle(ctx)

	ticker := time.NewTicker(r.cycleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runCycle(ctx)
		}
	}
}

func (r *Runner) runCycle(ctx context.Context) {
	r.cycleNumber++
	currentCycle := r.cycleNumber

	var wg sync.WaitGroup
	for _, parsedSpec := range r.specs {
		if !parsedSpec.IsActive() {
			continue
		}
		if !shouldRunSpecInCycle(parsedSpec, currentCycle) {
			continue
		}

		parsedSpec := parsedSpec
		wg.Go(func() {
			cycleStartedAt := time.Now()
			r.markCycleStarted(parsedSpec, cycleStartedAt)
			checkErr := validateSpec(ctx, parsedSpec)
			r.handleCycleResult(parsedSpec, checkErr, cycleStartedAt)
		})
	}
	wg.Wait()
}

func shouldRunSpecInCycle(parsedSpec spec.Spec, cycleNumber uint64) bool {
	everyCycles := specEveryCycles(parsedSpec)
	if everyCycles <= 1 {
		return true
	}
	return cycleNumber%uint64(everyCycles) == 0
}

func (r *Runner) markCycleStarted(parsedSpec spec.Spec, cycleStartedAt time.Time) {
	specID := parsedSpec.ID()
	currentState, ok := r.stateStore.Get(specID)
	if !ok {
		currentState = state.SpecState{Status: state.StatusHealthy}
	}
	currentState.LastCycleStartedAt = cycleStartedAt
	r.stateStore.Set(specID, currentState)
}

func (r *Runner) handleCycleResult(parsedSpec spec.Spec, checkErr error, cycleStartedAt time.Time) {
	cycles := specCycles(parsedSpec)
	failureThreshold := thresholdOrDefault(cycles.Failure, 1)
	successThreshold := thresholdOrDefault(cycles.Success, 1)
	cycleCompletedAt := time.Now()
	specID := parsedSpec.ID()
	specName := parsedSpec.Name()
	specType := parsedSpec.Kind()

	currentState, ok := r.stateStore.Get(specID)
	if !ok {
		currentState = state.SpecState{Status: state.StatusHealthy}
	}
	previousState := currentState
	currentState = resetStaleConsecutiveState(currentState, cycleCompletedAt, r.cycleInterval)

	nextState, transition := applyCycleResult(
		currentState,
		checkErr == nil,
		failureThreshold,
		successThreshold,
	)
	if hasStateChanged(previousState, nextState) {
		slog.Info("spec_state_changed",
			"name", specName,
			"type", specType,
			"source", parsedSpec.SourcePath,
			"from_status", previousState.Status,
			"to_status", nextState.Status,
			"from_consecutive_failures", previousState.ConsecutiveFailures,
			"to_consecutive_failures", nextState.ConsecutiveFailures,
			"from_consecutive_successes", previousState.ConsecutiveSuccesses,
			"to_consecutive_successes", nextState.ConsecutiveSuccesses,
		)
	}
	nextState.LastCycleStartedAt = cycleStartedAt
	nextState.LastCycleAt = cycleCompletedAt
	r.stateStore.Set(specID, nextState)
	took := cycleCompletedAt.Sub(cycleStartedAt)

	if checkErr == nil {
		slog.Debug("spec_ran",
			"name", specName,
			"type", specType,
			"result", "success",
			"source", parsedSpec.SourcePath,
			"took", took.String(),
		)
	} else {
		slog.Debug("spec_ran",
			"name", specName,
			"type", specType,
			"result", "failure",
			"source", parsedSpec.SourcePath,
			"took", took.String(),
			"error", checkErr,
		)
	}

	switch transition {
	case transitionFailure:
		slog.Warn("spec_failed",
			"name", specName,
			"type", specType,
			"source", parsedSpec.SourcePath,
			"error", checkErr,
		)
		r.triggerFailureActions(parsedSpec, checkErr)
	case transitionRecovery:
		slog.Info("spec_recovered",
			"name", specName,
			"type", specType,
			"source", parsedSpec.SourcePath,
		)
		r.triggerRecoveryActions(parsedSpec)
	}
}

func hasStateChanged(before, after state.SpecState) bool {
	return before.Status != after.Status ||
		before.ConsecutiveFailures != after.ConsecutiveFailures ||
		before.ConsecutiveSuccesses != after.ConsecutiveSuccesses
}

func resetStaleConsecutiveState(current state.SpecState, cycleAt time.Time, cycleInterval time.Duration) state.SpecState {
	if current.LastCycleAt.IsZero() || cycleInterval <= 0 {
		return current
	}

	maxGap := cycleInterval * staleCycleGapMultiplier
	if maxGap <= 0 {
		return current
	}
	if cycleAt.Sub(current.LastCycleAt) <= maxGap {
		return current
	}

	current.ConsecutiveFailures = 0
	current.ConsecutiveSuccesses = 0
	return current
}

func applyCycleResult(
	current state.SpecState,
	success bool,
	failureThreshold int,
	successThreshold int,
) (state.SpecState, transitionType) {
	if success {
		current.ConsecutiveFailures = 0
		if current.Status == state.StatusFailing {
			current.ConsecutiveSuccesses++
			if current.ConsecutiveSuccesses >= successThreshold {
				current.Status = state.StatusHealthy
				current.ConsecutiveSuccesses = 0
				return current, transitionRecovery
			}
			return current, transitionNone
		}
		current.ConsecutiveSuccesses = 0
		return current, transitionNone
	}

	current.ConsecutiveSuccesses = 0
	if current.Status == state.StatusFailing {
		current.ConsecutiveFailures++
		return current, transitionNone
	}

	current.ConsecutiveFailures++
	if current.ConsecutiveFailures >= failureThreshold {
		current.Status = state.StatusFailing
		current.ConsecutiveFailures = 0
		return current, transitionFailure
	}
	return current, transitionNone
}

func thresholdOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func specCycles(parsedSpec spec.Spec) spec.SpecCycles {
	switch {
	case parsedSpec.HTTP != nil:
		return parsedSpec.HTTP.Cycles
	case parsedSpec.TLS != nil:
		return parsedSpec.TLS.Cycles
	case parsedSpec.Probe != nil:
		return parsedSpec.Probe.Cycles
	case parsedSpec.S3 != nil:
		return parsedSpec.S3.Cycles
	default:
		return spec.SpecCycles{}
	}
}

func specEveryCycles(parsedSpec spec.Spec) int {
	switch {
	case parsedSpec.HTTP != nil:
		return parsedSpec.HTTP.EveryCycles
	case parsedSpec.TLS != nil:
		return parsedSpec.TLS.EveryCycles
	case parsedSpec.Probe != nil:
		return parsedSpec.Probe.EveryCycles
	case parsedSpec.S3 != nil:
		return parsedSpec.S3.EveryCycles
	default:
		return 0
	}
}

func specOnFailure(parsedSpec spec.Spec) string {
	switch {
	case parsedSpec.HTTP != nil:
		return parsedSpec.HTTP.OnFailure
	case parsedSpec.TLS != nil:
		return parsedSpec.TLS.OnFailure
	case parsedSpec.Probe != nil:
		return parsedSpec.Probe.OnFailure
	case parsedSpec.S3 != nil:
		return parsedSpec.S3.OnFailure
	default:
		return ""
	}
}

func specOnResolved(parsedSpec spec.Spec) string {
	switch {
	case parsedSpec.HTTP != nil:
		return parsedSpec.HTTP.OnResolved
	case parsedSpec.TLS != nil:
		return parsedSpec.TLS.OnResolved
	case parsedSpec.Probe != nil:
		return parsedSpec.Probe.OnResolved
	case parsedSpec.S3 != nil:
		return parsedSpec.S3.OnResolved
	default:
		return ""
	}
}

func specMailReceivers(parsedSpec spec.Spec) []string {
	switch {
	case parsedSpec.HTTP != nil:
		return parsedSpec.HTTP.MailReceivers
	case parsedSpec.TLS != nil:
		return parsedSpec.TLS.MailReceivers
	case parsedSpec.Probe != nil:
		return parsedSpec.Probe.MailReceivers
	case parsedSpec.S3 != nil:
		return parsedSpec.S3.MailReceivers
	default:
		return nil
	}
}

func validateSpec(ctx context.Context, parsedSpec spec.Spec) error {
	switch parsedSpec.Kind() {
	case "http":
		return validateHTTPSpec(ctx, parsedSpec)
	case "tls":
		return validateTLSSpec(ctx, parsedSpec)
	case "probe":
		return validateProbeSpec(ctx, parsedSpec)
	case "s3":
		return validateS3Spec(ctx, parsedSpec)
	default:
		return fmt.Errorf("unknown spec type")
	}
}

func validateHTTPSpec(ctx context.Context, parsedSpec spec.Spec) error {
	reqTimeout := parsedSpec.HTTP.Timeout
	if reqTimeout <= 0 {
		reqTimeout = 5 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, reqTimeout)
	defer cancel()

	targetURL, err := url.Parse(parsedSpec.HTTP.URL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return fmt.Errorf("url must include scheme and host: %q", parsedSpec.HTTP.URL)
	}

	if len(parsedSpec.HTTP.Args) > 0 {
		query := targetURL.Query()
		for key, value := range parsedSpec.HTTP.Args {
			query.Set(key, value)
		}
		targetURL.RawQuery = query.Encode()
	}

	method := strings.TrimSpace(parsedSpec.HTTP.Method)
	if method == "" {
		method = nethttp.MethodGet
	}

	req, err := nethttp.NewRequestWithContext(reqCtx, method, targetURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for headerName, value := range parsedSpec.HTTP.Headers {
		if strings.EqualFold(headerName, "host") {
			req.Host = value
			continue
		}
		req.Header.Set(headerName, value)
	}

	client := &nethttp.Client{Timeout: reqTimeout}
	if parsedSpec.HTTP.InsecureSkipTLS {
		client.Transport = &nethttp.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	if !parsedSpec.HTTP.FollowRedirects {
		client.CheckRedirect = func(_ *nethttp.Request, _ []*nethttp.Request) error {
			return nethttp.ErrUseLastResponse
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	bodyText := string(bodyBytes)

	if parsedSpec.HTTP.Expect.Code > 0 && resp.StatusCode != parsedSpec.HTTP.Expect.Code {
		return fmt.Errorf("unexpected status code: got %d, want %d", resp.StatusCode, parsedSpec.HTTP.Expect.Code)
	}
	if len(parsedSpec.HTTP.Expect.CodeAnyOf) > 0 && !containsStatusCode(parsedSpec.HTTP.Expect.CodeAnyOf, resp.StatusCode) {
		return fmt.Errorf("unexpected status code: got %d, want one of %v", resp.StatusCode, parsedSpec.HTTP.Expect.CodeAnyOf)
	}

	for headerName, expectedValue := range parsedSpec.HTTP.Expect.Header {
		actualValue := resp.Header.Get(headerName)
		if actualValue != expectedValue {
			return fmt.Errorf("unexpected header %q: got %q, want %q", headerName, actualValue, expectedValue)
		}
	}
	for headerName, expectedSubstring := range parsedSpec.HTTP.Expect.HeaderContains {
		actualValue := resp.Header.Get(headerName)
		if !strings.Contains(actualValue, expectedSubstring) {
			return fmt.Errorf("unexpected header %q: got %q, want substring %q", headerName, actualValue, expectedSubstring)
		}
	}

	if parsedSpec.HTTP.Expect.Body.Exact != "" && bodyText != parsedSpec.HTTP.Expect.Body.Exact {
		return fmt.Errorf("unexpected body exact match")
	}
	if parsedSpec.HTTP.Expect.Body.Contains != "" && !strings.Contains(bodyText, parsedSpec.HTTP.Expect.Body.Contains) {
		return fmt.Errorf("response body does not contain %q", parsedSpec.HTTP.Expect.Body.Contains)
	}

	return nil
}

func containsStatusCode(codes []int, statusCode int) bool {
	for _, code := range codes {
		if statusCode == code {
			return true
		}
	}
	return false
}

func (r *Runner) triggerFailureActions(parsedSpec spec.Spec, failureErr error) {
	onFailure := specOnFailure(parsedSpec)
	specName := parsedSpec.Name()
	specType := parsedSpec.Kind()
	specID := parsedSpec.ID()
	if onFailure != "" {
		go runScript("on_failure", specName, onFailure)
	}

	recipients := mergedMailRecipients(r.mailRecipients, specMailReceivers(parsedSpec))
	if r.mailService == nil || len(recipients) == 0 {
		return
	}
	subject := fmt.Sprintf("eddie failure: %s", specID)
	body := fmt.Sprintf(
		"spec failed: %s\r\nsource: %s\r\nreason: %v\r\n",
		specID,
		parsedSpec.SourcePath,
		failureErr,
	)
	r.sendEmailToRecipients(subject, body, recipients)
	slog.Debug("spec_failure_notification",
		"name", specName,
		"type", specType,
		"subject", subject,
		"recipient_count", len(recipients),
	)
}

func (r *Runner) triggerRecoveryActions(parsedSpec spec.Spec) {
	onResolved := specOnResolved(parsedSpec)
	specName := parsedSpec.Name()
	specType := parsedSpec.Kind()
	specID := parsedSpec.ID()
	if onResolved != "" {
		go runScript("on_resolved", specName, onResolved)
	}

	recipients := mergedMailRecipients(r.mailRecipients, specMailReceivers(parsedSpec))
	if r.mailService == nil || len(recipients) == 0 {
		return
	}
	subject := fmt.Sprintf("eddie recovery: %s", specID)
	body := fmt.Sprintf(
		"spec recovered: %s\r\nsource: %s\r\n",
		specID,
		parsedSpec.SourcePath,
	)
	r.sendEmailToRecipients(subject, body, recipients)
	slog.Debug("spec_recovery_notification",
		"name", specName,
		"type", specType,
		"subject", subject,
		"recipient_count", len(recipients),
	)
}

func (r *Runner) sendEmailToRecipients(subject, body string, recipients []string) {
	sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, recipient := range recipients {
		if err := r.mailService.Send(sendCtx, recipient, subject, body); err != nil {
			slog.Error("failed to send monitor email", "recipient", recipient, "error", err)
		}
	}
}

func mergedMailRecipients(globalRecipients, specRecipients []string) []string {
	seen := make(map[string]struct{}, len(globalRecipients)+len(specRecipients))
	merged := make([]string, 0, len(globalRecipients)+len(specRecipients))
	appendUnique := func(values []string) {
		for _, value := range values {
			normalized := strings.TrimSpace(value)
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			merged = append(merged, normalized)
		}
	}
	appendUnique(globalRecipients)
	appendUnique(specRecipients)
	return merged
}

func runScript(action, specName, script string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("script execution failed",
			"action", action,
			"spec", specName,
			"error", err,
			"output", strings.TrimSpace(string(output)),
		)
		return
	}

	slog.Debug("script_executed",
		"action", action,
		"spec", specName,
		"output", strings.TrimSpace(string(output)),
	)
}
