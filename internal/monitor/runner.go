package monitor

import (
	"context"
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
	var wg sync.WaitGroup
	for _, parsedSpec := range r.specs {
		if !parsedSpec.IsActive() {
			continue
		}

		parsedSpec := parsedSpec
		wg.Add(1)
		go func() {
			defer wg.Done()

			cycleStartedAt := time.Now()
			r.markCycleStarted(parsedSpec, cycleStartedAt)
			checkErr := validateHTTPSpec(ctx, parsedSpec)
			r.handleCycleResult(parsedSpec, checkErr, cycleStartedAt)
		}()
	}
	wg.Wait()
}

func (r *Runner) markCycleStarted(parsedSpec spec.Spec, cycleStartedAt time.Time) {
	currentState, ok := r.stateStore.Get(parsedSpec.HTTP.Name)
	if !ok {
		currentState = state.SpecState{Status: state.StatusHealthy}
	}
	currentState.LastCycleStartedAt = cycleStartedAt
	r.stateStore.Set(parsedSpec.HTTP.Name, currentState)
}

func (r *Runner) handleCycleResult(parsedSpec spec.Spec, checkErr error, cycleStartedAt time.Time) {
	failureThreshold := thresholdOrDefault(parsedSpec.HTTP.Cycles.Failure, 1)
	successThreshold := thresholdOrDefault(parsedSpec.HTTP.Cycles.Success, 1)
	cycleCompletedAt := time.Now()

	currentState, ok := r.stateStore.Get(parsedSpec.HTTP.Name)
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
			"name", parsedSpec.HTTP.Name,
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
	r.stateStore.Set(parsedSpec.HTTP.Name, nextState)

	if checkErr == nil {
		slog.Debug("spec_ran",
			"name", parsedSpec.HTTP.Name,
			"source", parsedSpec.SourcePath,
			"result", "success",
			"cycle_started_at", cycleStartedAt,
			"cycle_completed_at", cycleCompletedAt,
		)
	} else {
		slog.Debug("spec_ran",
			"name", parsedSpec.HTTP.Name,
			"source", parsedSpec.SourcePath,
			"result", "failure",
			"cycle_started_at", cycleStartedAt,
			"cycle_completed_at", cycleCompletedAt,
			"error", checkErr,
		)
	}

	switch transition {
	case transitionFailure:
		slog.Warn("spec_failed",
			"name", parsedSpec.HTTP.Name,
			"source", parsedSpec.SourcePath,
			"error", checkErr,
		)
		r.triggerFailureActions(parsedSpec, checkErr)
	case transitionRecovery:
		slog.Info("spec_recovered",
			"name", parsedSpec.HTTP.Name,
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

	client := &nethttp.Client{
		Timeout: reqTimeout,
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

	for headerName, expectedValue := range parsedSpec.HTTP.Expect.Header {
		actualValue := resp.Header.Get(headerName)
		if actualValue != expectedValue {
			return fmt.Errorf("unexpected header %q: got %q, want %q", headerName, actualValue, expectedValue)
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

func (r *Runner) triggerFailureActions(parsedSpec spec.Spec, failureErr error) {
	if parsedSpec.HTTP.OnFailure != "" {
		go runScript("on_failure", parsedSpec.HTTP.Name, parsedSpec.HTTP.OnFailure)
	}

	if r.mailService == nil || len(r.mailRecipients) == 0 {
		return
	}
	subject := fmt.Sprintf("eddie failure: %s", parsedSpec.HTTP.Name)
	body := fmt.Sprintf(
		"spec failed: %s\r\nsource: %s\r\nreason: %v\r\n",
		parsedSpec.HTTP.Name,
		parsedSpec.SourcePath,
		failureErr,
	)
	r.sendEmailToAll(subject, body)
}

func (r *Runner) triggerRecoveryActions(parsedSpec spec.Spec) {
	if parsedSpec.HTTP.OnSuccess != "" {
		go runScript("on_success", parsedSpec.HTTP.Name, parsedSpec.HTTP.OnSuccess)
	}

	if r.mailService == nil || len(r.mailRecipients) == 0 {
		return
	}
	subject := fmt.Sprintf("eddie recovery: %s", parsedSpec.HTTP.Name)
	body := fmt.Sprintf(
		"spec recovered: %s\r\nsource: %s\r\n",
		parsedSpec.HTTP.Name,
		parsedSpec.SourcePath,
	)
	r.sendEmailToAll(subject, body)
}

func (r *Runner) sendEmailToAll(subject, body string) {
	sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, recipient := range r.mailRecipients {
		if err := r.mailService.Send(sendCtx, recipient, subject, body); err != nil {
			slog.Error("failed to send monitor email", "recipient", recipient, "error", err)
		}
	}
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
